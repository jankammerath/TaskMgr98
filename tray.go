package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	nimAdd    = 0
	nimModify = 1
	nimDelete = 2

	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04

	wmTrayCallback  = 0x8001 // WM_APP + 1
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205

	mfChecked = 0x00000008

	tpmRightButton = 0x0002

	swpNoSize = 0x0001
	swpNoMove = 0x0002

	idTrayRestore = 1101
	idTrayClose   = 1102
	idTrayTopmost = 1103
)

// notifyIconData mirrors NOTIFYICONDATAW (x64 layout).
type notifyIconData struct {
	cbSize           uint32
	_                uint32
	hWnd             syscall.Handle
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	_                uint32
	hIcon            syscall.Handle
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     syscall.Handle
}

var (
	procShellNotifyIcon    = shell32.NewProc("Shell_NotifyIconW")
	procDestroyIcon        = user32.NewProc("DestroyIcon")
	procGetCursorPos       = user32.NewProc("GetCursorPos")
	procTrackPopupMenu     = user32.NewProc("TrackPopupMenu")
	procDestroyMenu        = user32.NewProc("DestroyMenu")
	procSetMenuDefaultItem = user32.NewProc("SetMenuDefaultItem")
	procIsIconic           = user32.NewProc("IsIconic")
	procSetWindowPos       = user32.NewProc("SetWindowPos")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procCreateBitmap       = gdi32.NewProc("CreateBitmap")

	trayIconAdded bool
	trayIcon      syscall.Handle
	alwaysOnTop   bool
)

// makeCPUTrayIcon renders a 16x16 LED-style bar of the current CPU usage.
func makeCPUTrayIcon(percent int) syscall.Handle {
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return 0
	}
	defer procReleaseDC.Call(0, screenDC)

	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	colorBmp, _, _ := procCreateCompatibleBitmap.Call(screenDC, 16, 16)
	oldBmp, _, _ := procSelectObject.Call(memDC, colorBmp)

	full := rect{left: 0, top: 0, right: 16, bottom: 16}
	blackBrush, _, _ := procCreateSolidBrush.Call(0x00000000)
	procFillRect.Call(memDC, uintptr(unsafe.Pointer(&full)), blackBrush)
	procDeleteObject.Call(blackBrush)

	barH := int32(min(max(percent, 0), 100)) * 16 / 100
	if percent > 0 && barH == 0 {
		barH = 1
	}
	if barH > 0 {
		greenBrush, _, _ := procCreateSolidBrush.Call(0x0000FF00)
		bar := rect{left: 0, top: 16 - barH, right: 16, bottom: 16}
		procFillRect.Call(memDC, uintptr(unsafe.Pointer(&bar)), greenBrush)
		procDeleteObject.Call(greenBrush)
	}

	// Dark green grid over the whole icon, splitting the bar into LED cells.
	gridPen, _, _ := procCreatePen.Call(psSolid, 1, 0x00006600)
	oldPen, _, _ := procSelectObject.Call(memDC, gridPen)
	for x := int32(0); x < 16; x += 4 {
		procMoveToEx.Call(memDC, uintptr(x), 0, 0)
		procLineTo.Call(memDC, uintptr(x), 16)
	}
	for y := int32(3); y < 16; y += 4 {
		procMoveToEx.Call(memDC, 0, uintptr(y), 0)
		procLineTo.Call(memDC, 16, uintptr(y))
	}
	procSelectObject.Call(memDC, oldPen)
	procDeleteObject.Call(gridPen)

	procSelectObject.Call(memDC, oldBmp)
	procDeleteDC.Call(memDC)

	// All-zero AND mask = fully opaque icon.
	var maskBits [32]byte
	maskBmp, _, _ := procCreateBitmap.Call(16, 16, 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))

	ii := iconInfo{fIcon: 1, hbmMask: syscall.Handle(maskBmp), hbmColor: syscall.Handle(colorBmp)}
	icon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))

	procDeleteObject.Call(colorBmp)
	procDeleteObject.Call(maskBmp)
	return syscall.Handle(icon)
}

// updateTrayIcon adds or refreshes the tray icon and its CPU usage tooltip.
func updateTrayIcon() {
	if appMainHwnd == 0 {
		return
	}
	icon := makeCPUTrayIcon(currentCPUUsage)
	if icon == 0 {
		return
	}

	nid := notifyIconData{
		hWnd:             appMainHwnd,
		uID:              1,
		uFlags:           nifMessage | nifIcon | nifTip,
		uCallbackMessage: wmTrayCallback,
		hIcon:            icon,
	}
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	tip, _ := syscall.UTF16FromString(fmt.Sprintf("CPU Usage: %d%%", currentCPUUsage))
	copy(nid.szTip[:], tip)

	cmd := uintptr(nimModify)
	if !trayIconAdded {
		cmd = nimAdd
	}
	if ok, _, _ := procShellNotifyIcon.Call(cmd, uintptr(unsafe.Pointer(&nid))); ok != 0 {
		trayIconAdded = true
	}

	if trayIcon != 0 {
		procDestroyIcon.Call(uintptr(trayIcon))
	}
	trayIcon = icon
}

func removeTrayIcon() {
	if trayIconAdded {
		nid := notifyIconData{hWnd: appMainHwnd, uID: 1}
		nid.cbSize = uint32(unsafe.Sizeof(nid))
		procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
		trayIconAdded = false
	}
	if trayIcon != 0 {
		procDestroyIcon.Call(uintptr(trayIcon))
		trayIcon = 0
	}
}

func restoreMainWindow() {
	procShowWindow.Call(uintptr(appMainHwnd), swRestore)
	procSetForegroundWindow.Call(uintptr(appMainHwnd))
}

func toggleAlwaysOnTop() {
	alwaysOnTop = !alwaysOnTop
	insertAfter := ^uintptr(1) // HWND_NOTOPMOST (-2)
	if alwaysOnTop {
		insertAfter = ^uintptr(0) // HWND_TOPMOST (-1)
	}
	procSetWindowPos.Call(uintptr(appMainHwnd), insertAfter, 0, 0, 0, 0, swpNoMove|swpNoSize)
}

// showTrayMenu pops up the tray context menu at the cursor; the selection arrives
// as WM_COMMAND in wndProc.
func showTrayMenu(hwnd syscall.Handle) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}

	appendItem := func(flags uintptr, id uintptr, text string) {
		t, _ := syscall.UTF16PtrFromString(text)
		procAppendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(t)))
	}

	iconic, _, _ := procIsIconic.Call(uintptr(hwnd))
	visible, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
	if iconic != 0 || visible == 0 {
		appendItem(mfString, idTrayRestore, "Restore")
		procSetMenuDefaultItem.Call(menu, idTrayRestore, 0)
	}
	appendItem(mfString, idTrayClose, "Close")
	procAppendMenu.Call(menu, mfSeparator, 0, 0)
	topFlags := uintptr(mfString)
	if alwaysOnTop {
		topFlags |= mfChecked
	}
	appendItem(topFlags, idTrayTopmost, "Always on Top")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	// Foreground focus is required so the menu closes when clicking elsewhere.
	procSetForegroundWindow.Call(uintptr(hwnd))
	procTrackPopupMenu.Call(menu, tpmRightButton, uintptr(pt.x), uintptr(pt.y), 0, uintptr(hwnd), 0)
	procPostMessage.Call(uintptr(hwnd), 0, 0, 0) // WM_NULL, per TrackPopupMenu docs
	procDestroyMenu.Call(menu)
}
