//go:build windows

package main

// 纯 Win32 界面（不依赖任何第三方库），编译产物是一个独立 exe，
// 在 Windows 上双击即用，不需要安装 Python / .NET / 运行库。

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")

	pRegisterClassExW   = user32.NewProc("RegisterClassExW")
	pCreateWindowExW    = user32.NewProc("CreateWindowExW")
	pDefWindowProcW     = user32.NewProc("DefWindowProcW")
	pGetMessageW        = user32.NewProc("GetMessageW")
	pTranslateMessage   = user32.NewProc("TranslateMessage")
	pDispatchMessageW   = user32.NewProc("DispatchMessageW")
	pPostQuitMessage    = user32.NewProc("PostQuitMessage")
	pShowWindow         = user32.NewProc("ShowWindow")
	pUpdateWindow       = user32.NewProc("UpdateWindow")
	pSendMessageW       = user32.NewProc("SendMessageW")
	pGetWindowTextW     = user32.NewProc("GetWindowTextW")
	pSetWindowTextW     = user32.NewProc("SetWindowTextW")
	pEnableWindow       = user32.NewProc("EnableWindow")
	pDestroyWindow      = user32.NewProc("DestroyWindow")
	pSetTimer           = user32.NewProc("SetTimer")
	pMessageBoxW        = user32.NewProc("MessageBoxW")
	pLoadCursorW        = user32.NewProc("LoadCursorW")
	pLoadIconW          = user32.NewProc("LoadIconW")
	pCreateFontW        = gdi32.NewProc("CreateFontW")
	pGetStockObject     = gdi32.NewProc("GetStockObject")
	pAdjustWindowRect   = user32.NewProc("AdjustWindowRect")
	pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	pGetDpiForSystem    = user32.NewProc("GetDpiForSystem")
	pGetSystemMetrics   = user32.NewProc("GetSystemMetrics")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pCreateMutexW     = kernel32.NewProc("CreateMutexW")

	pSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	pSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	pShellExecuteW        = shell32.NewProc("ShellExecuteW")
	pGetOpenFileNameW     = comdlg32.NewProc("GetOpenFileNameW")
	pShellNotifyIconW     = shell32.NewProc("Shell_NotifyIconW")
	pFindWindowW          = user32.NewProc("FindWindowW")
	pPostMessageW         = user32.NewProc("PostMessageW")
	pSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	pCreatePopupMenu      = user32.NewProc("CreatePopupMenu")
	pAppendMenuW          = user32.NewProc("AppendMenuW")
	pTrackPopupMenu       = user32.NewProc("TrackPopupMenu")
	pDestroyMenu          = user32.NewProc("DestroyMenu")
	pGetCursorPos         = user32.NewProc("GetCursorPos")
	pBeginPaint           = user32.NewProc("BeginPaint")
	pEndPaint             = user32.NewProc("EndPaint")
	pCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
	pCoInitializeEx       = ole32.NewProc("CoInitializeEx")
)

const (
	wsChild       = 0x40000000
	wsVisible     = 0x10000000
	wsBorder      = 0x00800000
	wsVScroll     = 0x00200000
	wsTabStop     = 0x00010000
	wsOverlapped  = 0x00CF0000
	wsThickFrame  = 0x00040000
	wsMaximizeBox = 0x00010000
	wsMinimizeBox = 0x00020000

	esAutoHScroll = 0x0080
	bsPushButton  = 0x00000000
	bsAutoCheck   = 0x00000003
	lbsNoIntegral = 0x00000100
	esMultiline   = 0x00000004
	esAutoVScroll = 0x00000040
	esReadOnly    = 0x00000800
	emSetSel      = 0x00B1
	emScrollCaret = 0x00B7

	wmCreate         = 0x0001
	wmDestroy        = 0x0002
	wmClose          = 0x0010
	wmCommand        = 0x0111
	wmTimer          = 0x0113
	wmSetFont        = 0x0030
	wmCtlColorStatic = 0x0138
	wsClipChildren   = 0x02000000
	bmSetCheck       = 0x00F1
	bmGetCheck       = 0x00F0
	lbAddString      = 0x0180
	lbDeleteStr      = 0x0182
	lbGetCount       = 0x018B
	lbSetTopIdx      = 0x0197

	wmTray            = 0x8001 // WM_APP+1：托盘图标回调
	wmShowMe          = 0x8002 // 第二个实例要求显示窗口
	swHide            = 0
	swMinimize        = 6
	swRestore         = 9
	swShowNormal      = 1
	swShowMinNoActive = 7
	cwUseDefault      = 0x80000000

	mbOK        = 0x00000000
	mbYesNo     = 0x00000004
	mbIconInfo  = 0x00000040
	mbIconWarn  = 0x00000030
	mbIconQuest = 0x00000020
	mbTopMost   = 0x00040000
	idYes       = 6

	bifReturnOnlyFSDirs = 0x0001
	bifEditBox          = 0x0010
	bifNewDialogStyle   = 0x0040
)

const (
	idEditSrc = 1001 + iota
	idBtnSrc
	idEditDst
	idBtnDst
	idEditSettle
	idChkMonth
	idChkExisting
	idChkAutorun
	idChkAutoUpd
	idEditUpdateURL
	idBtnCheckUpd
	idBtnStart
	idBtnStop
	idBtnOpenSrc
	idBtnOpenDst
	idListLog
	idStatus
	idEditRules
	idBtnRules
	idBtnRulesEdit
	idBtnRulesReload
	idEditFilter
	idLblRulesInfo
	idBtnQuit
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         syscall.Handle
	hCursor       syscall.Handle
	hbrBackground syscall.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       syscall.Handle
}

type msgT struct {
	hwnd    syscall.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
}

type rect struct {
	left, top, right, bottom int32
}

type paintStruct struct {
	hdc        uintptr
	fErase     int32
	rcPaint    rect
	fRestore   int32
	fIncUpdate int32
	rgb        [32]byte
}

