//go:build windows

package main

// 整包更新（Windows 端）：
//   1. 当前程序下载 zip 到 %TEMP%，校验大小/sha256；
//   2. 解压到 %TEMP%\pdfmover-update-<ver>\；
//   3. 启动解压出来的新程序，参数 --install-update --old-dir --new-dir --old-pid <本进程PID>；
//   4. 当前程序退出（释放 exe 文件锁）；
//   5. 新程序以"安装器"身份等本进程真正退出，然后整目录覆盖安装，再启动自己。

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// applyUpdate 执行更新：解压 + 拉起安装器，成功则退出本进程。
func applyUpdate(localPath string, m *Manifest) error {
	newDir := filepath.Join(os.TempDir(), fmt.Sprintf("pdfmover-update-%s", m.Version))
	_ = os.RemoveAll(newDir)
	if err := extractZip(localPath, newDir); err != nil {
		return err
	}
	newExe, err := findExeIn(newDir, m.ExeName)
	if err != nil {
		return err
	}
	oldExe, err := os.Executable()
	if err != nil {
		return err
	}
	oldExe, _ = filepath.Abs(oldExe)
	oldDir := filepath.Dir(oldExe)

	cmd := exec.Command(newExe,
		"--install-update",
		"--old-dir", oldDir,
		"--new-dir", newDir,
		"--old-pid", fmt.Sprintf("%d", os.Getpid()),
	)
	cmd.Dir = newDir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新程序失败：%v", err)
	}
	// 立即退出，让安装器能覆盖 exe
	os.Exit(0)
	return nil
}

// runInstaller 是 --install-update 模式的主逻辑。
func runInstaller() {
	var oldDir, newDir string
	oldPid := 0
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--old-dir":
			if i+1 < len(args) {
				oldDir = args[i+1]
				i++
			}
		case "--new-dir":
			if i+1 < len(args) {
				newDir = args[i+1]
				i++
			}
		case "--old-pid":
			if i+1 < len(args) {
				oldPid = atoiSafe(args[i+1])
				i++
			}
		}
	}
	if oldDir == "" || newDir == "" {
		return
	}
	exeName := filepath.Base(mustExecutable())

	// 1) 等旧进程彻底退出（exe 文件锁才会释放）
	waitProcessExit(oldPid, 120*time.Second)
	time.Sleep(400 * time.Millisecond)

	// 2) 整目录覆盖（保留用户自己的文件，只覆盖同名）
	failed := copyTree(newDir, oldDir)

	// 3) 启动新版本
	target := filepath.Join(oldDir, exeName)
	if _, err := os.Stat(target); err != nil {
		// 覆盖失败时，至少把主程序放回去
		_ = copyFile(filepath.Join(newDir, exeName), target)
	}
	cmd := exec.Command(target)
	cmd.Dir = oldDir
	_ = cmd.Start()

	// 4) 清理临时目录（自己还在里面，只能尽力而为）
	_ = os.RemoveAll(newDir)
	writeInstallLog(fmt.Sprintf("install-update: old=%s new=%s pid=%d copyFailed=%v", oldDir, newDir, oldPid, failed))
	os.Exit(0)
}

func mustExecutable() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}

// waitProcessExit 等指定 PID 结束；用 WaitForSingleObject，别用 tasklist 匹配（会子串误判）。
func waitProcessExit(pid int, timeout time.Duration) {
	if pid <= 0 {
		return
	}
	const synchronize = 0x00100000
	h, err := syscall.OpenProcess(synchronize, false, uint32(pid))
	if err != nil {
		return // 已经退出了
	}
	defer syscall.CloseHandle(h)
	ms := uint32(timeout / time.Millisecond)
	_, _ = syscall.WaitForSingleObject(h, ms)
}

// copyTree 把 src 目录里的文件覆盖到 dst（保留 dst 里多出来的文件）。
func copyTree(src, dst string) []string {
	var failed []string
	_ = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil || rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			_ = os.MkdirAll(target, 0o755)
			return nil
		}
		if err := copyFile(path, target); err != nil {
			failed = append(failed, rel+": "+err.Error())
		}
		return nil
	})
	return failed
}

func writeInstallLog(line string) {
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "PDFFolderMover-update.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(time.Now().Format("2006-01-02 15:04:05") + "  " + line + "\r\n")
}

// cleanupOldUpdates 启动时清理上次更新遗留的临时目录和 .old 文件。
func cleanupOldUpdates() {
	pattern := filepath.Join(os.TempDir(), "pdfmover-update-*")
	dirs, _ := filepath.Glob(pattern)
	for _, d := range dirs {
		_ = os.RemoveAll(d)
	}
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
	// 下载下来的包
	pkgs, _ := filepath.Glob(filepath.Join(os.TempDir(), "pdfmover-*.*"))
	for _, p := range pkgs {
		if strings.HasSuffix(strings.ToLower(p), ".zip") {
			if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) > 24*time.Hour {
				_ = os.Remove(p)
			}
		}
	}
}

// 保证 io 被使用（copyFile 在 watcher.go 里）
var _ = io.Copy
