package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Mover 轮询监听源文件夹，等 PDF 完全写完后，按规则移动到对应项目文件夹。
type Mover struct {
	byMonth bool // 是否按月份再分一层子文件夹（26.9）
	mu      sync.Mutex
	src     string
	settle  time.Duration
	moveOld bool
	router  *Router
	logf    func(string)

	stopCh  chan struct{}
	running bool
	state   map[string]*fileState
	warned  map[string]bool
	moved   int
	failed  int
	skipped int // 没有匹配规则、留在原地的
}

// fileState 记录某个文件上一次观测到的形态。
type fileState struct {
	size  int64
	mod   time.Time
	since time.Time // 大小/修改时间与当前一致的起始时刻
	skip  bool      // 启动时就已存在、且用户选择不搬移的文件
}

func NewMover() *Mover {
	return &Mover{
		state:  map[string]*fileState{},
		warned: map[string]bool{},
	}
}

func (m *Mover) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *Mover) Stats() (moved, failed, skipped int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.moved, m.failed, m.skipped
}

// SetByMonth 设置是否按月份归档（目标文件夹\26.9）。运行中切换也会立刻生效。
func (m *Mover) SetByMonth(on bool) {
	m.mu.Lock()
	m.byMonth = on
	m.mu.Unlock()
}

func (m *Mover) SetRouter(r *Router) {
	m.mu.Lock()
	m.router = r
	m.mu.Unlock()
}

// Start 校验目录并启动监听协程。
func (m *Mover) Start(src string, settleSec int, moveOld bool, router *Router, logf func(string)) error {
	src = strings.TrimSpace(src)
	if src == "" {
		return fmt.Errorf("请先设置源文件夹")
	}
	si, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("源文件夹不存在或无法访问：%s", src)
	}
	if !si.IsDir() {
		return fmt.Errorf("源路径不是文件夹：%s", src)
	}
	if router == nil {
		return fmt.Errorf("内部错误：没有规则")
	}
	if router.Count() == 0 && router.DefaultDir() == "" {
		return fmt.Errorf("还没有任何分拣规则，也没设置默认目标文件夹")
	}
	if settleSec < 1 {
		settleSec = 1
	}
	if settleSec > 60 {
		settleSec = 60
	}

	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("已经在运行中")
	}
	m.src = src
	m.settle = time.Duration(settleSec) * time.Second
	m.moveOld = moveOld
	m.router = router
	m.logf = logf
	m.state = map[string]*fileState{}
	m.warned = map[string]bool{}
	m.moved, m.failed, m.skipped = 0, 0, 0
	m.stopCh = make(chan struct{})
	m.running = true
	// 启动时先给已有文件拍快照：moveOld=false 时它们被标记跳过，
	// 这样只搬移"启动之后新生成"的 PDF。
	if !moveOld {
		if entries, err := os.ReadDir(src); err == nil {
			now := time.Now()
			for _, e := range entries {
				if e.IsDir() || !isPDF(e.Name()) {
					continue
				}
				if info, err := e.Info(); err == nil {
					m.state[e.Name()] = &fileState{size: info.Size(), mod: info.ModTime(), since: now, skip: true}
				}
			}
		}
	}
	nExisting := len(m.state)
	stopCh := m.stopCh
	m.mu.Unlock()

	m.logf(fmt.Sprintf("开始监控：%s（稳定判定 %d 秒，规则 %d 条）", src, settleSec, router.Count()))
	if nExisting > 0 {
		m.logf(fmt.Sprintf("源文件夹内已有 %d 个 PDF，按设置不搬移", nExisting))
	}
	go m.loop(stopCh)
	return nil
}

func (m *Mover) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.stopCh)
	m.mu.Unlock()
	m.logf("已停止监控")
}

func (m *Mover) loop(stopCh chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			m.logf(fmt.Sprintf("监听协程异常退出：%v", r))
		}
	}()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			m.scanOnce(time.Now())
		}
	}
}

func isPDF(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".pdf")
}