type browseInfo struct {
	hwndOwner      syscall.Handle
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      *uint16
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

type uiEvent struct {
	kind int
	text string
	man  *Manifest
}

const (
	evLog = iota + 1
	evFound
)

type guiApp struct {
	hwnd   syscall.Handle
	cfg    Config
	mover  *Mover
	events chan uiEvent
	scale  float64
	font   syscall.Handle

	hSrc, hBtnSrc, hDst, hBtnDst, hSettle             syscall.Handle
	hChkMonth, hChkExisting, hChkAutorun, hChkAutoUpd syscall.Handle
	hBtnStart, hBtnStop                               syscall.Handle
	hBtnQuit                                          syscall.Handle
	hBtnOpenSrc, hBtnOpenDst                          syscall.Handle
	hLog, hStatus                                     syscall.Handle
	hRules, hBtnRules                                 syscall.Handle
	hBtnRulesEdit, hBtnRulesReload                    syscall.Handle
	hFilter, hLblRulesInfo                            syscall.Handle
	th                                                theme
	cards                                             []rect
	editFrames                                        []rect
	sectionLabels                                     []syscall.Handle
	allButtons                                        []syscall.Handle
	minimized                                         bool
	logBuf                                            []string
	router                                            *Router
	rulesMod                                          time.Time
	tick                                              int
	trayAdded                                         bool
	balloonShown                                      bool
	reallyQuit                                        bool
}

var (
	app       *guiApp
	hInstance syscall.Handle
	keepAlive syscall.Handle // 单实例互斥体句柄，不能释放
	trayIcon  syscall.Handle // 托盘/窗口图标
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			reportFatal("启动异常", r)
		}
	}()
	trace("==== 启动 v%s，参数 %v，系统盘临时目录 %s ====", appVersion, os.Args, os.TempDir())
	// Win32 消息循环必须固定在同一 OS 线程上，否则消息收不到
	runtime.LockOSThread()

	// 安装器模式：由新版程序接手，等旧进程退出后覆盖安装（不能走单实例检查）
	for _, arg := range os.Args[1:] {
		if arg == "--install-update" {
			runInstaller()
			return
		}
	}

	// 单实例：两个进程同时搬同一个文件夹会互相抢文件。
	name, _ := syscall.UTF16PtrFromString("Local\\PDFFolderMover-single-instance")
	h, _, e := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	runtime.KeepAlive(name)
	keepAlive = syscall.Handle(h)
	if e == syscall.ERROR_ALREADY_EXISTS {
		// 已经在后台跑了：让已有实例把窗口叫出来，然后本进程退出
		cls, _ := syscall.UTF16PtrFromString("PDFFolderMoverWnd")
		h2, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
		runtime.KeepAlive(cls)
		if h2 != 0 {
			pPostMessageW.Call(h2, wmShowMe, 0, 0)
		}
		return
	}

	cleanupOldUpdates()

	if missing := preflight(); len(missing) > 0 {
		trace("缺少系统函数：%v", missing)
		msgBox(0, "系统缺少以下函数，程序无法启动：\r\n\r\n"+strings.Join(missing, "\r\n")+
			"\r\n\r\n请把这句话发给我。", "启动失败", mbOK|mbIconWarn)
		return
	}

	for _, arg := range os.Args[1:] {
		if arg == "--selftest" {
			runSelfTest(false)
			return
		}
		if arg == "--selftest-console" {
			runSelfTest(true)
			return
		}
	}

	// 文件夹选择对话框（新版样式）需要 COM 初始化
	pCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED

	pSetProcessDPIAware.Call()
	scale := 1.0
	if err := pGetDpiForSystem.Find(); err == nil { // Win10 1607 以前没有这个函数
		if dpi, _, _ := pGetDpiForSystem.Call(); dpi >= 72 && dpi <= 480 {
			scale = float64(dpi) / 96.0
		}
	}
	trace("DPI 缩放 %.2f", scale)

	cfg := loadConfig()
	app = &guiApp{
		cfg:    cfg,
		mover:  NewMover(),
		events: make(chan uiEvent, 2048),
		router: NewRouter(cfg.RulesPath),
	}

	hInst, _, _ := pGetModuleHandleW.Call(0)
	hInstance = syscall.Handle(hInst)

	className, _ := syscall.UTF16PtrFromString("PDFFolderMoverWnd")
	wc := wndClassEx{
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     hInstance,
		hbrBackground: syscall.Handle(6), // COLOR_WINDOW+1
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	cursor, _, _ := pLoadCursorW.Call(0, uintptr(32512))
	wc.hCursor = syscall.Handle(cursor)
	// 优先用资源里嵌的图标（rsrc 生成的 rsrc_windows_amd64.syso，资源 ID 1）
	icon, _, _ := pLoadIconW.Call(uintptr(hInstance), 1)
	if icon == 0 {
		icon, _, _ = pLoadIconW.Call(0, uintptr(32512))
	}
	trayIcon = syscall.Handle(icon)
	wc.hIcon = trayIcon
	wc.hIconSm = trayIcon

	trace("注册窗口类…")
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		msgBox(0, "窗口注册失败："+err.Error(), "启动失败", mbOK|mbIconWarn)
		return
	}

	// 按 96 DPI 设计稿 780x690 的客户区，换算成含边框的窗口尺寸。
	app.scale = scale
	style := uint32(wsOverlapped)&^uint32(wsThickFrame)&^uint32(wsMaximizeBox) | wsClipChildren
	rc := rect{0, 0, app.px(830), app.px(850)}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), uintptr(style), 0)

	title, _ := syscall.UTF16PtrFromString("PDF 日报分拣助手  v" + appVersion)
	hwnd, _, err := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(style),
		uintptr(cwUseDefault), uintptr(cwUseDefault),
		uintptr(rc.right-rc.left), uintptr(rc.bottom-rc.top),
		0, 0, uintptr(hInstance), 0,
	)
	runtime.KeepAlive(className)
	runtime.KeepAlive(title)
	if hwnd == 0 {
		trace("窗口创建失败：%v", err)
		msgBox(0, "窗口创建失败："+err.Error(), "启动失败", mbOK|mbIconWarn)
		return
	}
	trace("窗口已创建 hwnd=%d", hwnd)

	minimized := false
	for _, a := range os.Args[1:] {
		if a == "--minimized" || a == "/minimized" {
			minimized = true
		}
	}
	app.minimized = minimized
	if minimized {
		pUpdateWindow.Call(hwnd)
		app.hideToTray() // 开机自启：直接进托盘，不弹窗口
	} else {
		pShowWindow.Call(hwnd, swShowNormal)
		pUpdateWindow.Call(hwnd)
	}
	trace("窗口已显示，进入消息循环")

	trace("进入消息循环")
	var m msgT
	seen := 0
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			trace("消息循环结束：GetMessageW 返回 %d（0=收到 WM_QUIT，-1=出错），进程退出", int32(r))
			break
		}
		if seen < 80 || m.message == wmClose || m.message == wmDestroy || m.message == wmCommand {
			trace("MSG id=0x%04x wParam=%d lParam=%d", m.message, m.wParam, m.lParam)
			seen++
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	trace("main 正常返回")
}

