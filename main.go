package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"syscall"
	"unsafe"
)

const (
	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsChild            = 0x40000000
	cwUseDefault       = 0x80000000
	swShowDefault      = 10
	wmDestroy          = 0x0002
	wmSize             = 0x0005
	wmCommand          = 0x0111
	colorWindow        = 5  // COLOR_WINDOW
	colorBtnFace       = 15 // COLOR_BTNFACE

	mfString     = 0x00000000
	mfPopup      = 0x00000010
	mfSeparator  = 0x00000800
	mfByPosition = 0x00000400

	iccTabClasses      = 0x00000008
	iccBarClasses      = 0x00000004
	iccListViewClasses = 0x00000002

	tcmFirst       = 0x1300
	tcmInsertItem  = tcmFirst + 62 // TCM_INSERTITEMW
	tcmAdjustRect  = tcmFirst + 40 // TCM_ADJUSTRECT
	tcmGetCurSel   = tcmFirst + 11 // TCM_GETCURSEL
	tcifText       = 0x0001
	tcnSelChange   = -551 // TCN_SELCHANGE
	lvnColumnClick = -108 // LVN_FIRST(-100) - 8

	sbarsSizeGrip = 0x0100
	sbSetParts    = 0x0404 // SB_SETPARTS
	sbSetTextW    = 0x040B // SB_SETTEXTW

	wmSetFont              = 0x0030
	wmGetMinMaxInfo        = 0x0024
	wmNotify               = 0x004E
	wmTimer                = 0x0113
	wmCtlColorBtn          = 0x0135
	wmCtlColorStatic       = 0x0138
	transparentBkMode      = 1
	spiGetNonClientMetrics = 0x0029

	tabPadding = 6

	minWindowWidth  = 370
	minWindowHeight = 480

	idTab            = 100
	idStatus         = 101
	idFileExit       = 1001
	idHelpAbout      = 1002
	idOptAlwaysOnTop = 1003
	idOptMinimizeUse = 1004
	idOptHideWhenMin = 1005
	idHelpLink       = 1006
	idViewRefresh    = 1007
	idViewSpeedHigh  = 1008 // speed ids must stay contiguous for CheckMenuRadioItem
	idViewSpeedNorm  = 1009
	idViewSpeedLow   = 1010
	idViewSpeedPause = 1011
	idViewLargeIcons = 1012 // view mode ids must stay contiguous too
	idViewSmallIcons = 1013
	idViewDetails    = 1014
	idWinTileHorz    = 1015
	idWinTileVert    = 1016
	idWinMinimize    = 1017
	idWinMaximize    = 1018
	idWinCascade     = 1019
	idWinBringFront  = 1020
	idViewSelectCols = 1021
	idViewCPUHistAll = 1022 // contiguous with per-CPU id for CheckMenuRadioItem
	idViewCPUHistPer = 1023
	idViewKernelTime = 1024
	idAppListTimer   = 1
	timerIntervalMs  = 1500

	wmInitMenuPopup = 0x0117
	mfGrayed        = 0x00000001
	swMaximize      = 3
	mdiTileVert     = 0x0000 // MDITILE_VERTICAL
	mdiTileHorz     = 0x0001 // MDITILE_HORIZONTAL

	lvsTypeMask  = 0x0003
	lvsIcon      = 0x0000
	lvsSmallIcon = 0x0002

	swMinimize    = 6
	sizeMinimized = 1 // WM_SIZE wParam
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

type point struct{ x, y int32 }

type msg struct {
	hwnd    syscall.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type rect struct{ left, top, right, bottom int32 }

// minMaxInfo mirrors MINMAXINFO for enforcing a minimum window size on WM_GETMINMAXINFO.
type minMaxInfo struct {
	ptReserved     point
	ptMaxSize      point
	ptMaxPosition  point
	ptMinTrackSize point
	ptMaxTrackSize point
}

// nmhdr mirrors NMHDR for reading WM_NOTIFY codes off lParam.
type nmhdr struct {
	hwndFrom syscall.Handle
	idFrom   uintptr
	code     int32
}

// nmListView mirrors NMLISTVIEW for reading the clicked column off LVN_COLUMNCLICK;
// hdr's trailing padding (to 8-byte alignment) is load-bearing for iItem's offset.
type nmListView struct {
	hdr       nmhdr
	iItem     int32
	iSubItem  int32
	uNewState uint32
	uOldState uint32
	uChanged  uint32
	ptAction  point
	lParam    uintptr
}

type tcItemW struct {
	mask        uint32
	dwState     uint32
	dwStateMask uint32
	pszText     *uint16
	cchTextMax  int32
	iImage      int32
	lParam      uintptr
}

type initCommonControlsEx struct {
	dwSize uint32
	dwICC  uint32
}

type logFont struct {
	lfHeight         int32
	lfWidth          int32
	lfEscapement     int32
	lfOrientation    int32
	lfWeight         int32
	lfItalic         byte
	lfUnderline      byte
	lfStrikeOut      byte
	lfCharSet        byte
	lfOutPrecision   byte
	lfClipPrecision  byte
	lfQuality        byte
	lfPitchAndFamily byte
	lfFaceName       [32]uint16
}

// nonClientMetrics mirrors NONCLIENTMETRICSW including iPaddedBorderWidth (Vista+).
type nonClientMetrics struct {
	cbSize             uint32
	iBorderWidth       int32
	iScrollWidth       int32
	iScrollHeight      int32
	iCaptionWidth      int32
	iCaptionHeight     int32
	lfCaptionFont      logFont
	iSmCaptionWidth    int32
	iSmCaptionHeight   int32
	lfSmCaptionFont    logFont
	iMenuWidth         int32
	iMenuHeight        int32
	lfMenuFont         logFont
	lfStatusFont       logFont
	lfMessageFont      logFont
	iPaddedBorderWidth int32
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")

	procRegisterClassEx      = user32.NewProc("RegisterClassExW")
	procCreateWindowEx       = user32.NewProc("CreateWindowExW")
	procDefWindowProc        = user32.NewProc("DefWindowProcW")
	procShowWindow           = user32.NewProc("ShowWindow")
	procUpdateWindow         = user32.NewProc("UpdateWindow")
	procGetMessage           = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessage      = user32.NewProc("DispatchMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procLoadCursor           = user32.NewProc("LoadCursorW")
	procGetModuleHandle      = kernel32.NewProc("GetModuleHandleW")
	procCreateMenu           = user32.NewProc("CreateMenu")
	procCreatePopupMenu      = user32.NewProc("CreatePopupMenu")
	procAppendMenu           = user32.NewProc("AppendMenuW")
	procSetMenu              = user32.NewProc("SetMenu")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procMessageBox           = user32.NewProc("MessageBoxW")
	procGetClientRect        = user32.NewProc("GetClientRect")
	procGetWindowRect        = user32.NewProc("GetWindowRect")
	procMoveWindow           = user32.NewProc("MoveWindow")
	procSendMessage          = user32.NewProc("SendMessageW")
	procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
	procInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")
	procSetTimer             = user32.NewProc("SetTimer")
	procLoadIcon             = user32.NewProc("LoadIconW")
	procGetSystemDirectory   = kernel32.NewProc("GetSystemDirectoryW")
	procExtractIcon          = shell32.NewProc("ExtractIconW")
	procGetSysColorBrush     = user32.NewProc("GetSysColorBrush")
	gdi32                    = syscall.NewLazyDLL("gdi32.dll")
	procCreateFontIndirect   = gdi32.NewProc("CreateFontIndirectW")
	procSetBkMode            = gdi32.NewProc("SetBkMode")

	procCreateSolidBrush   = gdi32.NewProc("CreateSolidBrush")
	procCheckMenuItem      = user32.NewProc("CheckMenuItem")
	procCheckMenuRadioItem = user32.NewProc("CheckMenuRadioItem")
	procKillTimer          = user32.NewProc("KillTimer")
	procSetWindowLongW     = user32.NewProc("SetWindowLongW")
	procRemoveMenu         = user32.NewProc("RemoveMenu")
	procInsertMenu         = user32.NewProc("InsertMenuW")
	procDrawMenuBar        = user32.NewProc("DrawMenuBar")
	procEnableMenuItem     = user32.NewProc("EnableMenuItem")
	procTileWindows        = user32.NewProc("TileWindows")
	procCascadeWindows     = user32.NewProc("CascadeWindows")
	procBringWindowToTop   = user32.NewProc("BringWindowToTop")
)

// hwndTab and hwndStatus are set once in main and read by wndProc for layout.
var (
	hwndTab    syscall.Handle
	hwndStatus syscall.Handle

	hMainMenu          uintptr
	hViewMenu          uintptr
	hWindowsMenu       uintptr
	windowsMenuShown   bool
	appViewItemsShown  bool
	procViewItemsShown bool
	perfViewItemsShown bool
	hCPUHistMenu       uintptr
	appViewMode        = uintptr(idViewDetails) // Details is the startup default
	minimizeOnUse      bool
	hideWhenMinimized  bool
	updateIntervalMs   = timerIntervalMs // last non-paused timer interval
)

// wndProc is invoked directly by Windows (via the syscall.NewCallback registered as
// lpfnWndProc), so a panic here can't be caught by main()'s defer/recover -- Go can't
// unwind a panic across the native DispatchMessage stack frame in between, and the
// whole process aborts instead. Recover right here so a bug in any message handler
// (e.g. refreshAppList/refreshProcList on WM_TIMER) can't take down the app.
func wndProc(hwnd syscall.Handle, message uint32, wParam, lParam uintptr) (result uintptr) {
	defer func() {
		if r := recover(); r != nil {
			logCrash(r)
			ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
			result = ret
		}
	}()
	switch message {
	case wmDestroy:
		removeTrayIcon()
		procPostQuitMessage.Call(0)
		return 0
	case wmSize:
		if wParam == sizeMinimized && hideWhenMinimized {
			procShowWindow.Call(uintptr(hwnd), swHide)
			return 0
		}
		layoutChildren(hwnd)
		return 0
	case wmGetMinMaxInfo:
		mmi := *(**minMaxInfo)(unsafe.Pointer(&lParam))
		mmi.ptMinTrackSize = point{x: minWindowWidth, y: minWindowHeight}
		return 0
	case wmTimer:
		refreshAllViews()
		return 0
	case wmTrayCallback:
		switch lParam & 0xFFFF {
		case wmRButtonUp:
			showTrayMenu(hwnd)
		case wmLButtonDblClk:
			restoreMainWindow()
		}
		return 0
	case wmCtlColorBtn, wmCtlColorStatic:
		procSetBkMode.Call(wParam, transparentBkMode)

		if syscall.Handle(lParam) == hwndShowAllUsers {
			tabBgBrush, _, _ := procCreateSolidBrush.Call(0x00F9F9F9)
			return tabBgBrush
		}

		brush, _, _ := procGetSysColorBrush.Call(colorBtnFace)
		return brush
	case wmNotify:
		// Reinterpret via &lParam (a real *uintptr) rather than unsafe.Pointer(lParam)
		// directly, since the latter looks like a fabricated pointer to vet's unsafeptr check.
		hdr := *(**nmhdr)(unsafe.Pointer(&lParam))
		switch {
		case hdr.hwndFrom == hwndTab && hdr.code == tcnSelChange:
			sel, _, _ := procSendMessage.Call(uintptr(hwndTab), tcmGetCurSel, 0, 0)
			showAppList(int32(sel) == 0)
			showProcList(int32(sel) == 1)
			showPerfView(int32(sel) == 2)
			showNetView(int32(sel) == 3)
			updateAppViewMenu(int32(sel) == 0)
			updateWindowsMenu(hwnd, int32(sel) == 0)
			updateProcViewMenu(int32(sel) == 1)
			updatePerfViewMenu(int32(sel) == 2)
		case hdr.hwndFrom == hwndAppList && hdr.code == lvnColumnClick:
			nmlv := *(**nmListView)(unsafe.Pointer(&lParam))
			setAppSortColumn(nmlv.iSubItem)
		case hdr.hwndFrom == hwndProcList && hdr.code == lvnColumnClick:
			nmlv := *(**nmListView)(unsafe.Pointer(&lParam))
			setProcSortColumn(nmlv.iSubItem)
		}
		return 0
	case wmInitMenuPopup:
		if wParam == hWindowsMenu {
			updateWindowsMenuEnables()
			return 0
		}
	case wmCommand:
		switch wParam & 0xFFFF {
		case idFileExit:
			procDestroyWindow.Call(uintptr(hwnd))
			return 0
		case idHelpAbout:
			showAboutDialog(hwnd)
			return 0
		case idHelpLink:
			url, _ := syscall.UTF16PtrFromString("https://github.com/jankammerath/TaskMgr98")
			openVerb, _ := syscall.UTF16PtrFromString("open")
			procShellExecuteW.Call(0, uintptr(unsafe.Pointer(openVerb)), uintptr(unsafe.Pointer(url)), 0, 0, swShowDefault)
			return 0
		case idEndTask:
			endSelectedTask()
			return 0
		case idSwitchTo:
			if switchToSelectedTask() && minimizeOnUse {
				procShowWindow.Call(uintptr(hwnd), swMinimize)
			}
			return 0
		case idNewTask:
			if showNewTaskDialog(hwnd) && minimizeOnUse {
				procShowWindow.Call(uintptr(hwnd), swMinimize)
			}
			return 0
		case idEndProcess:
			endSelectedProcess()
			return 0
		case idShowAllUsers:
			refreshProcList()
			return 0
		case idTrayRestore:
			restoreMainWindow()
			return 0
		case idTrayClose:
			procDestroyWindow.Call(uintptr(hwnd))
			return 0
		case idTrayTopmost, idOptAlwaysOnTop:
			toggleAlwaysOnTop()
			return 0
		case idOptMinimizeUse:
			minimizeOnUse = !minimizeOnUse
			updateOptionsMenuChecks()
			return 0
		case idOptHideWhenMin:
			hideWhenMinimized = !hideWhenMinimized
			updateOptionsMenuChecks()
			return 0
		case idViewRefresh:
			refreshAllViews()
			return 0
		case idViewSpeedHigh:
			setUpdateSpeed(hwnd, 500, idViewSpeedHigh)
			return 0
		case idViewSpeedNorm:
			setUpdateSpeed(hwnd, timerIntervalMs, idViewSpeedNorm)
			return 0
		case idViewSpeedLow:
			setUpdateSpeed(hwnd, 4000, idViewSpeedLow)
			return 0
		case idViewSpeedPause:
			setUpdateSpeed(hwnd, 0, idViewSpeedPause)
			return 0
		case idViewLargeIcons, idViewSmallIcons, idViewDetails:
			setAppViewMode(wParam & 0xFFFF)
			return 0
		case idWinTileHorz, idWinTileVert, idWinMinimize, idWinMaximize, idWinCascade, idWinBringFront:
			runWindowsMenuAction(wParam & 0xFFFF)
			return 0
		case idViewSelectCols:
			showColumnsDialog(hwnd)
			return 0
		case idViewCPUHistAll, idViewCPUHistPer:
			cpuHistoryPerCPU = wParam&0xFFFF == idViewCPUHistPer
			syncPerfViewMenuChecks()
			procInvalidateRect.Call(uintptr(hwndCPUHist), 0, 0)
			return 0
		case idViewKernelTime:
			showKernelTimes = !showKernelTimes
			syncPerfViewMenuChecks()
			procInvalidateRect.Call(uintptr(hwndCPUHist), 0, 0)
			procInvalidateRect.Call(uintptr(hwndCPUMeter), 0, 0)
			return 0
		}
	}
	ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return ret
}

// refreshAllViews updates every tab's data plus the status bar and tray icon.
func refreshAllViews() {
	refreshAppList()
	refreshProcList()
	refreshPerfData()
	updateStatusBar()
	updateTrayIcon()
	if netViewVisible {
		refreshNetData()
	}
}

// setUpdateSpeed restarts (or, with ms == 0, pauses) the refresh timer and moves
// the Update Speed radio check.
func setUpdateSpeed(hwnd syscall.Handle, ms int, id uintptr) {
	if ms == 0 {
		procKillTimer.Call(uintptr(hwnd), idAppListTimer)
	} else {
		updateIntervalMs = ms
		procSetTimer.Call(uintptr(hwnd), idAppListTimer, uintptr(ms), 0)
	}
	if hMainMenu != 0 {
		procCheckMenuRadioItem.Call(hMainMenu, idViewSpeedHigh, idViewSpeedPause, id, 0) // MF_BYCOMMAND
	}
}

// updateAppViewMenu appends or removes the Applications-only view mode items,
// which sit at fixed positions 2-5 after "Refresh Now" and "Update Speed".
func updateAppViewMenu(show bool) {
	if hViewMenu == 0 || show == appViewItemsShown {
		return
	}
	if show {
		procAppendMenu.Call(hViewMenu, mfSeparator, 0, 0)
		for _, it := range []struct {
			id   uintptr
			text string
		}{
			{idViewLargeIcons, "Large Icons"},
			{idViewSmallIcons, "Small Icons"},
			{idViewDetails, "Details"},
		} {
			t, _ := syscall.UTF16PtrFromString(it.text)
			procAppendMenu.Call(hViewMenu, mfString, it.id, uintptr(unsafe.Pointer(t)))
		}
		procCheckMenuRadioItem.Call(hMainMenu, idViewLargeIcons, idViewDetails, appViewMode, 0)
	} else {
		for pos := 5; pos >= 2; pos-- {
			procRemoveMenu.Call(hViewMenu, uintptr(pos), mfByPosition)
		}
	}
	appViewItemsShown = show
}

// updateProcViewMenu appends or removes the Processes-only "Select Columns..."
// item (separator + item at positions 2-3 after "Refresh Now" and "Update Speed").
func updateProcViewMenu(show bool) {
	if hViewMenu == 0 || show == procViewItemsShown {
		return
	}
	if show {
		procAppendMenu.Call(hViewMenu, mfSeparator, 0, 0)
		t, _ := syscall.UTF16PtrFromString("Select Columns...")
		procAppendMenu.Call(hViewMenu, mfString, idViewSelectCols, uintptr(unsafe.Pointer(t)))
	} else {
		for pos := 3; pos >= 2; pos-- {
			procRemoveMenu.Call(hViewMenu, uintptr(pos), mfByPosition)
		}
	}
	procViewItemsShown = show
}

// updatePerfViewMenu appends or removes the Performance-only items (separator,
// CPU History submenu, Show Kernel Times) at positions 2-4.
func updatePerfViewMenu(show bool) {
	if hViewMenu == 0 || show == perfViewItemsShown {
		return
	}
	if show {
		if hCPUHistMenu == 0 {
			hCPUHistMenu, _, _ = procCreatePopupMenu.Call()
			if numCPUs > 1 {
				t, _ := syscall.UTF16PtrFromString("One Graph, All CPUs")
				procAppendMenu.Call(hCPUHistMenu, mfString, idViewCPUHistAll, uintptr(unsafe.Pointer(t)))
			}
			t, _ := syscall.UTF16PtrFromString("One Graph Per CPU")
			procAppendMenu.Call(hCPUHistMenu, mfString, idViewCPUHistPer, uintptr(unsafe.Pointer(t)))
		}
		procAppendMenu.Call(hViewMenu, mfSeparator, 0, 0)
		histLabel, _ := syscall.UTF16PtrFromString("CPU History")
		procAppendMenu.Call(hViewMenu, mfPopup, hCPUHistMenu, uintptr(unsafe.Pointer(histLabel)))
		kernelLabel, _ := syscall.UTF16PtrFromString("Show Kernel Times")
		procAppendMenu.Call(hViewMenu, mfString, idViewKernelTime, uintptr(unsafe.Pointer(kernelLabel)))
		syncPerfViewMenuChecks()
	} else {
		for pos := 4; pos >= 2; pos-- {
			procRemoveMenu.Call(hViewMenu, uintptr(pos), mfByPosition)
		}
	}
	perfViewItemsShown = show
}

// syncPerfViewMenuChecks moves the CPU History radio and Show Kernel Times check.
func syncPerfViewMenuChecks() {
	if hCPUHistMenu == 0 {
		return
	}
	sel := uintptr(idViewCPUHistAll)
	if cpuHistoryPerCPU {
		sel = idViewCPUHistPer
	}
	procCheckMenuRadioItem.Call(hCPUHistMenu, idViewCPUHistAll, idViewCPUHistPer, sel, 0)
	flags := uintptr(0)
	if showKernelTimes {
		flags = mfChecked
	}
	procCheckMenuItem.Call(hViewMenu, idViewKernelTime, flags)
}

// setAppViewMode switches the Applications list between icon/small icon/report view.
func setAppViewMode(id uintptr) {
	if hwndAppList == 0 {
		return
	}
	style := uintptr(lvsIcon)
	switch id {
	case idViewSmallIcons:
		style = lvsSmallIcon
	case idViewDetails:
		style = lvsReport
	}
	if id == idViewLargeIcons {
		// Large icon view draws from LVSIL_NORMAL (0), so mirror the small image list there.
		procSendMessage.Call(uintptr(hwndAppList), lvmSetImageList, 0, uintptr(appImageList))
	}
	gwlStyle := ^uintptr(15) // GWL_STYLE (-16)
	cur, _, _ := procGetWindowLongW.Call(uintptr(hwndAppList), gwlStyle)
	procSetWindowLongW.Call(uintptr(hwndAppList), gwlStyle, (cur&^uintptr(lvsTypeMask))|style)
	appViewMode = id
	if hMainMenu != 0 {
		procCheckMenuRadioItem.Call(hMainMenu, idViewLargeIcons, idViewDetails, id, 0)
	}
	refreshAppList()
}

// updateWindowsMenu inserts or removes the Applications-only Windows menu at bar
// position 3 (between View and Help); RemoveMenu keeps the popup alive for reuse.
func updateWindowsMenu(hwnd syscall.Handle, show bool) {
	if hMainMenu == 0 || show == windowsMenuShown {
		return
	}
	if show {
		label, _ := syscall.UTF16PtrFromString("Windows")
		procInsertMenu.Call(hMainMenu, 3, mfByPosition|mfPopup, hWindowsMenu, uintptr(unsafe.Pointer(label)))
	} else {
		procRemoveMenu.Call(hMainMenu, 3, mfByPosition)
	}
	windowsMenuShown = show
	procDrawMenuBar.Call(uintptr(hwnd))
}

// updateWindowsMenuEnables grays the items based on how many tasks are selected:
// tile/cascade need at least two windows, the rest need one.
func updateWindowsMenuEnables() {
	n := len(selectedTaskHwnds())
	enable := func(id uintptr, on bool) {
		flags := uintptr(mfGrayed)
		if on {
			flags = 0 // MF_ENABLED
		}
		procEnableMenuItem.Call(hWindowsMenu, id, flags) // MF_BYCOMMAND
	}
	enable(idWinTileHorz, n >= 2)
	enable(idWinTileVert, n >= 2)
	enable(idWinCascade, n >= 2)
	enable(idWinMinimize, n >= 1)
	enable(idWinMaximize, n >= 1)
	enable(idWinBringFront, n >= 1)
}

func runWindowsMenuAction(id uintptr) {
	hwnds := selectedTaskHwnds()
	if len(hwnds) == 0 {
		return
	}
	switch id {
	case idWinTileHorz, idWinTileVert:
		flag := uintptr(mdiTileHorz)
		if id == idWinTileVert {
			flag = mdiTileVert
		}
		procTileWindows.Call(0, flag, 0, uintptr(len(hwnds)), uintptr(unsafe.Pointer(&hwnds[0])))
	case idWinCascade:
		procCascadeWindows.Call(0, 0, 0, uintptr(len(hwnds)), uintptr(unsafe.Pointer(&hwnds[0])))
	case idWinMinimize:
		for _, h := range hwnds {
			procShowWindow.Call(uintptr(h), swMinimize)
		}
	case idWinMaximize:
		for _, h := range hwnds {
			procShowWindow.Call(uintptr(h), swMaximize)
		}
	case idWinBringFront:
		for _, h := range hwnds {
			procBringWindowToTop.Call(uintptr(h))
		}
	}
}

// layoutChildren positions the tab control to fill the client area above the status bar.
func layoutChildren(hwnd syscall.Handle) {
	if hwndTab == 0 || hwndStatus == 0 {
		return
	}

	procSendMessage.Call(uintptr(hwndStatus), wmSize, 0, 0)

	var client rect
	procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&client)))

	var statusRect rect
	procGetWindowRect.Call(uintptr(hwndStatus), uintptr(unsafe.Pointer(&statusRect)))
	statusHeight := statusRect.bottom - statusRect.top

	tabWidth := client.right - client.left - 2*tabPadding
	tabHeight := client.bottom - client.top - statusHeight - 2*tabPadding
	procMoveWindow.Call(uintptr(hwndTab), tabPadding, tabPadding, uintptr(tabWidth), uintptr(tabHeight), 1)

	clientW := client.right - client.left
	parts := [3]int32{clientW / 4, clientW / 2, -1}
	procSendMessage.Call(uintptr(hwndStatus), sbSetParts, uintptr(len(parts)), uintptr(unsafe.Pointer(&parts[0])))

	layoutAppList()
	layoutProcList()
	layoutPerfView()
	layoutNetView()
}

