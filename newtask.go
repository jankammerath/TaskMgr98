package main

import (
	"syscall"
	"unsafe"
)

const (
	idNewTaskOK     = 1 // IDOK, so IsDialogMessage's Enter handling maps here
	idNewTaskCancel = 2 // IDCANCEL, so Esc closes the dialog
	idNewTaskBrowse = 3

	esAutoHScroll = 0x0080
)

var (
	procSetFocus         = user32.NewProc("SetFocus")
	procIsDialogMessageW = user32.NewProc("IsDialogMessageW")

	newTaskClassRegistered = false
	hwndNewTaskDlg         syscall.Handle
	hwndNewTaskEdit        syscall.Handle
	newTaskOwner           syscall.Handle
)

func registerNewTaskClass(hInstance uintptr) {
	if newTaskClassRegistered {
		return
	}
	className, _ := syscall.UTF16PtrFromString("TaskMgr98NewTask")
	cursor, _, _ := procLoadCursor.Call(0, uintptr(32512))

	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		lpfnWndProc:   syscall.NewCallback(newTaskWndProc),
		hInstance:     syscall.Handle(hInstance),
		hCursor:       syscall.Handle(cursor),
		hbrBackground: syscall.Handle(colorBtnFace + 1),
		lpszClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	newTaskClassRegistered = true
}

// showNewTaskDialog opens the "Create New Task" run dialog (owner-disabled,
// modal-like); the actual launch happens in launchNewTask on OK.
func showNewTaskDialog(owner syscall.Handle) {
	if hwndNewTaskDlg != 0 {
		procSetForegroundWindow.Call(uintptr(hwndNewTaskDlg))
		return
	}
	hInstance, _, _ := procGetModuleHandle.Call(0)
	registerNewTaskClass(hInstance)

	const dlgW, dlgH = 400, 210

	var ownerRect rect
	procGetWindowRect.Call(uintptr(owner), uintptr(unsafe.Pointer(&ownerRect)))
	x := ownerRect.left + (ownerRect.right-ownerRect.left-dlgW)/2
	y := ownerRect.top + (ownerRect.bottom-ownerRect.top-dlgH)/2

	className, _ := syscall.UTF16PtrFromString("TaskMgr98NewTask")
	title, _ := syscall.UTF16PtrFromString("Create New Task")
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
	hwndNewTaskDlg = syscall.Handle(h)
	newTaskOwner = owner

	// App icon at top-left, same lookup order as the main window.
	icon := uintptr(loadEmbeddedIcon(hInstance))
	if icon == 0 {
		icon = uintptr(loadTaskManagerIcon())
	}
	if icon == 0 {
		icon, _, _ = procLoadIcon.Call(0, uintptr(32512))
	}
	stClass, _ := syscall.UTF16PtrFromString("STATIC")
	iconStatic, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(stClass)),
		0,
		uintptr(wsChild|wsVisible|ssIcon),
		16, 18, 32, 32,
		uintptr(hwndNewTaskDlg), 0, hInstance, 0,
	)
	procSendMessage.Call(iconStatic, stmSetIcon, icon, 0)

	introText, _ := syscall.UTF16PtrFromString("Type the name of a program, folder, document, or Internet resource, and Windows will open it for you.")
	procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(stClass)),
		uintptr(unsafe.Pointer(introText)),
		uintptr(wsChild|wsVisible),
		64, 16, dlgW-64-20, 36,
		uintptr(hwndNewTaskDlg), 0, hInstance, 0,
	)

	openText, _ := syscall.UTF16PtrFromString("Open:")
	procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(stClass)),
		uintptr(unsafe.Pointer(openText)),
		uintptr(wsChild|wsVisible),
		16, 74, 44, 18,
		uintptr(hwndNewTaskDlg), 0, hInstance, 0,
	)

	editClass, _ := syscall.UTF16PtrFromString("EDIT")
	e, _, _ := procCreateWindowEx.Call(
		0x00000200, // WS_EX_CLIENTEDGE
		uintptr(unsafe.Pointer(editClass)),
		0,
		uintptr(wsChild|wsVisible|wsTabStop|esAutoHScroll),
		64, 71, dlgW-64-28, 22,
		uintptr(hwndNewTaskDlg), 0, hInstance, 0,
	)
	hwndNewTaskEdit = syscall.Handle(e)

	btnClass, _ := syscall.UTF16PtrFromString("BUTTON")
	makeButton := func(text string, id uintptr, x int32, style uintptr) {
		t, _ := syscall.UTF16PtrFromString(text)
		procCreateWindowEx.Call(
			0,
			uintptr(unsafe.Pointer(btnClass)),
			uintptr(unsafe.Pointer(t)),
			uintptr(wsChild|wsVisible|wsTabStop)|style,
			uintptr(x), uintptr(dlgH-72), 80, 24,
			uintptr(hwndNewTaskDlg), id, hInstance, 0,
		)
	}
	const bsDefPushButton = 0x00000001
	makeButton("OK", idNewTaskOK, dlgW-24-80-88-88, bsDefPushButton)
	makeButton("Cancel", idNewTaskCancel, dlgW-24-80-88, bsPushButton)
	makeButton("Browse...", idNewTaskBrowse, dlgW-24-80, bsPushButton)

	if font := createMessageFont(); font != 0 {
		procEnumChildWindows.Call(uintptr(hwndNewTaskDlg), perfFontCallback, font)
	}

	procEnableWindow.Call(uintptr(owner), 0)
	procShowWindow.Call(uintptr(hwndNewTaskDlg), swShowDefault)
	procUpdateWindow.Call(uintptr(hwndNewTaskDlg))
	procSetFocus.Call(uintptr(hwndNewTaskEdit))
}