// preflight 在创建界面前把所有用到的系统函数解析一遍。
// 之前 CreateFontW 被错挂到 user32.dll 上，就是在回调里炸掉的 ——
// 有了这一步，同类错误会变成一条明确的提示。
func preflight() []string {
	procs := map[string]*syscall.LazyProc{
		"RegisterClassExW":      pRegisterClassExW,
		"CreateWindowExW":       pCreateWindowExW,
		"DefWindowProcW":        pDefWindowProcW,
		"GetMessageW":           pGetMessageW,
		"TranslateMessage":      pTranslateMessage,
		"DispatchMessageW":      pDispatchMessageW,
		"PostQuitMessage":       pPostQuitMessage,
		"ShowWindow":            pShowWindow,
		"UpdateWindow":          pUpdateWindow,
		"SendMessageW":          pSendMessageW,
		"GetWindowTextW":        pGetWindowTextW,
		"SetWindowTextW":        pSetWindowTextW,
		"EnableWindow":          pEnableWindow,
		"DestroyWindow":         pDestroyWindow,
		"SetTimer":              pSetTimer,
		"MessageBoxW":           pMessageBoxW,
		"LoadCursorW":           pLoadCursorW,
		"LoadIconW":             pLoadIconW,
		"AdjustWindowRect":      pAdjustWindowRect,
		"CreateFontW(gdi32)":    pCreateFontW,
		"GetStockObject(gdi32)": pGetStockObject,
		"GetModuleHandleW":      pGetModuleHandleW,
		"CreateMutexW":          pCreateMutexW,
		"SHBrowseForFolderW":    pSHBrowseForFolderW,
		"SHGetPathFromIDListW":  pSHGetPathFromIDListW,
		"ShellExecuteW":         pShellExecuteW,
		"GetOpenFileNameW":      pGetOpenFileNameW,
		"CoTaskMemFree":         pCoTaskMemFree,
		"CoInitializeEx":        pCoInitializeEx,
		"Shell_NotifyIconW":     pShellNotifyIconW,
		"FindWindowW":           pFindWindowW,
		"PostMessageW":          pPostMessageW,
		"SetForegroundWindow":   pSetForegroundWindow,
		"CreatePopupMenu":       pCreatePopupMenu,
		"AppendMenuW":           pAppendMenuW,
		"TrackPopupMenu":        pTrackPopupMenu,
		"DestroyMenu":           pDestroyMenu,
		"GetCursorPos":          pGetCursorPos,
	}
	var missing []string
	for name, pr := range procs {
		if err := pr.Find(); err != nil {
			missing = append(missing, name)
		}
	}
	return missing
}

// goSafe 跑一个协程并兜住 panic —— 协程里的 panic 不会被主线程的 recover 抓到，
// 在 GUI 程序里会表现为"进程无声消失"。
func (a *guiApp) goSafe(what string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				trace("协程 %s 崩溃：%v\n%s", what, r, string(debugStack()))
				a.logLine(fmt.Sprintf("内部错误（%s）：%v", what, r))
			}
		}()
		fn()
	}()
}

func (a *guiApp) px(v int32) int32 {
	return int32(float64(v)*a.scale + 0.5)
}

func wndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) (ret uintptr) {
	defer func() {
		if r := recover(); r != nil {
			reportFatal(fmt.Sprintf("界面消息处理异常 (msg=0x%x)", msg), r)
			ret = 0
		}
	}()
	if app == nil {
		r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return r
	}
	switch msg {
	case wmCreate:
		app.hwnd = hwnd
		trace("收到 WM_CREATE，开始创建控件…")
		app.buildUI()
		trace("控件创建完成")
		return 0
	case wmCommand:
		app.onCommand(int32(wParam & 0xFFFF))
		return 0
	case wmTimer:
		app.drain()
		app.tick++
		app.trackHover()
		if app.tick%5 == 0 {
			app.checkRulesChanged()
		}
		return 0
	case wmCtlColorStatic:
		return app.ctlColorStatic(wParam, syscall.Handle(lParam))
	case wmClose:
		app.onClose()
		return 0
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		app.paintWindow(hdc)
		pEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmEraseBg:
		var rc rect
		pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rc)))
		pFillRect.Call(wParam, uintptr(unsafe.Pointer(&rc)), app.th.brBg)
		return 1
	case wmDrawItem:
		if lParam != 0 {
			app.drawButton((*drawItem)(unsafe.Pointer(lParam)))
		}
		return 0
	case wmTray:
		app.onTray(lParam)
		return 0
	case wmShowMe:
		app.showFromTray()
		return 0
	case wmDestroy:
		trace("收到 WM_DESTROY，发送 WM_QUIT")
		app.removeTray()
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

func (a *guiApp) mk(class, text string, style uint32, x, y, w, h int32, id int32) syscall.Handle {
	cls, _ := syscall.UTF16PtrFromString(class)
	txt, _ := syscall.UTF16PtrFromString(text)
	hw, _, _ := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(txt)),
		uintptr(style|wsChild|wsVisible),
		uintptr(a.px(x)), uintptr(a.px(y)), uintptr(a.px(w)), uintptr(a.px(h)),
		uintptr(a.hwnd), uintptr(id), uintptr(hInstance), 0,
	)
	runtime.KeepAlive(cls)
	runtime.KeepAlive(txt)
	hh := syscall.Handle(hw)
	if a.font != 0 && hh != 0 {
		pSendMessageW.Call(uintptr(hh), wmSetFont, uintptr(a.font), 1)
	}
	return hh
}

