//go:build !windows

package main

// 非 Windows 平台上用 flock 语义模拟"是否被独占"。
// 仅用于在 Linux 上验证核心逻辑，实际交付物是 Windows 版。
import "syscall"

func isFileLocked(path string) bool {
	fd, err := syscall.Open(path, syscall.O_RDONLY, 0)
	if err != nil {
		return true
	}
	defer syscall.Close(fd)
	// 尝试加独占锁：加不上说明有别的进程在写。
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return true
	}
	_ = syscall.Flock(fd, syscall.LOCK_UN)
	return false
}
