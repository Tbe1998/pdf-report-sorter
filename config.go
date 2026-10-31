package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config 是保存到 %APPDATA%\PDFFolderMover\config.json 的用户设置。
type Config struct {
	SourceDir    string `json:"source_dir"`     // 监听文件夹（PDF 打印机输出位置）
	TargetDir    string `json:"target_dir"`     // 默认目标文件夹（规则没命中时用，可空）
	RulesPath    string `json:"rules_path"`     // 分拣规则文件
	Filter       string `json:"filter"`         // 只处理文件名包含该词的 PDF（可空）
	SettleSec    int    `json:"settle_sec"`     // 文件大小/修改时间连续不变的秒数
	MoveExisting bool   `json:"move_existing"`  // 启动时是否也搬移已存在的 PDF
	ByMonth      bool   `json:"by_month"`       // 是否按月份再分一层子文件夹（26.9）
	AutoStartMon bool   `json:"auto_start_mon"` // 打开窗口后自动开始监控
	WinAutoRun   bool   `json:"autostart_windows"`
	AutoCheckUpd bool   `json:"auto_check_update"`
}

func defaultConfig() Config {
	return Config{
		SettleSec:    3,
		AutoCheckUpd: true,
		ByMonth:      true,
		RulesPath:    filepath.Join(appDir(), "rules.txt"),
		Filter:       "",
	}
}

func appDir() string {
	base, err := os.UserConfigDir() // Windows: %APPDATA%
	if err != nil || base == "" {
		base = "."
	}
	d := filepath.Join(base, "PDFFolderMover")
	_ = os.MkdirAll(d, 0o755)
	return d
}

func configPath() string  { return filepath.Join(appDir(), "config.json") }
func logFilePath() string { return filepath.Join(appDir(), "app.log") }

func loadConfig() Config {
	c := defaultConfig()
	if b, err := os.ReadFile(configPath()); err == nil {
		_ = json.Unmarshal(b, &c)
		// 老版本配置里没有 by_month 这一项：默认开启按月份归档
		var raw map[string]json.RawMessage
		if json.Unmarshal(b, &raw) == nil {
			if _, ok := raw["by_month"]; !ok {
				c.ByMonth = true
			}
		}
	}
	if c.SettleSec < 1 || c.SettleSec > 60 {
		c.SettleSec = 3
	}
	return c
}

func (c Config) save() error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), b, 0o644)
}

// appendLogFile 把日志追加到 app.log，超过 1MB 时自动截断重写。
func appendLogFile(line string) {
	p := logFilePath()
	if st, err := os.Stat(p); err == nil && st.Size() > 1<<20 {
		_ = os.WriteFile(p, []byte(""), 0o644)
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line + "\r\n")
}

// decideStartup 决定启动时要做什么。
// "自动开始分拣" 和 "自动检查更新" 是两件独立的事，必须分开判断 ——
// v1.2.0 写成了 if/else，导致勾了自动分拣就永远不检查更新。
func decideStartup(cfg Config, minimized bool) (startMover, checkUpdate bool) {
	startMover = (cfg.AutoStartMon || minimized) && cfg.SourceDir != ""
	checkUpdate = cfg.AutoCheckUpd
	return
}
