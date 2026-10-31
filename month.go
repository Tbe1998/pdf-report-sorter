package main

// 按月份归档：把目标文件夹再分一层"两位年.月"，例如 2026 年 9 月 → 26.9。
// 月份优先取文件名里的日期（日报文件名通常带日期，补录旧文件也准），
// 文件名里没有日期时退回文件的修改时间（打印落盘时间）。

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MonthFolder 返回月份文件夹名：两位年 + "." + 月（月份不补零）。
//
//	2026-09-30 → "26.9"
//	2026-10-01 → "26.10"
//	2025-01-05 → "25.1"
func MonthFolder(t time.Time) string {
	return fmt.Sprintf("%02d.%d", t.Year()%100, int(t.Month()))
}

// 文件名里的日期格式，按顺序尝试：先试带日期的，再试只要年月的。
var datePatterns = []*regexp.Regexp{
	// 2026-09-30 / 2026/9/30 / 2026.09.30 / 2026年9月30日
	regexp.MustCompile(`(20\d{2})\s*[-./年]\s*(\d{1,2})\s*[-./月]\s*(\d{1,2})\s*日?`),
	// 2026年9月 / 2026-09 / 2026.9
	regexp.MustCompile(`(20\d{2})\s*[-./年]\s*(\d{1,2})\s*月?`),
	// 20260930
	regexp.MustCompile(`(20\d{2})(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])`),
	// 202609
	regexp.MustCompile(`(20\d{2})(0[1-9]|1[0-2])`),
}

// ParseDateFromName 从文件名里解析日期。
// 同一个文件名里有多处日期时取最后一处（"xx日报2026-09-30" 这种习惯把日期放最后）。
func ParseDateFromName(name string) (time.Time, bool) {
	// 注意：不能用 filepath.Base —— 文件名里出现 "/" 时会被当成路径分隔符截断
	base := strings.TrimSuffix(name, filepath.Ext(name))
	for _, re := range datePatterns {
		ms := re.FindAllStringSubmatch(base, -1)
		for i := len(ms) - 1; i >= 0; i-- {
			m := ms[i]
			year, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			month, err := strconv.Atoi(m[2])
			if err != nil || month < 1 || month > 12 {
				continue
			}
			day := 1
			if len(m) > 3 {
				d, err := strconv.Atoi(m[3])
				if err != nil || d < 1 || d > 31 {
					continue
				}
				day = d
			}
			return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local), true
		}
	}
	return time.Time{}, false
}

// MonthDirFor 计算归档目录：目标文件夹\26.9。
// fromName 表示月份取自文件名（否则取自文件修改时间）。
func MonthDirFor(destDir, name string, modTime time.Time) (dir string, fromName bool) {
	if t, ok := ParseDateFromName(name); ok {
		return filepath.Join(destDir, MonthFolder(t)), true
	}
	return filepath.Join(destDir, MonthFolder(modTime)), false
}