// updateStatusBar fills the Processes / CPU Usage / Commit Charge panes from the
// perf data gathered by refreshPerfData.
func updateStatusBar() {
	if hwndStatus == 0 {
		return
	}
	setPart := func(i int, text string) {
		t, _ := syscall.UTF16PtrFromString(text)
		procSendMessage.Call(uintptr(hwndStatus), sbSetTextW, uintptr(i), uintptr(unsafe.Pointer(t)))
	}
	pageKB := uint64(perfStats.pageSize) / 1024
	setPart(0, fmt.Sprintf("Processes: %d", perfStats.processCount))
	setPart(1, fmt.Sprintf("CPU Usage: %d%%", currentCPUUsage))
	setPart(2, fmt.Sprintf("Commit Charge: %dK / %dK", uint64(perfStats.commitTotal)*pageKB, uint64(perfStats.commitLimit)*pageKB))
}

// createMessageFont builds the current system UI font (e.g. Segoe UI) so controls
// don't fall back to the legacy stock bitmap font.
func createMessageFont() uintptr {
	var ncm nonClientMetrics
	ncm.cbSize = uint32(unsafe.Sizeof(ncm))
	ok, _, _ := procSystemParametersInfo.Call(spiGetNonClientMetrics, uintptr(ncm.cbSize), uintptr(unsafe.Pointer(&ncm)), 0)
	if ok == 0 {
		return 0
	}
	font, _, _ := procCreateFontIndirect.Call(uintptr(unsafe.Pointer(&ncm.lfMessageFont)))
	return font
}

