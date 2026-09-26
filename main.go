package main

import (
	"runtime"
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
	colorWindow        = 5 // COLOR_WINDOW

	mfString = 0x00000000
	mfPopup  = 0x00000010

	iccTabClasses = 0x00000008
	iccBarClasses = 0x00000004

	tcmFirst      = 0x1300
	tcmInsertItem = tcmFirst + 62 // TCM_INSERTITEMW
	tcifText      = 0x0001

	sbarsSizeGrip = 0x0100

	wmSetFont              = 0x0030
	spiGetNonClientMetrics = 0x0029

	tabPadding = 6

	idTab       = 100
	idStatus    = 101
	idFileExit  = 1001
	idHelpAbout = 1002
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
	gdi32                    = syscall.NewLazyDLL("gdi32.dll")
	procCreateFontIndirect   = gdi32.NewProc("CreateFontIndirectW")
)

// hwndTab and hwndStatus are set once in main and read by wndProc for layout.
var (
	hwndTab    syscall.Handle
	hwndStatus syscall.Handle
)

// wndProc handles window messages; WM_DESTROY quits the message loop, ending the process.
func wndProc(hwnd syscall.Handle, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	case wmSize:
		layoutChildren(hwnd)
		return 0
	case wmCommand:
		switch wParam & 0xFFFF {
		case idFileExit:
			procDestroyWindow.Call(uintptr(hwnd))
			return 0
		case idHelpAbout:
			text, _ := syscall.UTF16PtrFromString("WinGo")
			caption, _ := syscall.UTF16PtrFromString("About")
			procMessageBox.Call(uintptr(hwnd), uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(caption)), 0)
			return 0
		}
	}
	ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return ret
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

	addPopup := func(label string, items ...menuItem) {
		hPopup, _, _ := procCreatePopupMenu.Call()
		for _, item := range items {
			itemText, _ := syscall.UTF16PtrFromString(item.text)
			procAppendMenu.Call(hPopup, mfString, item.id, uintptr(unsafe.Pointer(itemText)))
		}
		labelText, _ := syscall.UTF16PtrFromString(label)
		procAppendMenu.Call(hMenuBar, mfPopup, hPopup, uintptr(unsafe.Pointer(labelText)))
	}

	addPopup("File", menuItem{idFileExit, "Exit"})
	addPopup("Options")
	addPopup("View")
	addPopup("Help", menuItem{idHelpAbout, "About"})

	procSetMenu.Call(uintptr(hwnd), hMenuBar)
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

	for i, label := range []string{"Applications", "Processes", "Performance", "Networking", "Users"} {
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

func main() {
	// A window's message queue is bound to the OS thread that created it; without this
	// the Go scheduler can migrate the goroutine to another thread and GetMessage/DispatchMessage
	// stop delivering messages, making the window appear frozen.
	runtime.LockOSThread()

	hInstance, _, _ := procGetModuleHandle.Call(0)

	icc := initCommonControlsEx{dwICC: iccTabClasses | iccBarClasses}
	icc.dwSize = uint32(unsafe.Sizeof(icc))
	procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))

	className, _ := syscall.UTF16PtrFromString("WinGoWindowClass")
	title, _ := syscall.UTF16PtrFromString("WinGo")

	// IDC_ARROW cursor
	cursor, _, _ := procLoadCursor.Call(0, uintptr(32512))

	wc := wndClassEx{
		style:         0,
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     syscall.Handle(hInstance),
		hCursor:       syscall.Handle(cursor),
		hbrBackground: syscall.Handle(colorWindow + 1),
		lpszClassName: className,
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
		cwUseDefault, cwUseDefault, 640, 480,
		0, 0,
		hInstance,
		0,
	)
	if hwnd == 0 {
		panic("CreateWindowEx failed")
	}

	createMenuBar(syscall.Handle(hwnd))
	hwndTab = createTabControl(syscall.Handle(hwnd), hInstance)
	hwndStatus = createStatusBar(syscall.Handle(hwnd), hInstance)
	if font := createMessageFont(); font != 0 {
		procSendMessage.Call(uintptr(hwndTab), wmSetFont, font, 1)
		procSendMessage.Call(uintptr(hwndStatus), wmSetFont, font, 1)
	}
	layoutChildren(syscall.Handle(hwnd))

	procShowWindow.Call(hwnd, swShowDefault)
	procUpdateWindow.Call(hwnd)

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