func (a *guiApp) buildUI() {
	a.initTheme()
	a.font = a.th.fBody

	setFont := func(h syscall.Handle, f syscall.Handle) {
		pSendMessageW.Call(uintptr(h), wmSetFont, uintptr(f), 1)
	}
	lbl := func(text string, x, y, w, h int32) syscall.Handle {
		return a.mk("STATIC", text, 0, x, y, w, h, 0)
	}
	section := func(text string, x, y, w, h int32) syscall.Handle {
		h2 := lbl(text, x, y, w, h)
		setFont(h2, a.th.fLabel)
		a.sectionLabels = append(a.sectionLabels, h2)
		return h2
	}
	edit := func(x, y, w, h int32, id int32, extra uint32) syscall.Handle {
		h2 := a.mk("EDIT", "", esAutoHScroll|wsTabStop|extra, x, y, w, h, id)
		// 圆角外框稍微外扩 2px，由 paintWindow 负责画
		a.editFrames = append(a.editFrames, rect{
			a.px(x - 2), a.px(y - 2), a.px(x + w + 2), a.px(y + h + 2),
		})
		// 文字内边距，避免贴边
		pSendMessageW.Call(uintptr(h2), 0x00D3, /*EM_SETMARGINS*/
			0x0003 /*EC_LEFTMARGIN|EC_RIGHTMARGIN*/, uintptr(8|(8<<16)))
		return h2
	}
	btn := func(text string, x, y, w, h int32, id int32) syscall.Handle {
		b := a.mk("BUTTON", text, bsOwnerDraw|wsTabStop, x, y, w, h, id)
		a.allButtons = append(a.allButtons, b)
		return b
	}
	card := func(x, y, w, h int32) {
		a.cards = append(a.cards, rect{a.px(x), a.px(y), a.px(x + w), a.px(y + h)})
	}

	// 卡片底板
	card(16, 84, 798, 148)
	card(16, 244, 798, 150)
	card(16, 500, 798, 300)

	// ① 源文件夹
	section("① 源文件夹  ·  PDF 打印机会把日报生成到这里", 32, 96, 520, 20)
	a.hSrc = edit(32, 116, 670, 30, idEditSrc, 0)
	a.hBtnSrc = btn("浏览…", 710, 116, 88, 30, idBtnSrc)

	// ② 默认目标
	section("② 默认目标文件夹  ·  规则没命中时用，留空则留在原地", 32, 156, 620, 20)
	a.hDst = edit(32, 176, 670, 30, idEditDst, 0)
	a.hBtnDst = btn("浏览…", 710, 176, 88, 30, idBtnDst)

	// ③ 分拣规则
	section("③ 分拣规则  ·  关键词 => 项目文件夹", 32, 256, 500, 20)
	a.hRules = edit(32, 276, 460, 30, idEditRules, 0)
	a.hBtnRules = btn("浏览…", 500, 276, 78, 30, idBtnRules)
	a.hBtnRulesEdit = btn("编辑规则", 586, 276, 90, 30, idBtnRulesEdit)
	a.hBtnRulesReload = btn("重新加载", 684, 276, 114, 30, idBtnRulesReload)
	a.hLblRulesInfo = lbl("规则：未加载", 32, 312, 760, 20)
	setFont(a.hLblRulesInfo, a.th.fSmall)

	// ④ 过滤 + 稳定判定
	section("④ 只处理包含", 32, 340, 110, 20)
	a.hFilter = edit(146, 337, 140, 26, idEditFilter, 0)
	lbl("的 PDF（留空=全部）", 294, 340, 190, 20)
	lbl("稳定判定", 500, 340, 70, 20)
	a.hSettle = edit(574, 337, 56, 26, idEditSettle, 0x2000 /*ES_NUMBER*/)
	lbl("秒", 636, 340, 24, 20)

	// 动作按钮
	a.hBtnStart = btn("开始分拣", 32, 412, 150, 38, idBtnStart)
	a.hBtnStop = btn("停止", 190, 412, 100, 38, idBtnStop)
	a.hBtnOpenSrc = btn("打开源文件夹", 298, 412, 130, 38, idBtnOpenSrc)
	a.hBtnOpenDst = btn("打开默认目标", 436, 412, 140, 38, idBtnOpenDst)
	a.hBtnQuit = btn("退出程序", 584, 412, 110, 38, idBtnQuit)

	// 选项
	a.hChkMonth = a.mk("BUTTON", "按月份分文件夹", bsAutoCheck|wsTabStop, 32, 464, 260, 24, idChkMonth)
	a.hChkExisting = a.mk("BUTTON", "启动时分拣已有 PDF", bsAutoCheck|wsTabStop, 300, 464, 230, 24, idChkExisting)
	a.hChkAutorun = a.mk("BUTTON", "开机自动启动", bsAutoCheck|wsTabStop, 540, 464, 230, 24, idChkAutorun)
	// 更新检查改为常开且无界面（地址写死，用户不可见）
	a.hChkAutoUpd = 0
	a.cfg.AutoCheckUpd = true

	// 日志卡片
	section("运行日志", 32, 512, 200, 20)
	lgl := lbl("同一份记录在 "+logFilePath(), 130, 512, 660, 20)
	setFont(lgl, a.th.fSmall)
	a.hLog = a.mk("EDIT", "", wsVScroll|esMultiline|esReadOnly|esAutoVScroll|wsTabStop,
		32, 534, 750, 250, idListLog)
	a.editFrames = append(a.editFrames, rect{
		a.px(30), a.px(532), a.px(784), a.px(786),
	})
	pSendMessageW.Call(uintptr(a.hLog), 0x00D3, 0x0003, uintptr(10|(8<<16)))
	setFont(a.hLog, a.th.fLog)

	a.hStatus = lbl("状态：未运行", 16, 812, 780, 22)
	setFont(a.hStatus, a.th.fSmall)

	// 填配置
	a.setText(a.hSrc, a.cfg.SourceDir)
	a.setText(a.hDst, a.cfg.TargetDir)
	a.setText(a.hRules, a.cfg.RulesPath)
	a.setText(a.hFilter, a.cfg.Filter)
	a.setText(a.hSettle, fmt.Sprintf("%d", a.cfg.SettleSec))
	a.setCheck(a.hChkMonth, a.cfg.ByMonth)
	a.setCheck(a.hChkExisting, a.cfg.MoveExisting)
	if isAutoRun() {
		a.setCheck(a.hChkAutorun, true)
	}

	trace("控件句柄：src=%d dst=%d rules=%d filter=%d log=%d status=%d",
		a.hSrc, a.hDst, a.hRules, a.hFilter, a.hLog, a.hStatus)

	a.enable(a.hBtnStop, false)
	pSetTimer.Call(uintptr(a.hwnd), 1, 400, 0)

	a.logLine("程序启动，版本 v" + appVersion)
	a.logLine("设置文件：" + configPath())
	if a.cfg.AutoCheckUpd {
		a.logLine("更新检查：已启用（使用内置更新源）")
	}

	if err := EnsureRulesFile(a.cfg.RulesPath); err != nil {
		a.logLine("创建规则文件失败：" + err.Error())
	}
	a.reloadRules(false)

	startMover, checkUpdate := decideStartup(a.cfg, a.minimized)
	trace("启动决策：自动分拣=%v，自动检查更新=%v", startMover, checkUpdate)
	if startMover {
		a.startMover(true)
	}
	if checkUpdate {
		a.goSafe("自动检查更新", func() {
			time.Sleep(1500 * time.Millisecond)
			a.doCheckUpdate(defaultUpdateURL, false)
		})
	}
}