// launchNewTask opens whatever is typed in the edit box (program, folder, or URL).
func launchNewTask() {
	var buf [1024]uint16
	n, _, _ := procGetWindowTextW.Call(uintptr(hwndNewTaskEdit), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return
	}
	target, _ := syscall.UTF16PtrFromString(syscall.UTF16ToString(buf[:n]))
	openVerb, _ := syscall.UTF16PtrFromString("open")
	procShellExecuteW.Call(0, uintptr(unsafe.Pointer(openVerb)), uintptr(unsafe.Pointer(target)), 0, 0, swShowDefault)
	closeNewTaskDialog()
	if minimizeOnUse {
		procShowWindow.Call(uintptr(appMainHwnd), swMinimize)
	}
}

func closeNewTaskDialog() {
	if hwndNewTaskDlg != 0 {
		procDestroyWindow.Call(uintptr(hwndNewTaskDlg))
	}
}

// newTaskWndProc handles the Create New Task dialog; recover locally since a panic
// can't unwind across the native DispatchMessage frame (see wndProc).
func newTaskWndProc(hwnd syscall.Handle, message uint32, wParam, lParam uintptr) (result uintptr) {
	defer func() {
		if r := recover(); r != nil {
			logCrash(r)
			ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
			result = ret
		}
	}()
	switch message {
	case wmCommand:
		switch wParam & 0xFFFF {
		case idNewTaskOK:
			launchNewTask()
			return 0
		case idNewTaskCancel:
			closeNewTaskDialog()
			return 0
		case idNewTaskBrowse:
			if path, ok := pickProgramPath(hwnd); ok {
				t, _ := syscall.UTF16PtrFromString(path)
				procSendMessage.Call(uintptr(hwndNewTaskEdit), 0x000C, 0, uintptr(unsafe.Pointer(t))) // WM_SETTEXT
			}
			return 0
		}
	case wmClose:
		closeNewTaskDialog()
		return 0
	case wmDestroy:
		procEnableWindow.Call(uintptr(newTaskOwner), 1)
		procSetForegroundWindow.Call(uintptr(newTaskOwner))
		hwndNewTaskDlg = 0
		hwndNewTaskEdit = 0
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return ret
}
