package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	wsPopup   = 0x80000000
	wsCaption = 0x00C00000
	wsSysMenu = 0x00080000

	wsExDlgModalFrame = 0x00000001

	ssIcon       = 0x00000003
	ssEtchedHorz = 0x00000010
	stmSetIcon   = 0x0170

	idAboutOK = 1
)

var (
	procEnableWindow = user32.NewProc("EnableWindow")

	aboutClassRegistered = false
	hwndAbout            syscall.Handle
	aboutOwner           syscall.Handle
)

func registerAboutClass(hInstance uintptr) {
	if aboutClassRegistered {
		return
	}
	className, _ := syscall.UTF16PtrFromString("TaskMgr98About")
	cursor, _, _ := procLoadCursor.Call(0, uintptr(32512))

	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		lpfnWndProc:   syscall.NewCallback(aboutWndProc),
		hInstance:     syscall.Handle(hInstance),
		hCursor:       syscall.Handle(cursor),
		hbrBackground: syscall.Handle(colorBtnFace + 1),
		lpszClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	aboutClassRegistered = true
}

// showAboutDialog opens the About box as an owner-disabled (modal-like) popup.
func showAboutDialog(owner syscall.Handle) {
	if hwndAbout != 0 {
		procSetForegroundWindow.Call(uintptr(hwndAbout))
		return
	}
	hInstance, _, _ := procGetModuleHandle.Call(0)
	registerAboutClass(hInstance)

	const dlgW, dlgH = 420, 240

	// Center over the owner window.
	var ownerRect rect
	procGetWindowRect.Call(uintptr(owner), uintptr(unsafe.Pointer(&ownerRect)))
	x := ownerRect.left + (ownerRect.right-ownerRect.left-dlgW)/2
	y := ownerRect.top + (ownerRect.bottom-ownerRect.top-dlgH)/2

	className, _ := syscall.UTF16PtrFromString("TaskMgr98About")
	title, _ := syscall.UTF16PtrFromString("About Task Manager 98")
	h, _, _ := procCreateWindowEx.Call(
		wsExDlgModalFrame,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(wsPopup|wsCaption|wsSysMenu),
		uintptr(x), uintptr(y), dlgW, dlgH,
		uintptr(owner), 0, hInstance, 0,
	)
	if h == 0 {
		return
	}
	hwndAbout = syscall.Handle(h)
	aboutOwner = owner

	makeStatic := func(text string, style, x, y, w, hgt uintptr) syscall.Handle {
		stClass, _ := syscall.UTF16PtrFromString("STATIC")
		tPtr, _ := syscall.UTF16PtrFromString(text)
		sh, _, _ := procCreateWindowEx.Call(
			0,
			uintptr(unsafe.Pointer(stClass)),
			uintptr(unsafe.Pointer(tPtr)),
			uintptr(wsChild|wsVisible)|style,
			x, y, w, hgt,
			uintptr(hwndAbout), 0, hInstance, 0,
		)
		return syscall.Handle(sh)
	}

	// App icon at top-left, same lookup order as the main window.
	icon := uintptr(loadEmbeddedIcon(hInstance))
	if icon == 0 {
		icon = uintptr(loadTaskManagerIcon())
	}
	if icon == 0 {
		icon, _, _ = procLoadIcon.Call(0, uintptr(32512))
	}
	iconStatic := makeStatic("", ssIcon, 20, 20, 32, 32)
	procSendMessage.Call(uintptr(iconStatic), stmSetIcon, icon, 0)

	makeStatic("Task Manager 98", 0, 72, 20, 320, 18)
	makeStatic("Version 1.2026.10.1", 0, 72, 40, 320, 18)
	makeStatic("Copyright © Jan Kamerath 1985-2026", 0, 72, 60, 320, 18)

	makeStatic("", ssEtchedHorz, 72, 118, dlgW-72-20, 2)

	physKB := uint64(perfStats.physicalTotal) * uint64(perfStats.pageSize) / 1024
	makeStatic(fmt.Sprintf("Physical memory available to Windows:   %sB", formatKB(physKB)), 0, 72, 132, 330, 18)

	btnClass, _ := syscall.UTF16PtrFromString("BUTTON")
	okText, _ := syscall.UTF16PtrFromString("OK")
	ok, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(btnClass)),
		uintptr(unsafe.Pointer(okText)),
		uintptr(wsChild|wsVisible|wsTabStop|bsPushButton),
		dlgW-80-16, dlgH-32, 80, 24,
		uintptr(hwndAbout), idAboutOK, hInstance, 0,
	)

	if font := createMessageFont(); font != 0 {
		for _, c := range []uintptr{uintptr(iconStatic), ok} {
			procSendMessage.Call(c, wmSetFont, font, 1)
		}
		procEnumChildWindows.Call(uintptr(hwndAbout), perfFontCallback, font)
	}

	procEnableWindow.Call(uintptr(owner), 0)
	procShowWindow.Call(uintptr(hwndAbout), swShowDefault)
	procUpdateWindow.Call(uintptr(hwndAbout))
}

func closeAboutDialog() {
	if hwndAbout != 0 {
		procDestroyWindow.Call(uintptr(hwndAbout))
	}
}

// aboutWndProc handles the About box; recover locally since a panic can't unwind
// across the native DispatchMessage frame (see wndProc).
func aboutWndProc(hwnd syscall.Handle, message uint32, wParam, lParam uintptr) (result uintptr) {
	defer func() {
		if r := recover(); r != nil {
			logCrash(r)
			ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
			result = ret
		}
	}()
	switch message {
	case wmCommand:
		if wParam&0xFFFF == idAboutOK {
			closeAboutDialog()
			return 0
		}
	case wmClose:
		closeAboutDialog()
		return 0
	case wmDestroy:
		// Re-enable the owner before the dialog vanishes so focus returns to it.
		procEnableWindow.Call(uintptr(aboutOwner), 1)
		procSetForegroundWindow.Call(uintptr(aboutOwner))
		hwndAbout = 0
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return ret
}
