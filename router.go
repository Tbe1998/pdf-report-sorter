package main

// 分拣路由：根据 PDF 文件名匹配规则，找到它该去的项目文件夹。
//
// 规则文件是纯文本，一行一条：
//     关键词1,关键词2 => D:\监测日报\地铁1号线
// 命中规则：文件名包含任一关键词（不区分大小写），多条同时命中时取"最长关键词"那条。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Rule struct {
	Keywords []string
	Dest     string
	lower    []string // 预计算的小写关键词，避免每次扫描重复 ToLower
}

type Router struct {
	mu      sync.RWMutex
	rules   []Rule
	deflt   string // 默认目标（规则都没命中时用，可空）
	filter  string // 只处理文件名包含该词的 PDF（可空=全部）
	path    string
	modTime time.Time
	errs    []string
	lastErr string
}

func NewRouter(path string) *Router {
	return &Router{path: path}
}

func (r *Router) Path() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.path
}

func (r *Router) SetPath(p string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.path = p
}

// SetDefault 设置"默认目标文件夹"和"文件名过滤词"。
func (r *Router) SetDefault(dir, filter string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deflt = strings.TrimSpace(dir)
	r.filter = strings.TrimSpace(filter)
}

func (r *Router) DefaultDir() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.deflt
}

func (r *Router) Filter() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.filter
}

func (r *Router) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.rules)
}

func (r *Router) Rules() []Rule {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Rule, len(r.rules))
	copy(out, r.rules)
	return out
}

func (r *Router) LastErr() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastErr
}

func (r *Router) ErrLines() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.errs))
	copy(out, r.errs)
	return out
}

func (r *Router) ModTime() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.modTime
}

// Load 读取并解析规则文件。返回规则条数。
func (r *Router) Load() (int, error) {
	p := r.Path()
	st, err := os.Stat(p)
	if err != nil {
		r.setErr(err.Error())
		return 0, fmt.Errorf("规则文件不存在：%s", p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		r.setErr(err.Error())
		return 0, err
	}
	rules, errLines := ParseRules(string(b))

	r.mu.Lock()
	r.rules = rules
	r.errs = errLines
	r.modTime = st.ModTime()
	if len(errLines) > 0 {
		r.lastErr = fmt.Sprintf("%d 行规则格式有问题", len(errLines))
	} else {
		r.lastErr = ""
	}
	r.mu.Unlock()
	return len(rules), nil
}

func (r *Router) setErr(msg string) {
	r.mu.Lock()
	r.lastErr = msg
	r.mu.Unlock()
}

// ParseRules 解析规则文本，返回规则和错误行说明。
func ParseRules(content string) ([]Rule, []string) {
	var rules []Rule
	var errs []string
	for i, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		var kwPart, dest string
		switch {
		case strings.Contains(line, "=>"):
			idx := strings.LastIndex(line, "=>")
			kwPart, dest = line[:idx], line[idx+2:]
		case strings.Contains(line, "\t"):
			idx := strings.LastIndex(line, "\t")
			kwPart, dest = line[:idx], line[idx+1:]
		default:
			errs = append(errs, fmt.Sprintf("第 %d 行缺少 => 分隔符：%s", i+1, line))
			continue
		}
		dest = strings.TrimSpace(dest)
		if dest == "" {
			errs = append(errs, fmt.Sprintf("第 %d 行没有目标文件夹", i+1))
			continue
		}
		kwPart = strings.NewReplacer("，", ",", "；", ",", ";", ",").Replace(kwPart)
		var kws []string
		for _, k := range strings.Split(kwPart, ",") {
			k = strings.TrimSpace(k)
			if k != "" {
				kws = append(kws, k)
			}
		}
		if len(kws) == 0 {
			errs = append(errs, fmt.Sprintf("第 %d 行没有关键词", i+1))
			continue
		}
		rules = append(rules, Rule{Keywords: kws, Dest: dest, lower: lowerAll(kws)})
	}
	return rules, errs
}

// lowerAll 预先把关键词转成小写，Resolve 里就只对文件名做一次 ToLower。
func lowerAll(kws []string) []string {
	out := make([]string, len(kws))
	for i, k := range kws {
		out[i] = strings.ToLower(k)
	}
	return out
}

// Resolve 决定一个文件名该去哪个目录。
// 返回：目标目录（空=不搬）、命中的关键词或说明、是否命中。
func (r *Router) Resolve(name string) (string, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lower := strings.ToLower(name)
	bestIdx, bestLen, bestKw := -1, 0, ""
	for i, ru := range r.rules {
		kws := ru.lower
		if len(kws) != len(ru.Keywords) {
			// 手工构造的 Rule（测试/老代码）没有预计算，回退现场转小写
			kws = lowerAll(ru.Keywords)
		}
		for j, kw := range kws {
			if kw == "" {
				continue
			}
			if len(kw) > bestLen && strings.Contains(lower, kw) {
				// 用字节长度比较，中文关键词更长的更精确；同长时先出现的优先
				bestIdx, bestLen, bestKw = i, len(kw), ru.Keywords[j]
			}
		}
	}
	if bestIdx >= 0 {
		return r.rules[bestIdx].Dest, bestKw, true
	}
	if r.deflt != "" {
		return r.deflt, "默认目标", true
	}
	return "", "", false
}

// PassFilter 判断文件名是否要通过"只处理包含某词的 PDF"这个过滤。
func (r *Router) PassFilter(name string) bool {
	r.mu.RLock()
	f := r.filter
	r.mu.RUnlock()
	if f == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(f))
}

// RuleTemplate 是首次运行时生成的规则文件内容。
const RuleTemplate = `# PDF 日报分拣规则
#
# 写法（一行一条）：关键词 => 目标文件夹
#   · 关键词可以用英文逗号或中文逗号分隔多个，命中任意一个就算匹配，不区分大小写
#   · 多个规则同时命中时，用"最长关键词"的那条（越具体越优先）
#   · 以 # 开头的行是注释
#
# 把下面示例前面的 # 去掉，改成你自己的项目和文件夹：
#
# 甲项目,车辆段 => D:\监测日报\地铁1号线
# 城西路      => D:\监测日报\城西路
# 跨江,隧道    => D:\监测日报\跨江隧道
#
# 注意：目标文件夹必须是已经存在的文件夹；没命中的 PDF 会留在源文件夹不动。
`

// EnsureRulesFile 首次运行时把模板写到规则文件里。
func EnsureRulesFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(RuleTemplate), 0o644)
}
