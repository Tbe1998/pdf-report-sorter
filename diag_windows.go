//go:build windows

package main

// 启动追踪：GUI 程序崩溃时没有任何控制台输出，
// 所以每一步都写进 %TEMP%\PDFFolderMover-startup.log，
// 出问题至少能看出死在哪一步。

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

func tracePath() string {
	return filepath.Join(os.TempDir(), "PDFFolderMover-startup.log")
}

func trace(format string, a ...any) {
	line := time.Now().Format("2006-01-02 15:04:05.000") + "  " + fmt.Sprintf(format, a...)
	f, err := os.OpenFile(tracePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line + "\r\n")
	_ = f.Close()
	// 诊断版有控制台时同时打印
	fmt.Println(line)
}

// reportFatal 处理 panic：写日志 + 弹窗 + 返回码 1。
func reportFatal(where string, r any) {
	stack := string(debug.Stack())
	msg := fmt.Sprintf("%s：%v", where, r)
	trace("！！！%s\n%s", msg, stack)
	appendLogFile("！！！" + msg + "\r\n" + stack)
	body := "程序出错了：\r\n\r\n" + msg +
		"\r\n\r\n详细信息已写入：\r\n" + tracePath() +
		"\r\n\r\n请把这个文件发给作者。"
	msgBox(0, body, "PDF 文件夹搬运助手", mbOK|mbIconWarn)
	os.Exit(1)
}

// debugStack 供其它文件取调用栈。
func debugStack() []byte { return debug.Stack() }
