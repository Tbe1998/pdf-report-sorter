@echo off
chcp 65001 >nul
rem 在 Windows 上重新打包（需要先装 Go 1.21+：https://go.dev/dl/）
rem 打包出的 exe 不依赖 Go 运行时，可以直接发给别人用。
setlocal
cd /d "%~dp0"
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -trimpath -ldflags "-H windowsgui -s -w" -o "PDF文件夹搬运助手.exe" .
if errorlevel 1 (
  echo 打包失败。
  pause
  exit /b 1
)
echo 已生成 PDF文件夹搬运助手.exe
pause