// createMenuBar builds the File/Options/View/Help menu bar and attaches it to hwnd.
func createMenuBar(hwnd syscall.Handle) {
	type menuItem struct {
		id   uintptr
		text string
	}

	hMenuBar, _, _ := procCreateMenu.Call()

	addPopup := func(label string, items ...menuItem) uintptr {
		hPopup, _, _ := procCreatePopupMenu.Call()
		for _, item := range items {
			if item.text == "-" {
				procAppendMenu.Call(hPopup, mfSeparator, 0, 0)
				continue
			}
			itemText, _ := syscall.UTF16PtrFromString(item.text)
			procAppendMenu.Call(hPopup, mfString, item.id, uintptr(unsafe.Pointer(itemText)))
		}
		labelText, _ := syscall.UTF16PtrFromString(label)
		procAppendMenu.Call(hMenuBar, mfPopup, hPopup, uintptr(unsafe.Pointer(labelText)))
		return hPopup
	}

	addPopup("File",
		menuItem{idNewTask, "New Task (Run...)"},
		menuItem{0, "-"},
		menuItem{idFileExit, "Exit Task Manager 98"})
	addPopup("Options",
		menuItem{idOptAlwaysOnTop, "Always On Top"},
		menuItem{idOptMinimizeUse, "Minimize On Use"},
		menuItem{idOptHideWhenMin, "Hide When Minimized"})
	viewPopup := addPopup("View", menuItem{idViewRefresh, "Refresh Now"})
	speedPopup, _, _ := procCreatePopupMenu.Call()
	for _, item := range []menuItem{
		{idViewSpeedHigh, "High"},
		{idViewSpeedNorm, "Normal"},
		{idViewSpeedLow, "Low"},
		{idViewSpeedPause, "Paused"},
	} {
		itemText, _ := syscall.UTF16PtrFromString(item.text)
		procAppendMenu.Call(speedPopup, mfString, item.id, uintptr(unsafe.Pointer(itemText)))
	}
	speedLabel, _ := syscall.UTF16PtrFromString("Update Speed")
	procAppendMenu.Call(viewPopup, mfPopup, speedPopup, uintptr(unsafe.Pointer(speedLabel)))
	hViewMenu = viewPopup

	addPopup("Help",
		menuItem{idHelpLink, "Task Manager 98 Help Topics"},
		menuItem{0, "-"},
		menuItem{idHelpAbout, "About Task Manager 98"})

	winPopup, _, _ := procCreatePopupMenu.Call()
	for _, item := range []menuItem{
		{idWinTileHorz, "Tile Horizontally"},
		{idWinTileVert, "Tile Vertically"},
		{idWinMinimize, "Minimize"},
		{idWinMaximize, "Maximize"},
		{idWinCascade, "Cascade"},
		{idWinBringFront, "Bring To Front"},
	} {
		itemText, _ := syscall.UTF16PtrFromString(item.text)
		procAppendMenu.Call(winPopup, mfString, item.id, uintptr(unsafe.Pointer(itemText)))
	}
	hWindowsMenu = winPopup

	hMainMenu = hMenuBar
	procCheckMenuRadioItem.Call(hMainMenu, idViewSpeedHigh, idViewSpeedPause, idViewSpeedNorm, 0)
	updateAppViewMenu(true)       // Applications is the startup tab
	updateWindowsMenu(hwnd, true) // ditto
	procSetMenu.Call(uintptr(hwnd), hMenuBar)
}