func (a *guiApp) getText(h syscall.Handle) string {
	buf := make([]uint16, 4096)
	n, _, _ := pGetWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return strings.TrimSpace(syscall.UTF16ToString(buf[:int(n)]))
}

func (a *guiApp) setText(h syscall.Handle, s string) {
	p, _ := syscall.UTF16PtrFromString(s)
	pSetWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p) // 必须：否则 GC 可能在系统函数读这块内存前把它回收
}

func (a *guiApp) setCheck(h syscall.Handle, on bool) {
	v := uintptr(0)
	if on {
		v = 1
	}
	pSendMessageW.Call(uintptr(h), bmSetCheck, v, 0)
}

func (a *guiApp) getCheck(h syscall.Handle) bool {
	r, _, _ := pSendMessageW.Call(uintptr(h), bmGetCheck, 0, 0)
	return r == 1
}

func (a *guiApp) enable(h syscall.Handle, on bool) {
	v := uintptr(0)
	if on {
		v = 1
	}
	pEnableWindow.Call(uintptr(h), v)
}

// logLine 可以从任意协程调用：写进通道，由界面线程（定时器）取出显示。
func (a *guiApp) logLine(s string) {
	line := time.Now().Format("15:04:05") + "  " + s
	select {
	case a.events <- uiEvent{kind: evLog, text: line}:
	default:
	}
}

func (a *guiApp) drain() {
	defer func() {
		if r := recover(); r != nil {
			trace("定时器处理异常：%v", r)
		}
	}()
	for {
		select {
		case e := <-a.events:
			switch e.kind {
			case evLog:
				a.appendLog(e.text)
				appendLogFile(e.text)
			case evFound:
				a.promptUpdate(e.man)
			}
		default:
			return
		}
	}
}

func (a *guiApp) appendLog(line string) {
	a.logBuf = append(a.logBuf, line)
	if len(a.logBuf) > 400 {
		a.logBuf = append([]string(nil), a.logBuf[len(a.logBuf)-400:]...)
	}
	if a.hLog == 0 {
		return // 控件没建起来，别去碰它
	}
	a.setText(a.hLog, strings.Join(a.logBuf, "\r\n"))
	// 光标移到末尾并滚动（这两个消息不带指针，安全）
	pSendMessageW.Call(uintptr(a.hLog), emSetSel, 0x7FFFFFFF, 0x7FFFFFFF)
	pSendMessageW.Call(uintptr(a.hLog), emScrollCaret, 0, 0)
}

func (a *guiApp) setStatus(s string) {
	a.setText(a.hStatus, "状态："+s)
}

func (a *guiApp) onCommand(id int32) {
	trace("命令 id=%d", id)
	switch id {
	case idBtnSrc:
		if d := browseForFolder(a.hwnd, "选择 PDF 打印机输出（监听）的文件夹"); d != "" {
			a.setText(a.hSrc, d)
		}
	case idBtnDst:
		if d := browseForFolder(a.hwnd, "选择默认目标文件夹（规则都没命中时用，可留空）"); d != "" {
			a.setText(a.hDst, d)
		}
	case idBtnRules:
		if f := browseForFile(a.hwnd, "选择规则文件（rules.txt）", "文本文件 (*.txt)\x00*.txt\x00所有文件 (*.*)\x00*.*\x00"); f != "" {
			a.setText(a.hRules, f)
			a.saveCfg()
			a.reloadRules(true)
		}
	case idBtnRulesEdit:
		a.editRules()
	case idBtnRulesReload:
		a.saveCfg()
		a.reloadRules(true)
	case idBtnStart:
		a.startMover(false)
	case idBtnStop:
		a.stopMover()
	case idBtnOpenSrc:
		a.shellOpen(a.getText(a.hSrc))
	case idBtnOpenDst:
		a.shellOpen(a.getText(a.hDst))
	case idBtnQuit:
		a.quitApp()
	case idChkMonth:
		on := a.getCheck(a.hChkMonth)
		a.mover.SetByMonth(on)
		if on {
			a.logLine("已开启按月份归档：目标文件夹\\26.9")
		} else {
			a.logLine("已关闭按月份归档：直接放进项目文件夹")
		}
		a.saveCfg()
	case idChkAutorun:
		a.applyAutoRun()
	}
}