func (m *Mover) scanOnce(now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			m.logf(fmt.Sprintf("扫描文件夹时出错：%v", r))
		}
	}()

	m.mu.Lock()
	src, router, byMonth := m.src, m.router, m.byMonth
	m.mu.Unlock()
	if router == nil {
		return
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		m.logf("读取源文件夹失败：" + err.Error())
		return
	}

	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isPDF(name) || strings.HasPrefix(name, "~$") || strings.HasPrefix(name, ".") {
			continue
		}
		// 文件名过滤（比如只处理"日报"）
		if !router.PassFilter(name) {
			continue
		}
		seen[name] = true
		info, err := e.Info()
		if err != nil {
			continue
		}
		full := filepath.Join(src, name)

		prev, known := m.state[name]
		if !known {
			// 第一次看到：先记账，等稳定期过去再说。
			m.state[name] = &fileState{size: info.Size(), mod: info.ModTime(), since: now}
			continue
		}
		if prev.skip {
			continue
		}
		if info.Size() != prev.size || !info.ModTime().Equal(prev.mod) {
			// 还在写：重置稳定计时。
			prev.size, prev.mod, prev.since = info.Size(), info.ModTime(), now
			continue
		}
		if now.Sub(prev.since) < m.settle {
			continue
		}
		// 稳定性达标，再确认没有进程独占该文件（打印驱动/阅读器仍持有句柄）。
		if isFileLocked(full) {
			if !m.warned[name] {
				m.warned[name] = true
				m.logf("文件仍被占用，继续等待：" + name)
			}
			continue
		}

		destDir, label, ok := router.Resolve(name)
		if !ok || destDir == "" {
			if !m.warned["nomatch:"+name] {
				m.warned["nomatch:"+name] = true
				m.logf("没有匹配的规则，留在原地：" + name)
			}
			m.mu.Lock()
			m.skipped++
			m.mu.Unlock()
			continue
		}
		if st, err := os.Stat(destDir); err != nil || !st.IsDir() {
			if !m.warned["nodir:"+name] {
				m.warned["nodir:"+name] = true
				m.logf(fmt.Sprintf("目标文件夹不存在，跳过：%s（%s）", destDir, name))
			}
			m.mu.Lock()
			m.failed++
			m.mu.Unlock()
			continue
		}

		delete(m.warned, name)
		destLabel := filepath.Base(destDir)
		if byMonth {
			monthDir, fromName := MonthDirFor(destDir, name, info.ModTime())
			if err := os.MkdirAll(monthDir, 0o755); err != nil {
				m.mu.Lock()
				m.failed++
				m.mu.Unlock()
				m.logf(fmt.Sprintf("创建月份文件夹失败：%s（%v）", monthDir, err))
				continue
			}
			destDir = monthDir
			destLabel = destLabel + "\\" + filepath.Base(monthDir)
			if !fromName && !m.warned["mtime:"+name] {
				m.warned["mtime:"+name] = true
				m.logf(fmt.Sprintf("文件名里没有日期，按文件时间归到 %s：%s",
					filepath.Base(monthDir), name))
			}
		}
		target := uniqueTarget(destDir, name)
		if err := moveFile(full, target); err != nil {
			m.mu.Lock()
			m.failed++
			m.mu.Unlock()
			m.logf("移动失败：" + name + " → " + err.Error())
			delete(m.state, name)
			continue
		}
		m.mu.Lock()
		m.moved++
		m.mu.Unlock()
		m.logf(fmt.Sprintf("已移动：%s → [%s] %s", name, label, destLabel))
		delete(m.state, name)
	}
	// 清理已经消失的记录
	for name := range m.state {
		if !seen[name] {
			delete(m.state, name)
			delete(m.warned, name)
			delete(m.warned, "nomatch:"+name)
			delete(m.warned, "nodir:"+name)
		}
	}
}

// uniqueTarget 生成不冲突的目标路径：同名时追加 (2)、(3)…
func uniqueTarget(dir, name string) string {
	base := filepath.Join(dir, name)
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return base
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 2; i <= 999; i++ {
		cand := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s (999)%s", stem, ext))
}

// moveFile 先尝试改名（同盘瞬间完成），跨盘时退回复制+删除。
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("已复制到目标位置，但删除源文件失败：%w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}

// isSubPath 判断 child 是否位于 parent 内部。
func isSubPath(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".."
}