// updateOptionsMenuChecks syncs the Options menu check marks with the current state.
func updateOptionsMenuChecks() {
	if hMainMenu == 0 {
		return
	}
	check := func(id uintptr, on bool) {
		flags := uintptr(0) // MF_BYCOMMAND | MF_UNCHECKED
		if on {
			flags = mfChecked
		}
		procCheckMenuItem.Call(hMainMenu, id, flags)
	}
	check(idOptAlwaysOnTop, alwaysOnTop)
	check(idOptMinimizeUse, minimizeOnUse)
	check(idOptHideWhenMin, hideWhenMinimized)
}

// createTabControl creates the center tab view with the given tab labels.
func createTabControl(hwnd syscall.Handle, hInstance uintptr) syscall.Handle {
	className, _ := syscall.UTF16PtrFromString("SysTabControl32")
	h, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		uintptr(wsChild|wsVisible),
		0, 0, 0, 0,
		uintptr(hwnd), idTab, hInstance, 0,
	)
	tab := syscall.Handle(h)

	for i, label := range []string{"Applications", "Processes", "Performance", "Networking"} {
		text, _ := syscall.UTF16PtrFromString(label)
		item := tcItemW{mask: tcifText, pszText: text}
		procSendMessage.Call(uintptr(tab), tcmInsertItem, uintptr(i), uintptr(unsafe.Pointer(&item)))
	}

	return tab
}

