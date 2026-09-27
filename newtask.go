package main

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	idNewTaskOK     = 1 // IDOK, so IsDialogMessage's Enter handling maps here
	idNewTaskCancel = 2 // IDCANCEL, so Esc closes the dialog
	idNewTaskBrowse = 3

	cbsDropDown    = 0x0002
	cbsAutoHScroll = 0x0040
	wsVScroll      = 0x00200000
	cbAddString    = 0x0143

	taskMRUSize = 4

	hkeyCurrentUser = 0x80000001
	regSz           = 1
	keyReadWrite    = 0x2001F // KEY_READ | KEY_WRITE
)

var (
	procSetFocus         = user32.NewProc("SetFocus")
	procIsDialogMessageW = user32.NewProc("IsDialogMessageW")

	procRegCreateKeyExW  = advapi32.NewProc("RegCreateKeyExW")
	procRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	procRegCloseKey      = advapi32.NewProc("RegCloseKey")

	newTaskClassRegistered = false
	hwndNewTaskDlg         syscall.Handle
	hwndNewTaskCombo       syscall.Handle
	newTaskOwner           syscall.Handle
)

// openTaskMRUKey opens (creating if needed) HKCU\Software\TaskMgr98.
func openTaskMRUKey() (uintptr, bool) {
	sub, _ := syscall.UTF16PtrFromString(`Software\TaskMgr98`)
	var hKey uintptr
	ret, _, _ := procRegCreateKeyExW.Call(
		hkeyCurrentUser, uintptr(unsafe.Pointer(sub)),
		0, 0, 0, keyReadWrite, 0,
		uintptr(unsafe.Pointer(&hKey)), 0,
	)
	return hKey, ret == 0
}

// loadTaskMRU reads the last executed commands (MRU0..MRU3) from the registry.
func loadTaskMRU() []string {
	hKey, ok := openTaskMRUKey()
	if !ok {
		return nil
	}
	defer procRegCloseKey.Call(hKey)

	var list []string
	for i := 0; i < taskMRUSize; i++ {
		name, _ := syscall.UTF16PtrFromString(fmt.Sprintf("MRU%d", i))
		var buf [1024]uint16
		size := uint32(len(buf) * 2)
		var typ uint32
		ret, _, _ := procRegQueryValueExW.Call(
			hKey, uintptr(unsafe.Pointer(name)), 0,
			uintptr(unsafe.Pointer(&typ)),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&size)),
		)
		if ret == 0 && typ == regSz {
			if s := syscall.UTF16ToString(buf[:]); s != "" {
				list = append(list, s)
			}
		}
	}
	return list
}

// saveTaskMRU writes up to taskMRUSize commands back to the registry.
func saveTaskMRU(list []string) {
	hKey, ok := openTaskMRUKey()
	if !ok {
		return
	}
	defer procRegCloseKey.Call(hKey)

	for i := 0; i < taskMRUSize && i < len(list); i++ {
		name, _ := syscall.UTF16PtrFromString(fmt.Sprintf("MRU%d", i))
		data, err := syscall.UTF16FromString(list[i])
		if err != nil {
			continue
		}
		procRegSetValueExW.Call(
			hKey, uintptr(unsafe.Pointer(name)), 0, regSz,
			uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2),
		)
	}
}

// addTaskMRU puts cmd at the front of the MRU list, deduplicated and trimmed.
func addTaskMRU(cmd string) {
	out := []string{cmd}
	for _, s := range loadTaskMRU() {
		if !strings.EqualFold(s, cmd) && len(out) < taskMRUSize {
			out = append(out, s)
		}
	}
	saveTaskMRU(out)
}

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

	editClass, _ := syscall.UTF16PtrFromString("COMBOBOX")
	e, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(editClass)),
		0,
		uintptr(wsChild|wsVisible|wsTabStop|wsVScroll|cbsDropDown|cbsAutoHScroll),
		64, 71, dlgW-64-28, 150, // height includes the drop-down list
		uintptr(hwndNewTaskDlg), 0, hInstance, 0,
	)
	hwndNewTaskCombo = syscall.Handle(e)
	for _, cmd := range loadTaskMRU() {
		c, _ := syscall.UTF16PtrFromString(cmd)
		procSendMessage.Call(uintptr(hwndNewTaskCombo), cbAddString, 0, uintptr(unsafe.Pointer(c)))
	}

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
	procSetFocus.Call(uintptr(hwndNewTaskCombo))
}

// launchNewTask opens whatever is typed in the combo box (program, folder, or URL)
// and remembers it in the registry MRU.
func launchNewTask() {
	var buf [1024]uint16
	n, _, _ := procGetWindowTextW.Call(uintptr(hwndNewTaskCombo), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return
	}
	cmd := syscall.UTF16ToString(buf[:n])
	addTaskMRU(cmd)
	target, _ := syscall.UTF16PtrFromString(cmd)
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
				procSendMessage.Call(uintptr(hwndNewTaskCombo), 0x000C, 0, uintptr(unsafe.Pointer(t))) // WM_SETTEXT
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
		hwndNewTaskCombo = 0
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return ret
}