// reloadRules 重新读取规则文件，并刷新界面上的规则摘要。
func (a *guiApp) reloadRules(verbose bool) {
	path := a.getText(a.hRules)
	if path == "" {
		a.setText(a.hLblRulesInfo, "规则：未设置规则文件")
		return
	}
	if err := EnsureRulesFile(path); err != nil {
		a.setText(a.hLblRulesInfo, "规则：无法创建规则文件 — "+err.Error())
	}
	a.router.SetPath(path)
	a.router.SetDefault(a.getText(a.hDst), a.getText(a.hFilter))
	n, err := a.router.Load()
	if err != nil {
		a.setText(a.hLblRulesInfo, "规则：读取失败 — "+err.Error())
		if verbose {
			a.logLine("读取规则失败：" + err.Error())
		}
		return
	}
	a.rulesMod = a.router.ModTime()
	errs := a.router.ErrLines()
	txt := fmt.Sprintf("规则：已加载 %d 条", n)
	if len(errs) > 0 {
		txt += fmt.Sprintf("，%d 行有问题", len(errs))
	}
	txt += "（点「编辑规则」修改后会自动重载）"
	a.setText(a.hLblRulesInfo, txt)
	if verbose {
		a.logLine(fmt.Sprintf("规则已重新加载：%d 条", n))
		for _, e := range errs {
			a.logLine("  " + e)
		}
		for _, r := range a.router.Rules() {
			a.logLine(fmt.Sprintf("  %s → %s", strings.Join(r.Keywords, "/"), r.Dest))
		}
	}
}

// checkRulesChanged 定时器里调用：规则文件被改过就自动重载。
func (a *guiApp) checkRulesChanged() {
	p := a.getText(a.hRules)
	if p == "" {
		return
	}
	st, err := os.Stat(p)
	if err != nil {
		return
	}
	if !st.ModTime().Equal(a.rulesMod) {
		a.logLine("检测到规则文件有改动，自动重载")
		a.reloadRules(true)
	}
}

// editRules 用系统默认编辑器打开规则文件（不存在则先按模板创建）。
func (a *guiApp) editRules() {
	p := a.getText(a.hRules)
	if p == "" {
		msgBox(a.hwnd, "请先指定规则文件的位置。", "还没有规则文件", mbOK|mbIconInfo)
		return
	}
	if err := EnsureRulesFile(p); err != nil {
		msgBox(a.hwnd, "创建规则文件失败：\r\n"+err.Error(), "打不开", mbOK|mbIconWarn)
		return
	}
	a.shellOpen(p)
	a.logLine("已打开规则文件，保存后会自动重载")
}

func (a *guiApp) saveCfg() {
	a.cfg.SourceDir = a.getText(a.hSrc)
	a.cfg.TargetDir = a.getText(a.hDst)
	a.cfg.RulesPath = a.getText(a.hRules)
	a.cfg.Filter = a.getText(a.hFilter)
	a.cfg.ByMonth = a.getCheck(a.hChkMonth)
	a.cfg.MoveExisting = a.getCheck(a.hChkExisting)
	a.cfg.AutoCheckUpd = true // 更新检查常开，界面上不提供开关
	a.cfg.WinAutoRun = a.getCheck(a.hChkAutorun)
	if n := atoiSafe(a.getText(a.hSettle)); n >= 1 && n <= 60 {
		a.cfg.SettleSec = n
	} else {
		a.cfg.SettleSec = 3
		a.setText(a.hSettle, "3")
	}
	if err := a.cfg.save(); err != nil {
		a.logLine("保存设置失败：" + err.Error())
	}
}

func atoiSafe(s string) int {
	n := 0
	neg := false
	for i, c := range s {
		if i == 0 && (c == '-' || c == '+') {
			neg = c == '-'
			continue
		}
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
		if n > 100000 {
			return -1
		}
	}
	if neg {
		n = -n
	}
	return n
}

func (a *guiApp) applyAutoRun() {
	want := a.getCheck(a.hChkAutorun)
	exe, err := os.Executable()
	if err != nil {
		a.logLine("读取程序路径失败：" + err.Error())
		return
	}
	exe, _ = filepath.Abs(exe)
	if err := setAutoRun(want, exe); err != nil {
		a.logLine("设置开机启动失败：" + err.Error())
		a.setCheck(a.hChkAutorun, !want)
		return
	}
	if want {
		a.logLine("已设置开机自动启动")
	} else {
		a.logLine("已取消开机自动启动")
	}
}

func (a *guiApp) startMover(silent bool) {
	if a.mover.Running() {
		return
	}
	a.saveCfg()
	if a.cfg.SourceDir == "" {
		if !silent {
			msgBox(a.hwnd, "请先选择源文件夹。", "还差一步", mbOK|mbIconInfo)
		}
		return
	}
	// 每次启动前重新读一遍规则，保证用的是最新规则
	a.router.SetPath(a.cfg.RulesPath)
	a.router.SetDefault(a.cfg.TargetDir, a.cfg.Filter)
	if _, err := a.router.Load(); err != nil {
		a.logLine("读取规则失败：" + err.Error())
	}
	if a.router.Count() == 0 && a.cfg.TargetDir == "" {
		if !silent {
			msgBox(a.hwnd, "还没有任何分拣规则，也没设置默认目标文件夹。\r\n\r\n"+
				"请点「编辑规则」，按里面的示例填上：\r\n关键词 => 项目文件夹", "无法开始", mbOK|mbIconWarn)
		}
		return
	}
	a.mover.SetByMonth(a.cfg.ByMonth)
	err := a.mover.Start(a.cfg.SourceDir, a.cfg.SettleSec, a.cfg.MoveExisting, a.router, a.logLine)
	if err != nil {
		a.logLine("启动失败：" + err.Error())
		if !silent {
			msgBox(a.hwnd, err.Error(), "无法开始分拣", mbOK|mbIconWarn)
		}
		a.setStatus("未运行")
		return
	}
	a.enable(a.hBtnStart, false)
	a.enable(a.hBtnStop, true)
	a.cfg.AutoStartMon = true
	_ = a.cfg.save()
	a.setStatus(fmt.Sprintf("分拣中 …（规则 %d 条）", a.router.Count()))
}

