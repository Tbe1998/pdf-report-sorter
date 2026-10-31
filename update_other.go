//go:build !windows

package main

import "fmt"

// 自动更新只在 Windows 交付版里启用；其它平台编译时占位。
func applyUpdate(localPath string, m *Manifest) error {
	return fmt.Errorf("自动更新仅在 Windows 版可用")
}

func runInstaller() {}

func cleanupOldUpdates() {}