// createStatusBar creates the bottom status bar.
func createStatusBar(hwnd syscall.Handle, hInstance uintptr) syscall.Handle {
	className, _ := syscall.UTF16PtrFromString("msctls_statusbar32")
	text, _ := syscall.UTF16PtrFromString("Ready")
	h, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(text)),
		uintptr(wsChild|wsVisible|sbarsSizeGrip),
		0, 0, 0, 0,
		uintptr(hwnd), idStatus, hInstance, 0,
	)
	return syscall.Handle(h)
}

// logCrash writes a panic and its stack trace next to the exe; the release build runs
// with -H=windowsgui, so there's no console to see a panic message otherwise.
func logCrash(r interface{}) {
	dir := "."
	if exePath, err := os.Executable(); err == nil {
		dir = filepath.Dir(exePath)
	}
	f, err := os.Create(filepath.Join(dir, "TaskMgr98-crash.log"))
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "panic: %v\n\n%s", r, debug.Stack())
}

// loadEmbeddedIcon loads the icon embedded via rsrc.syso (see build.ps1/build.sh);
// rsrc assigns the icon group resource ID 1 when no manifest is passed to it.
func loadEmbeddedIcon(hInstance uintptr) syscall.Handle {
	h, _, _ := procLoadIcon.Call(hInstance, uintptr(1))
	return syscall.Handle(h)
}