func (a *guiApp) stopMover() {
	a.mover.Stop()
	a.cfg.AutoStartMon = false
	_ = a.cfg.save()
	a.enable(a.hBtnStart, true)
	a.enable(a.hBtnStop, false)
	mv, fl, sk := a.mover.Stats()
	a.setStatus(fmt.Sprintf("已停止（本次成功 %d 个，失败 %d 个，未匹配 %d 个）", mv, fl, sk))
}

func (a *guiApp) shellOpen(p string) {
	if p == "" {
		return
	}
	if _, err := os.Stat(p); err != nil {
		msgBox(a.hwnd, "路径不存在：\r\n"+p, "打不开", mbOK|mbIconWarn)
		return
	}
	op, _ := syscall.UTF16PtrFromString("open")
	file, _ := syscall.UTF16PtrFromString(p)
	pShellExecuteW.Call(uintptr(a.hwnd), uintptr(unsafe.Pointer(op)),
		uintptr(unsafe.Pointer(file)), 0, 0, swShowNormal)
	runtime.KeepAlive(op)
	runtime.KeepAlive(file)
}

// ---------- 托盘（关闭窗口 = 退到后台） ----------

// NOTIFYICONDATAW 在 x64 上是 976 字节；这里按固定偏移写内存，
// 比在 Go 里手写结构体对齐更稳（对不上就会崩）。
const (
	nidSize        = 976
	nidOffHWnd     = 8
	nidOffUID      = 16
	nidOffUFlags   = 20
	nidOffCallback = 24
	nidOffHIcon    = 32
	nidOffSzTip    = 40  // 128 个 wchar = 256 字节
	nidOffSzInfo   = 304 // 256 个 wchar = 512 字节
	nidOffSzTitle  = 820 // 64 个 wchar = 128 字节
	nidOffInfoFlag = 948

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010

	nimAdd    = 0
	nimModify = 1
	nimDelete = 2
)

func putU32(b []byte, off int, v uint32)  { binary.LittleEndian.PutUint32(b[off:], v) }
func putPtr(b []byte, off int, v uintptr) { binary.LittleEndian.PutUint64(b[off:], uint64(v)) }

func putW(b []byte, off, maxW int, s string) {
	u := utf16.Encode([]rune(s))
	if len(u) > maxW-1 {
		u = u[:maxW-1]
	}
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[off+i*2:], uint16(c))
	}
}

// trayData 组装 NOTIFYICONDATAW 的字节缓冲。
func (a *guiApp) trayData(withInfo bool) []byte {
	b := make([]byte, nidSize)
	putU32(b, 0, nidSize) // cbSize
	putPtr(b, nidOffHWnd, uintptr(a.hwnd))
	putU32(b, nidOffUID, 1)
	flags := uint32(nifMessage | nifIcon | nifTip)
	if withInfo {
		flags |= nifInfo
	}
	putU32(b, nidOffUFlags, flags)
	putU32(b, nidOffCallback, wmTray)
	putPtr(b, nidOffHIcon, uintptr(trayIcon))
	putW(b, nidOffSzTip, 128, "PDF 日报分拣助手（后台分拣中）")
	if withInfo {
		putW(b, nidOffSzTitle, 64, "PDF 日报分拣助手")
		putW(b, nidOffSzInfo, 256, "已最小化到托盘，仍在后台分拣。双击图标可打开，右键可退出。")
		putU32(b, nidOffInfoFlag, 1) // NIIF_INFO
	}
	return b
}

func (a *guiApp) addTray() bool {
	if a.trayAdded {
		return true
	}
	b := a.trayData(false)
	r, _, err := pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&b[0])))
	runtime.KeepAlive(b)
	if r == 0 {
		trace("托盘图标创建失败：%v", err)
		return false
	}
	a.trayAdded = true
	return true
}

func (a *guiApp) removeTray() {
	if !a.trayAdded {
		return
	}
	b := a.trayData(false)
	pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&b[0])))
	runtime.KeepAlive(b)
	a.trayAdded = false
}

// hideToTray 隐藏窗口，只留托盘图标。
func (a *guiApp) hideToTray() {
	if !a.addTray() {
		// 托盘建不出来就退回"最小化到任务栏"，否则窗口会彻底找不到
		pShowWindow.Call(uintptr(a.hwnd), swMinimize)
		a.logLine("无法创建托盘图标，已最小化到任务栏（仍在后台运行）")
		return
	}
	pShowWindow.Call(uintptr(a.hwnd), swHide)
	a.logLine("窗口已收到托盘，程序仍在后台运行")
	if !a.balloonShown {
		a.balloonShown = true
		b := a.trayData(true)
		pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&b[0])))
		runtime.KeepAlive(b)
	}
}

// showFromTray 从托盘重新打开窗口。
func (a *guiApp) showFromTray() {
	pShowWindow.Call(uintptr(a.hwnd), swShowNormal)
	pSetForegroundWindow.Call(uintptr(a.hwnd))
}

// onTray 处理托盘图标上的鼠标事件。
func (a *guiApp) onTray(lParam uintptr) {
	switch uint32(lParam) & 0xFFFF {
	case 0x0202, 0x0203: // WM_LBUTTONUP / WM_LBUTTONDBLCLK
		a.showFromTray()
	case 0x0205, 0x007B: // WM_RBUTTONUP / WM_CONTEXTMENU
		a.showTrayMenu()
	}
}

func (a *guiApp) showTrayMenu() {
	hMenu, _, _ := pCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	items := []struct {
		id   uintptr
		text string
	}{{1, "打开主窗口"}, {2, "打开日志文件夹"}, {3, "退出"}}
	for _, it := range items {
		t, _ := syscall.UTF16PtrFromString(it.text)
		pAppendMenuW.Call(hMenu, 0, it.id, uintptr(unsafe.Pointer(t)))
		runtime.KeepAlive(t)
	}
	var pt struct{ x, y int32 }
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(uintptr(a.hwnd))
	cmd, _, _ := pTrackPopupMenu.Call(hMenu,
		uintptr(0x0002|0x0100|0x0080), // TPM_RIGHTBUTTON|TPM_RETURNCMD|TPM_NONOTIFY
		uintptr(pt.x), uintptr(pt.y), 0, uintptr(a.hwnd), 0)
	pDestroyMenu.Call(hMenu)
	switch cmd {
	case 1:
		a.showFromTray()
	case 2:
		a.shellOpen(appDir())
	case 3:
		a.quitApp()
	}
}

