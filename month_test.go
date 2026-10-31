package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMonthFolder(t *testing.T) {
	cases := []struct {
		t    time.Time
		want string
	}{
		{time.Date(2026, 9, 30, 10, 0, 0, 0, time.Local), "26.9"},
		{time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local), "26.10"},
		{time.Date(2026, 12, 31, 23, 59, 0, 0, time.Local), "26.12"},
		{time.Date(2025, 1, 5, 0, 0, 0, 0, time.Local), "25.1"},
		{time.Date(2030, 11, 7, 0, 0, 0, 0, time.Local), "30.11"},
	}
	for _, c := range cases {
		if got := MonthFolder(c.t); got != c.want {
			t.Errorf("MonthFolder(%s) = %q，应为 %q", c.t.Format("2006-01-02"), got, c.want)
		}
	}
}

func TestParseDateFromName(t *testing.T) {
	cases := []struct {
		name string
		want string // 空表示解析不出来
	}{
		{"地铁1号线监测日报2026-09-30.pdf", "2026-09"},
		{"监测日报2026-9-5.pdf", "2026-09"},
		{"监测日报2026/09/30.pdf", "2026-09"},
		{"监测日报2026.09.30.pdf", "2026-09"},
		{"监测日报2026年9月30日.pdf", "2026-09"},
		{"监测日报2026年10月.pdf", "2026-10"},
		{"监测日报20260930.pdf", "2026-09"},
		{"监测日报202610.pdf", "2026-10"},
		{"监测日报2026-12.pdf", "2026-12"},
		{"补录2025-13-01.pdf", ""},                       // 月份非法
		{"隧道第三方监测日报.pdf", ""},                          // 没有日期
		{"18号线车辆段监测日报.pdf", ""},                        // 只有线路号，不能当成日期
		{"日报2026-09-30（第2版）2026-10-08.pdf", "2026-10"}, // 多处日期取最后一个
	}
	for _, c := range cases {
		got, ok := ParseDateFromName(c.name)
		if c.want == "" {
			if ok {
				t.Errorf("%s 不该解析出日期，却得到 %s", c.name, got.Format("2006-01-02"))
			}
			continue
		}
		if !ok {
			t.Errorf("%s 应该解析出 %s，但没解析出来", c.name, c.want)
			continue
		}
		if s := got.Format("2006-01"); s != c.want {
			t.Errorf("%s 解析成 %s，应为 %s", c.name, s, c.want)
		}
	}
}

func TestMonthDirFor(t *testing.T) {
	dst := filepath.Join("D:", "监测日报", "项目甲")
	// 文件名带日期：按文件名
	dir, fromName := MonthDirFor(dst, "甲项目监测日报2026-09-30.pdf",
		time.Date(2026, 11, 2, 9, 0, 0, 0, time.Local))
	if !fromName || filepath.Base(dir) != "26.9" {
		t.Fatalf("应按文件名归到 26.9，实际 %s（fromName=%v）", dir, fromName)
	}
	if filepath.Dir(dir) != dst {
		t.Fatalf("月份文件夹应建在目标文件夹下，实际 %s", dir)
	}
	// 文件名没日期：按修改时间
	dir, fromName = MonthDirFor(dst, "甲项目监测日报.pdf",
		time.Date(2026, 11, 2, 9, 0, 0, 0, time.Local))
	if fromName || filepath.Base(dir) != "26.11" {
		t.Fatalf("应按文件时间归到 26.11，实际 %s（fromName=%v）", dir, fromName)
	}
}

// 端到端：开启按月份归档后，文件进 项目\26.9，且月份文件夹自动创建。
func TestMonthArchiveEndToEnd(t *testing.T) {
	m, r, src, root, logs := newTestMover(t, 1*time.Second, "甲项目 => P1", "")
	p1 := mkdir(t, filepath.Join(root, "P1"))
	r.rules[0].Dest = p1
	m.SetByMonth(true)

	name := "甲项目监测日报2026-09-30.pdf"
	write(t, filepath.Join(src, name), "%PDF month")

	t0 := time.Now()
	m.scanOnce(t0)
	m.scanOnce(t0.Add(2 * time.Second))

	want := filepath.Join(p1, "26.9", name)
	if !exists(want) {
		t.Fatalf("应落到 %s；日志：%v", want, logs)
	}
	if exists(filepath.Join(p1, name)) {
		t.Fatalf("不该直接放在项目文件夹根目录")
	}
	// 已存在的月份文件夹不能被重复创建/覆盖：再放一份同月文件
	name2 := "甲项目监测日报2026-09-15.pdf"
	write(t, filepath.Join(src, name2), "%PDF month2")
	m.scanOnce(t0)
	m.scanOnce(t0.Add(2 * time.Second))
	if !exists(filepath.Join(p1, "26.9", name2)) {
		t.Fatalf("同月第二份也应进 26.9；日志：%v", logs)
	}
}

// 关闭按月份归档：维持原来的行为（直接进项目文件夹）。
func TestMonthArchiveOff(t *testing.T) {
	m, r, src, root, _ := newTestMover(t, 1*time.Second, "甲项目 => P1", "")
	p1 := mkdir(t, filepath.Join(root, "P1"))
	r.rules[0].Dest = p1
	m.SetByMonth(false)

	name := "甲项目监测日报2026-09-30.pdf"
	write(t, filepath.Join(src, name), "%PDF x")
	t0 := time.Now()
	m.scanOnce(t0)
	m.scanOnce(t0.Add(2 * time.Second))

	if !exists(filepath.Join(p1, name)) {
		t.Fatalf("关闭时应直接放在项目文件夹")
	}
	if exists(filepath.Join(p1, "26.9")) {
		t.Fatalf("关闭时不该创建月份文件夹")
	}
}