// loadTaskManagerIcon extracts the real Task Manager icon out of system32\taskmgr.exe,
// so the window/taskbar show it instead of the generic stock application icon.
func loadTaskManagerIcon() syscall.Handle {
	buf := make([]uint16, 260)
	n, _, _ := procGetSystemDirectory.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || n >= uintptr(len(buf)) {
		return 0
	}
	exePath, err := syscall.UTF16PtrFromString(syscall.UTF16ToString(buf[:n]) + "\\taskmgr.exe")
	if err != nil {
		return 0
	}
	h, _, _ := procExtractIcon.Call(0, uintptr(unsafe.Pointer(exePath)), 0)
	// ExtractIconW returns NULL (no icons) or -1 (file not found/invalid); either way fall back.
	if h == 0 || uint32(h) == 0xFFFFFFFF {
		return 0
	}
	return syscall.Handle(h)
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			logCrash(r)
			panic(r)
		}
	}()

	// A window's message queue is bound to the OS thread that created it; without this
	// the Go scheduler can migrate the goroutine to another thread and GetMessage/DispatchMessage
	// stop delivering messages, making the window appear frozen.
	runtime.LockOSThread()

	hInstance, _, _ := procGetModuleHandle.Call(0)

	icc := initCommonControlsEx{dwICC: iccTabClasses | iccBarClasses | iccListViewClasses}
	icc.dwSize = uint32(unsafe.Sizeof(icc))
	procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))

	className, _ := syscall.UTF16PtrFromString("TaskMgr98WindowClass")
	title, _ := syscall.UTF16PtrFromString("Task Manager 98")

	// IDC_ARROW cursor
	cursor, _, _ := procLoadCursor.Call(0, uintptr(32512))
	// Prefer the icon embedded via rsrc.syso, then the real Task Manager icon, then the stock icon.
	icon := uintptr(loadEmbeddedIcon(hInstance))
	if icon == 0 {
		icon = uintptr(loadTaskManagerIcon())
	}
	if icon == 0 {
		icon, _, _ = procLoadIcon.Call(0, uintptr(32512))
	}

	wc := wndClassEx{
		style:         0,
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     syscall.Handle(hInstance),
		hIcon:         syscall.Handle(icon),
		hCursor:       syscall.Handle(cursor),
		hbrBackground: syscall.Handle(colorBtnFace + 1),
		lpszClassName: className,
		hIconSm:       syscall.Handle(icon),
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))

	if ret, _, _ := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); ret == 0 {
		panic("RegisterClassEx failed")
	}

	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(wsOverlappedWindow|wsVisible),
		cwUseDefault, cwUseDefault, 480, 600,
		0, 0,
		hInstance,
		0,
	)
	if hwnd == 0 {
		panic("CreateWindowEx failed")
	}

	createMenuBar(syscall.Handle(hwnd))
	appMainHwnd = syscall.Handle(hwnd)
	hwndTab = createTabControl(syscall.Handle(hwnd), hInstance)
	hwndAppList = createAppListView(syscall.Handle(hwnd), hInstance)
	createAppListButtons(syscall.Handle(hwnd), hInstance)
	hwndProcList = createProcListView(syscall.Handle(hwnd), hInstance)
	createProcListButtons(syscall.Handle(hwnd), hInstance)
	createPerfView(syscall.Handle(hwnd), hInstance)
	createNetView(syscall.Handle(hwnd), hInstance)
	hwndStatus = createStatusBar(syscall.Handle(hwnd), hInstance)
	if font := createMessageFont(); font != 0 {
		procSendMessage.Call(uintptr(hwndTab), wmSetFont, font, 1)
		procSendMessage.Call(uintptr(hwndAppList), wmSetFont, font, 1)
		procSendMessage.Call(uintptr(hwndEndTask), wmSetFont, font, 1)
		procSendMessage.Call(uintptr(hwndSwitchTo), wmSetFont, font, 1)
		procSendMessage.Call(uintptr(hwndNewTask), wmSetFont, font, 1)
		procSendMessage.Call(uintptr(hwndProcList), wmSetFont, font, 1)
		procSendMessage.Call(uintptr(hwndEndProcess), wmSetFont, font, 1)
		procSendMessage.Call(uintptr(hwndShowAllUsers), wmSetFont, font, 1)
		setPerfFonts(font)
		setNetFonts(font)
		procSendMessage.Call(uintptr(hwndStatus), wmSetFont, font, 1)
	}
	layoutChildren(syscall.Handle(hwnd))
	refreshAppList()
	refreshPerfData()
	updateStatusBar()
	updateTrayIcon()

	procShowWindow.Call(hwnd, swShowDefault)
	procUpdateWindow.Call(hwnd)
	procSetTimer.Call(hwnd, idAppListTimer, timerIntervalMs, 0)

	var m msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}
