//go:build windows

package main

// 自检：不碰用户的真实文件夹，只在本机临时目录里跑一遍完整流程，
// 用来确认"等文件写完才分拣"+"按文件名进对应项目文件夹"在这台电脑上确实成立。
//
// 命令行：PDF文件夹搬运助手.exe --selftest           （弹窗报结果）
//         PDF文件夹搬运助手.exe --selftest-console   （打到控制台，给 bat 采集）

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runSelfTest(console bool) {
	var lines []string
	pass, fail := 0, 0
	check := func(cond bool, msg string) {
		if cond {
			pass++
			lines = append(lines, "通过  "+msg)
		} else {
			fail++
			lines = append(lines, "失败  "+msg)
		}
	}

	root := filepath.Join(os.TempDir(), fmt.Sprintf("pdfmover-selftest-%d", time.Now().UnixNano()))
	defer os.RemoveAll(root)

	src := filepath.Join(root, "源文件夹")
	projA := filepath.Join(root, "项目甲")
	projB := filepath.Join(root, "项目乙")
	for _, d := range []string{src, projA, projB} {
		_ = os.MkdirAll(d, 0o755)
	}

	// 规则文件
	rulesTxt := "甲项目 => " + projA + "\r\n乙项目 => " + projB + "\r\n"
	rulesPath := filepath.Join(root, "rules.txt")
	_ = os.WriteFile(rulesPath, []byte(rulesTxt), 0o644)

	router := NewRouter(rulesPath)
	n, err := router.Load()
	check(err == nil && n == 2, fmt.Sprintf("规则文件能正确解析（%d 条）", n))

	// 启动前就存在的 PDF
	oldFile := filepath.Join(src, "启动前就有的甲项目.pdf")
	_ = os.WriteFile(oldFile, []byte("%PDF old"), 0o644)

	m := NewMover()
	var logs []string
	err = m.Start(src, 1, false, router, func(s string) { logs = append(logs, s) })
	check(err == nil, "监控可以启动")
	if err != nil {
		showSelfTest(lines, pass, fail, logs, console)
		return
	}

	// 模拟 PDF 打印机：先建文件、分几次写入
	nameA := "甲项目监测日报2026-10-30.pdf"
	pA := filepath.Join(src, nameA)
	f, ferr := os.Create(pA)
	check(ferr == nil, "能创建被监听的 PDF 文件")
	if ferr == nil {
		_, _ = f.Write([]byte("%PDF-1.7\n"))
		_ = f.Sync()
		time.Sleep(700 * time.Millisecond)
		_, _ = f.Write(make([]byte, 200000))
		_ = f.Sync()
		time.Sleep(600 * time.Millisecond)

		_, earlyErr := os.Stat(filepath.Join(projA, nameA))
		check(os.IsNotExist(earlyErr), "文件还在写入时不会被提前分拣")
		_ = f.Close()
	}

	// 等它被搬走（最多 8 秒）
	deadline := time.Now().Add(8 * time.Second)
	movedA := false
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(projA, nameA)); err == nil {
			movedA = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	check(movedA, "「甲项目」的日报被分到项目甲文件夹")
	if movedA {
		b, err := os.ReadFile(filepath.Join(projA, nameA))
		check(err == nil && strings.HasPrefix(string(b), "%PDF-1.7") && len(b) == 200010,
			fmt.Sprintf("移动后内容完整（%d 字节）", len(b)))
	}

	// 第二份，走另一个项目
	nameB := "乙项目监测日报2026-10-30.pdf"
	_ = os.WriteFile(filepath.Join(src, nameB), []byte("%PDF-1.7 B"), 0o644)
	deadline = time.Now().Add(8 * time.Second)
	movedB := false
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(projB, nameB)); err == nil {
			movedB = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	check(movedB, "「乙项目」的日报被分到项目乙文件夹（不是同一个目录）")

	check(existsFile(oldFile), "启动前就有的 PDF 没有被乱搬")
	check(!existsFile(filepath.Join(projA, "启动前就有的甲项目.pdf")), "启动前的旧文件没有进项目文件夹")

	mvt, flt, _ := m.Stats()
	check(mvt == 2 && flt == 0, fmt.Sprintf("统计正确（成功 %d、失败 %d）", mvt, flt))

	cfg := loadConfig()
	check(cfg.save() == nil, "设置文件可以正常保存到 "+appDir())
	check(EnsureRulesFile(filepath.Join(root, "new_rules.txt")) == nil, "规则模板可以自动创建")

	m.Stop()
	showSelfTest(lines, pass, fail, logs, console)
}

func existsFile(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func showSelfTest(lines []string, pass, fail int, logs []string, console bool) {
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l + "\r\n")
	}
	sb.WriteString(fmt.Sprintf("\r\n结果：通过 %d 项，失败 %d 项\r\n", pass, fail))
	sb.WriteString("\r\n运行日志：\r\n")
	for _, l := range logs {
		sb.WriteString("  " + l + "\r\n")
	}
	text := sb.String()
	appendLogFile("=== 自检 ===\r\n" + text)

	if console {
		fmt.Println("")
		fmt.Println("================ 自检结果 ================")
		fmt.Print(text)
		fmt.Println("日志文件：" + logFilePath())
		if fail == 0 {
			os.Exit(0)
		}
		os.Exit(1)
	}

	if fail == 0 {
		msgBox(0, "自检全部通过。\r\n\r\n说明这台电脑上「等 PDF 写完 → 按文件名分到对应项目文件夹」的逻辑工作正常。\r\n\r\n"+
			"自检没有动你的任何真实文件夹，临时目录已清理。",
			"自检通过", mbOK|mbIconInfo)
		os.Exit(0)
	}
	msgBox(0, "自检有项目失败：\r\n\r\n"+text+"\r\n详细信息已写入：\r\n"+logFilePath(),
		"自检未通过", mbOK|mbIconWarn)
	os.Exit(1)
}
