//go:build windows

package main

import (
	"runtime"
	"syscall"
	"unsafe"
)

var (
	procCreateFileW = syscall.NewLazyDLL("kernel32.dll").NewProc("CreateFileW")
	procCloseHandle = syscall.NewLazyDLL("kernel32.dll").NewProc("CloseHandle")
)

const (
	genericRead    = 0x80000000
	fileShareNone  = 0x00000000
	openExisting   = 3
	fileAttrNormal = 0x00000080
)

// isFileLocked 以"不允许任何共享"的方式尝试打开文件。
// 只要打印驱动、阅读器或任何进程还持有句柄，就会返回 ERROR_SHARING_VIOLATION，
// 说明文件尚未写完整，此时不能移动。
func isFileLocked(path string) bool {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	h, _, _ := procCreateFileW.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(genericRead),
		uintptr(fileShareNone),
		0,
		uintptr(openExisting),
		uintptr(fileAttrNormal),
		0,
	)
	runtime.KeepAlive(p)
	if h == uintptr(syscall.InvalidHandle) {
		return true
	}
	procCloseHandle.Call(h)
	return false
}
