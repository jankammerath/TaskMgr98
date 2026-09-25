package main

import (
	"syscall"
	"unsafe"
)

const (
	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	cwUseDefault       = 0x80000000
	swShowDefault      = 10
	wmDestroy          = 0x0002
	colorWindow        = 5 // COLOR_WINDOW
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

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procGetMessage       = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procLoadCursor       = user32.NewProc("LoadCursorW")
	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
)

// wndProc handles window messages; WM_DESTROY quits the message loop, ending the process.
func wndProc(hwnd syscall.Handle, message uint32, wParam, lParam uintptr) uintptr {
	if message == wmDestroy {
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return ret
}

func main() {
	hInstance, _, _ := procGetModuleHandle.Call(0)

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