// quitApp 真正退出（托盘菜单 / 界面退出按钮）。
func (a *guiApp) quitApp() {
	a.reallyQuit = true
	if a.mover.Running() {
		a.mover.Stop()
	}
	a.saveCfg()
	pDestroyWindow.Call(uintptr(a.hwnd))
}

func (a *guiApp) onClose() {
	trace("收到 WM_CLOSE")
	if a.reallyQuit {
		pDestroyWindow.Call(uintptr(a.hwnd))
		return
	}
	// 默认行为：退到托盘后台继续运行
	a.hideToTray()
}

// ---------- 更新 ----------

func (a *guiApp) doCheckUpdate(urlStr string, manual bool) {
	a.logLine("正在检查更新…")
	man, err := fetchManifest(urlStr)
	if err != nil {
		a.logLine("检查更新失败：" + err.Error())
		if manual {
			msgBox(a.hwnd, "检查更新失败：\n\n"+err.Error(), "检查更新", mbOK|mbIconWarn)
		}
		return
	}
	if compareVersion(man.Version, appVersion) <= 0 {
		a.logLine("已是最新版本 v" + appVersion)
		if manual {
			msgBox(a.hwnd, "当前已是最新版本 v"+appVersion+"。", "检查更新", mbOK|mbIconInfo)
		}
		return
	}
	a.logLine("发现新版本 v" + man.Version)
	a.events <- uiEvent{kind: evFound, man: man}
}

func (a *guiApp) promptUpdate(man *Manifest) {
	notes := strings.TrimSpace(man.Notes)
	if notes == "" {
		notes = "（无更新说明）"
	}
	text := fmt.Sprintf("发现新版本：v%s\n当前版本：v%s\n\n更新内容：\n%s\n\n是否现在下载并自动更新？\n（更新时程序会短暂重启，正在搬运的任务不受影响）",
		man.Version, appVersion, notes)
	if msgBox(a.hwnd, text, "发现新版本", mbYesNo|mbIconQuest) != idYes {
		a.logLine("已跳过本次更新")
		return
	}
	a.goSafe("下载安装更新", func() {
		a.logLine("开始下载更新包…")
		path, err := downloadPackage(man)
		if err != nil {
			a.logLine("下载失败：" + err.Error())
			return
		}
		a.logLine("下载完成，正在解压并替换…（程序会短暂重启）")
		if err := applyUpdate(path, man); err != nil {
			a.logLine("更新失败：" + err.Error())
		}
	})
}

// ---------- Win32 辅助 ----------

func msgBox(owner syscall.Handle, text, caption string, flags uint32) int32 {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(caption)
	r, _, _ := pMessageBoxW.Call(uintptr(owner), uintptr(unsafe.Pointer(t)),
		uintptr(unsafe.Pointer(c)), uintptr(flags))
	runtime.KeepAlive(t)
	runtime.KeepAlive(c)
	return int32(r)
}

// openFileName 对应 Win32 的 OPENFILENAMEW（x64 布局）。
type openFileName struct {
	lStructSize       uint32
	hwndOwner         syscall.Handle
	hInstance         syscall.Handle
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	Flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	FlagsEx           uint32
}

// utf16z 生成以 0 结尾的 UTF-16 缓冲区，允许字符串里带内嵌 NUL（过滤器需要）。
func utf16z(s string) []uint16 {
	r := utf16.Encode([]rune(s))
	return append(r, 0)
}

// browseForFile 弹出"打开文件"对话框。
func browseForFile(owner syscall.Handle, title, filter string) string {
	fbuf := utf16z(filter)
	fileBuf := make([]uint16, 1024)
	titleP, _ := syscall.UTF16PtrFromString(title)
	ofn := openFileName{
		hwndOwner:    owner,
		lpstrFilter:  &fbuf[0],
		nFilterIndex: 1,
		lpstrFile:    &fileBuf[0],
		nMaxFile:     uint32(len(fileBuf)),
		lpstrTitle:   titleP,
		// OFN_FILEMUSTEXIST|OFN_PATHMUSTEXIST|OFN_EXPLORER|OFN_HIDEREADONLY
		Flags: 0x00001000 | 0x00000800 | 0x00080000 | 0x00000004,
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	runtime.KeepAlive(fbuf)
	runtime.KeepAlive(fileBuf)
	runtime.KeepAlive(titleP)
	if r == 0 {
		return ""
	}
	return strings.TrimSpace(syscall.UTF16ToString(fileBuf))
}

func browseForFolder(owner syscall.Handle, title string) string {
	display := make([]uint16, 260)
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	bi := browseInfo{
		hwndOwner:      owner,
		pszDisplayName: &display[0],
		lpszTitle:      titlePtr,
		ulFlags:        bifReturnOnlyFSDirs | bifEditBox | bifNewDialogStyle,
	}
	pidl, _, _ := pSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	runtime.KeepAlive(titlePtr)
	runtime.KeepAlive(display)
	if pidl == 0 {
		return ""
	}
	defer pCoTaskMemFree.Call(pidl)
	buf := make([]uint16, 1024)
	ok, _, _ := pSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&buf[0])))
	runtime.KeepAlive(buf)
	if ok == 0 {
		return ""
	}
	return strings.TrimSpace(syscall.UTF16ToString(buf))
}

const runKeyPath = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
const runValueName = "PDFFolderMover"

func runReg(args ...string) error {
	cmd := exec.Command("reg.exe", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Run()
}

func setAutoRun(enable bool, exe string) error {
	if enable {
		return runReg("add", runKeyPath, "/v", runValueName, "/t", "REG_SZ",
			"/d", fmt.Sprintf("\"%s\" --minimized", exe), "/f")
	}
	err := runReg("delete", runKeyPath, "/v", runValueName, "/f")
	if err != nil {
		// 本来就没有这一项，等同于成功
		return nil
	}
	return nil
}

func isAutoRun() bool {
	return runReg("query", runKeyPath, "/v", runValueName) == nil
}
