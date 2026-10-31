package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestMover(t *testing.T, settle time.Duration, rules string, deflt string) (*Mover, *Router, string, string, []string) {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	var logs []string
	r := NewRouter(filepath.Join(root, "rules.txt"))
	rs, errs := ParseRules(rules)
	if len(errs) > 0 {
		t.Fatalf("规则解析出错：%v", errs)
	}
	r.rules = rs
	r.deflt = deflt
	m := NewMover()
	m.src = src
	m.settle = settle
	m.router = r
	m.logf = func(s string) { logs = append(logs, s) }
	return m, r, src, root, logs
}

func mkdir(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// 慢慢写一个 PDF：写的过程中不能搬走，写完并稳定后才搬走。
func TestSlowWriteNotMovedUntilStable(t *testing.T) {
	m, r, src, root, _ := newTestMover(t, 2*time.Second, "日报 => dst", "")
	dst := mkdir(t, filepath.Join(root, "dst"))
	r.rules[0].Dest = dst

	f := filepath.Join(src, "工程 日报.pdf")
	write(t, f, "%PDF-1.4 部分内容")

	t0 := time.Now()
	m.scanOnce(t0)
	if exists(filepath.Join(dst, "工程 日报.pdf")) {
		t.Fatal("首次发现就搬走了")
	}

	write(t, f, "%PDF-1.4 部分内容 更多更多更多")
	m.scanOnce(t0.Add(1 * time.Second))
	if exists(filepath.Join(dst, "工程 日报.pdf")) {
		t.Fatal("文件还在增长时被搬走")
	}

	m.scanOnce(t0.Add(2500 * time.Millisecond))
	if exists(filepath.Join(dst, "工程 日报.pdf")) {
		t.Fatal("稳定期未到就搬走了")
	}

	m.scanOnce(t0.Add(3600 * time.Millisecond))
	if !exists(filepath.Join(dst, "工程 日报.pdf")) {
		t.Fatal("稳定后没有搬到目标文件夹")
	}
	if exists(f) {
		t.Fatal("源文件应已移走")
	}
	if mv, _, _ := m.Stats(); mv != 1 {
		t.Fatalf("统计应为 1，实际 %d", mv)
	}
}

// 启动时已存在的 PDF，默认不搬。
func TestExistingFileSkipped(t *testing.T) {
	m, r, src, root, _ := newTestMover(t, 1*time.Second, "日报 => dst", "")
	dst := mkdir(t, filepath.Join(root, "dst"))
	r.rules[0].Dest = dst

	old := filepath.Join(src, "旧文件.pdf")
	write(t, old, "%PDF old")
	m.state["旧文件.pdf"] = &fileState{size: 8, mod: time.Now(), since: time.Now(), skip: true}

	t0 := time.Now()
	for i := 0; i < 4; i++ {
		m.scanOnce(t0.Add(time.Duration(i) * time.Second))
	}
	if !exists(old) {
		t.Fatal("已有文件被搬走了")
	}

	nw := filepath.Join(src, "新文件日报.pdf")
	write(t, nw, "%PDF new")
	m.scanOnce(t0)
	m.scanOnce(t0.Add(2 * time.Second))
	if !exists(filepath.Join(dst, "新文件日报.pdf")) {
		t.Fatal("新文件没有被搬走")
	}
}

// 目标文件夹已有同名文件时不覆盖，追加 (2)。
func TestNameConflict(t *testing.T) {
	m, r, src, root, _ := newTestMover(t, 1*time.Second, "报告 => dst", "")
	dst := mkdir(t, filepath.Join(root, "dst"))
	r.rules[0].Dest = dst
	write(t, filepath.Join(dst, "报告.pdf"), "old")

	f := filepath.Join(src, "报告.pdf")
	write(t, f, "new")

	t0 := time.Now()
	m.scanOnce(t0)
	m.scanOnce(t0.Add(2 * time.Second))

	if exists(f) {
		t.Fatal("源文件未移走")
	}
	if b, err := os.ReadFile(filepath.Join(dst, "报告 (2).pdf")); err != nil || string(b) != "new" {
		t.Fatalf("应生成 报告 (2).pdf，实际 %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "报告.pdf")); string(b) != "old" {
		t.Fatal("同名旧文件被覆盖了")
	}
}

// 非 PDF 文件必须无视。
func TestIgnoresNonPDF(t *testing.T) {
	m, r, src, root, _ := newTestMover(t, 1*time.Second, "日报 => dst", "")
	mkdir(t, filepath.Join(root, "dst"))
	write(t, filepath.Join(src, "note.txt"), "hi")
	t0 := time.Now()
	m.scanOnce(t0)
	m.scanOnce(t0.Add(3 * time.Second))
	if !exists(filepath.Join(src, "note.txt")) {
		t.Fatal("txt 被搬走了")
	}
	_ = r
}

// 按文件名分流到不同项目文件夹。
func TestRouteByProject(t *testing.T) {
	m, r, src, root, logs := newTestMover(t, 1*time.Second,
		"甲项目,车辆段 => P1\n城西路 => P2\n跨江 => P3", "")
	p1 := mkdir(t, filepath.Join(root, "P1"))
	p2 := mkdir(t, filepath.Join(root, "P2"))
	p3 := mkdir(t, filepath.Join(root, "P3"))
	r.rules[0].Dest = p1
	r.rules[1].Dest = p2
	r.rules[2].Dest = p3

	files := map[string]string{
		"地铁1号线车辆段施工Ⅲ标段监测日报2026-10-30.pdf": p1,
		"城西路隧道第三方监测日报2026-10-30.pdf":      p2,
		"跨江隧道监测日报.pdf":                    p3,
	}
	for name := range files {
		write(t, filepath.Join(src, name), "%PDF "+name)
	}
	t0 := time.Now()
	m.scanOnce(t0)
	m.scanOnce(t0.Add(2 * time.Second))

	for name, wantDir := range files {
		if !exists(filepath.Join(wantDir, name)) {
			t.Errorf("%s 没有进到 %s；日志：%v", name, filepath.Base(wantDir), logs)
		}
		if exists(filepath.Join(src, name)) {
			t.Errorf("%s 还在源文件夹", name)
		}
	}
	if mv, fl, sk := m.Stats(); mv != 3 || fl != 0 || sk != 0 {
		t.Fatalf("统计应为 3/0/0，实际 %d/%d/%d", mv, fl, sk)
	}
}

// 多个关键词同时命中时，取最长（最具体）的那条。
func TestLongestKeywordWins(t *testing.T) {
	m, r, src, root, _ := newTestMover(t, 1*time.Second, "隧道 => A\n城西路隧道 => B", "")
	a := mkdir(t, filepath.Join(root, "A"))
	b := mkdir(t, filepath.Join(root, "B"))
	r.rules[0].Dest = a
	r.rules[1].Dest = b

	name := "城西路隧道监测日报.pdf"
	write(t, filepath.Join(src, name), "%PDF x")
	t0 := time.Now()
	m.scanOnce(t0)
	m.scanOnce(t0.Add(2 * time.Second))

	if !exists(filepath.Join(b, name)) {
		t.Fatalf("应该进更精确的 B；日志见上")
	}
}

// 没命中规则、也没有默认目标 -> 留在原地，不报错。
func TestNoMatchKeepsFile(t *testing.T) {
	m, r, src, root, logs := newTestMover(t, 1*time.Second, "甲项目 => P1", "")
	mkdir(t, filepath.Join(root, "P1"))
	write(t, filepath.Join(src, "无关文件.pdf"), "%PDF x")
	t0 := time.Now()
	m.scanOnce(t0)
	m.scanOnce(t0.Add(2 * time.Second))
	if !exists(filepath.Join(src, "无关文件.pdf")) {
		t.Fatal("没匹配规则的文件被搬走了")
	}
	if _, _, sk := m.Stats(); sk != 1 {
		t.Fatalf("未匹配计数应为 1，实际 %d（日志 %v）", sk, logs)
	}
	_ = r
}

// 规则都没命中时走默认目标文件夹。
func TestDefaultFallback(t *testing.T) {
	m, r, src, root, _ := newTestMover(t, 1*time.Second, "甲项目 => P1", "")
	dflt := mkdir(t, filepath.Join(root, "DEFAULT"))
	r.deflt = dflt
	name := "其他项目日报.pdf"
	write(t, filepath.Join(src, name), "%PDF x")
	t0 := time.Now()
	m.scanOnce(t0)
	m.scanOnce(t0.Add(2 * time.Second))
	if !exists(filepath.Join(dflt, name)) {
		t.Fatal("没有走默认目标")
	}
}

// 文件名过滤：只处理包含"日报"的 PDF。
func TestFilter(t *testing.T) {
	m, r, src, root, _ := newTestMover(t, 1*time.Second, "日报 => P1", "")
	dst := mkdir(t, filepath.Join(root, "P1"))
	r.rules[0].Dest = dst
	r.filter = "日报"

	write(t, filepath.Join(src, "合同扫描件.pdf"), "%PDF a")
	write(t, filepath.Join(src, "项目日报.pdf"), "%PDF b")
	t0 := time.Now()
	m.scanOnce(t0)
	m.scanOnce(t0.Add(2 * time.Second))

	if !exists(filepath.Join(src, "合同扫描件.pdf")) {
		t.Fatal("不符合过滤条件的文件被搬走了")
	}
	if !exists(filepath.Join(dst, "项目日报.pdf")) {
		t.Fatal("符合过滤条件的日报没被搬走")
	}
}

// 规则文件解析：注释、空行、中文逗号、错误行。
func TestParseRules(t *testing.T) {
	content := `# 注释
甲项目，车辆段 => D:\a\甲项目
城西路	=> D:\a\城西路

坏行没有分隔符
空关键词 => 
跨江 => D:\a\跨江
`
	rules, errs := ParseRules(content)
	if len(rules) != 3 {
		t.Fatalf("应解析出 3 条规则，实际 %d：%v", len(rules), rules)
	}
	if len(rules[0].Keywords) != 2 || rules[0].Keywords[1] != "车辆段" {
		t.Fatalf("中文逗号分词失败：%v", rules[0].Keywords)
	}
	if rules[1].Dest != `D:\a\城西路` {
		t.Fatalf("tab 分隔解析失败：%q", rules[1].Dest)
	}
	if len(errs) != 2 {
		t.Fatalf("应报 2 个错误行，实际 %d：%v", len(errs), errs)
	}
}

// 大小写不敏感匹配。
func TestCaseInsensitiveMatch(t *testing.T) {
	r := NewRouter("x")
	r.rules = []Rule{{Keywords: []string{"ABC项目"}, Dest: "P"}}
	if _, _, ok := r.Resolve("abc项目日报2026.pdf"); !ok {
		t.Fatal("大小写不敏感匹配失败")
	}
	if _, _, ok := r.Resolve("其他.pdf"); ok {
		t.Fatal("不该匹配")
	}
}

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.10", "1.0.9", 1},
		{"v1.2.0", "1.1.9", 1},
		{"0.9", "1.0", -1},
		{"1.0.0", "1.0.0.1", -1},
		{"1.0.0-beta", "1.0.0", 0},
	}
	for _, c := range cases {
		if got := compareVersion(c.a, c.b); got != c.want {
			t.Errorf("compareVersion(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCopyFileIntegrity(t *testing.T) {
	d := t.TempDir()
	s := filepath.Join(d, "a.bin")
	dst := filepath.Join(d, "b.bin")
	data := make([]byte, 3<<20)
	for i := range data {
		data[i] = byte(i * 7)
	}
	if err := os.WriteFile(s, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(s, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || len(got) != len(data) || string(got) != string(data) {
		t.Fatal("复制后内容不一致")
	}
}

func TestIsPDF(t *testing.T) {
	for _, n := range []string{"a.pdf", "A.PDF", "日报.Pdf"} {
		if !isPDF(n) {
			t.Errorf("%s 应识别为 PDF", n)
		}
	}
	for _, n := range []string{"a.txt", "a.pdf.tmp", "pdf"} {
		if isPDF(n) {
			t.Errorf("%s 不该识别为 PDF", n)
		}
	}
	if !strings.HasSuffix(filepath.Ext("a.pdf"), "pdf") {
		t.Fatal("sanity")
	}
}

// 启动决策：自动分拣 与 自动检查更新 必须能同时为真（v1.2.0 的 if/else bug）
func TestDecideStartup(t *testing.T) {
	cfg := Config{SourceDir: `D:\待分拣`, AutoStartMon: true, AutoCheckUpd: true}
	start, check := decideStartup(cfg, false)
	if !start || !check {
		t.Fatalf("两者都应生效，实际 分拣=%v 检查更新=%v", start, check)
	}

	// 开机自启（--minimized）也要检查更新
	start, check = decideStartup(cfg, true)
	if !start || !check {
		t.Fatalf("最小化启动时两者都应生效，实际 分拣=%v 检查更新=%v", start, check)
	}

	// 没设源文件夹：不自动分拣，但仍要检查更新
	cfg2 := Config{AutoStartMon: true, AutoCheckUpd: true}
	start, check = decideStartup(cfg2, false)
	if start || !check {
		t.Fatalf("无源文件夹时不该自动分拣，但仍应检查更新，实际 分拣=%v 检查=%v", start, check)
	}

	// 手动模式（没勾自动分拣）：只检查更新
	cfg3 := Config{SourceDir: "D:\\x", AutoStartMon: false, AutoCheckUpd: true}
	start, check = decideStartup(cfg3, false)
	if start || !check {
		t.Fatalf("手动模式：分拣=%v 检查=%v", start, check)
	}
}
