package main

import (
	"syscall"
	"unsafe"
)

const (
	lvsReport        = 0x0001
	lvsShowSelAlways = 0x0008

	lvsExFullRowSelect = 0x00000020

	lvmFirst                    = 0x1000
	lvmGetNextItem              = lvmFirst + 12
	lvmDeleteAllItems           = lvmFirst + 9
	lvmSetItemState             = lvmFirst + 43
	lvmSetItemW                 = lvmFirst + 76
	lvmInsertItemW              = lvmFirst + 77
	lvmSetExtendedListViewStyle = lvmFirst + 54
	lvmInsertColumnW            = lvmFirst + 97

	lvcfFmt     = 0x0001
	lvcfWidth   = 0x0002
	lvcfText    = 0x0004
	lvcfSubItem = 0x0008
	lvcfmtLeft  = 0

	lvifText     = 0x0001
	lvifImage    = 0x0002
	lvniSelected = 0x0002
	lvisFocused  = 0x0001
	lvisSelected = 0x0002

	lvsilSmall      = 1
	lvmSetImageList = lvmFirst + 3
	ilcMask         = 0x00000001
	ilcColor32      = 0x00000020

	smCxSmIcon = 49
	smCySmIcon = 50

	wmGeticon       = 0x007F
	iconSmall       = 0
	iconBig         = 1
	iconSmall2      = 2
	gclpHicon       = -14
	gclpHiconsm     = -34
	smtoAbortIfHung = 0x0002

	gwOwner        = 4
	gwlExStyle     = -20
	wsExToolWindow = 0x00000080

	swHide           = 0
	swShowNoActivate = 4
	swRestore        = 9
	wmClose          = 0x0010
	wsTabStop        = 0x00010000
	bsPushButton     = 0x00000000

	ofnFileMustExist = 0x00001000
	ofnPathMustExist = 0x00000800
	ofnHideReadOnly  = 0x00000004

	idAppList  = 200
	idEndTask  = 201
	idSwitchTo = 202
	idNewTask  = 203

	buttonAreaHeight = 40
	buttonHeight     = 24
	buttonWidth      = 90
	buttonGap        = 8

	processQueryLimitedInformation = 0x1000
	shgfiIcon                      = 0x000000100
	shgfiSmallIcon                 = 0x000000001
	shgfiUseFileAttributes         = 0x000000010
)

// shFileInfoW mirrors SHFILEINFOW from shellapi.h
type shFileInfoW struct {
	hIcon         syscall.Handle
	iIcon         int32
	dwAttributes  uint32
	szDisplayName [260]uint16
	szTypeName    [80]uint16
}

// lvColumnW mirrors LVCOLUMNW; field order/alignment must match the Win32 struct.
type lvColumnW struct {
	mask       uint32
	fmt        int32
	cx         int32
	pszText    *uint16
	cchTextMax int32
	iSubItem   int32
	iImage     int32
	iOrder     int32
	cxMin      int32
	cxDefault  int32
	cxIdeal    int32
}

// lvItemW mirrors the base (pre-groups) fields of LVITEMW.
type lvItemW struct {
	mask       uint32
	iItem      int32
	iSubItem   int32
	state      uint32
	stateMask  uint32
	pszText    *uint16
	cchTextMax int32
	iImage     int32
	lParam     uintptr
	iIndent    int32
}

// openFileNameW mirrors OPENFILENAMEW, used by the "New Task..." program picker.
type openFileNameW struct {
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
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

// appEntry pairs a top-level window handle with its title for one Applications row.
type appEntry struct {
	hwnd  syscall.Handle
	title string
}

// safeCall invokes a LazyProc, recovering if resolving/calling it panics (e.g. an
// export missing on the current Windows version) so one bad API can't crash the app.
func safeCall(p *syscall.LazyProc, args ...uintptr) (r1 uintptr, ok bool) {
	defer func() {
		if recover() != nil {
			r1, ok = 0, false
		}
	}()
	r1, _, _ = p.Call(args...)
	return r1, true
}

var (
	procEnumWindows          = user32.NewProc("EnumWindows")
	procIsWindowVisible      = user32.NewProc("IsWindowVisible")
	procGetWindow            = user32.NewProc("GetWindow")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procGetWindowLongW       = user32.NewProc("GetWindowLongW")
	procPostMessage          = user32.NewProc("PostMessageW")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procMapWindowPoints      = user32.NewProc("MapWindowPoints")
	procSendMessageTimeout   = user32.NewProc("SendMessageTimeoutW")
	procGetClassLongPtrW     = user32.NewProc("GetClassLongPtrW")
	procGetSystemMetrics     = user32.NewProc("GetSystemMetrics")

	procImageListCreate      = comctl32.NewProc("ImageList_Create")
	procImageListReplaceIcon = comctl32.NewProc("ImageList_ReplaceIcon")
	procImageListRemoveAll   = comctl32.NewProc("ImageList_RemoveAll")

	comdlg32             = syscall.NewLazyDLL("comdlg32.dll")
	procGetOpenFileNameW = comdlg32.NewProc("GetOpenFileNameW")
	shell32              = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW    = shell32.NewProc("ShellExecuteW")

	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procCloseHandle                = kernel32.NewProc("CloseHandle")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procGetWindowThreadProcessId   = user32.NewProc("GetWindowThreadProcessId")
	procFindWindowExW              = user32.NewProc("FindWindowExW")
	procLoadIconW                  = user32.NewProc("LoadIconW")
	procSHGetFileInfoW             = shell32.NewProc("SHGetFileInfoW")

	// appMainHwnd is excluded from the enumerated list so the app doesn't list itself.
	appMainHwnd  syscall.Handle
	hwndAppList  syscall.Handle
	appImageList syscall.Handle
	fallbackIcon syscall.Handle

	hwndEndTask  syscall.Handle
	hwndSwitchTo syscall.Handle
	hwndNewTask  syscall.Handle

	appListCallback = syscall.NewCallback(enumAppWindowsProc)
	pendingEntries  []appEntry
	currentEntries  []appEntry
)

// createAppListView creates the report-mode list view backing the Applications tab.
// It is parented to the main window (not the tab control) so its notifications and
// the action buttons' WM_COMMAND messages reach wndProc.
func createAppListView(hwndParent syscall.Handle, hInstance uintptr) syscall.Handle {
	className, _ := syscall.UTF16PtrFromString("SysListView32")
	h, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		uintptr(wsChild|wsVisible|lvsReport|lvsShowSelAlways),
		0, 0, 0, 0,
		uintptr(hwndParent), idAppList, hInstance, 0,
	)
	list := syscall.Handle(h)

	procSendMessage.Call(uintptr(list), lvmSetExtendedListViewStyle, 0, lvsExFullRowSelect)

	addColumn := func(index int32, label string, width int32) {
		text, _ := syscall.UTF16PtrFromString(label)
		col := lvColumnW{
			mask:     lvcfFmt | lvcfWidth | lvcfText | lvcfSubItem,
			fmt:      lvcfmtLeft,
			cx:       width,
			pszText:  text,
			iSubItem: index,
		}
		procSendMessage.Call(uintptr(list), lvmInsertColumnW, uintptr(index), uintptr(unsafe.Pointer(&col)))
	}
	addColumn(0, "Task", 260)
	addColumn(1, "Status", 100)

	if cx, ok := safeCall(procGetSystemMetrics, smCxSmIcon); ok {
		if cy, ok := safeCall(procGetSystemMetrics, smCySmIcon); ok {
			if himl, ok := safeCall(procImageListCreate, cx, cy, ilcColor32|ilcMask, 0, 8); ok && himl != 0 {
				appImageList = syscall.Handle(himl)
				procSendMessage.Call(uintptr(list), lvmSetImageList, lvsilSmall, uintptr(appImageList))
			}
		}
	}

	return list
}

// createAppListButtons creates the End Task / Switch To / New Task... buttons.
func createAppListButtons(hwndParent syscall.Handle, hInstance uintptr) {
	className, _ := syscall.UTF16PtrFromString("BUTTON")
	makeButton := func(id uintptr, label string) syscall.Handle {
		text, _ := syscall.UTF16PtrFromString(label)
		h, _, _ := procCreateWindowEx.Call(
			0,
			uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(text)),
			uintptr(wsChild|wsVisible|wsTabStop|bsPushButton),
			0, 0, 0, 0,
			uintptr(hwndParent), id, hInstance, 0,
		)
		return syscall.Handle(h)
	}
	hwndEndTask = makeButton(idEndTask, "End Task")
	hwndSwitchTo = makeButton(idSwitchTo, "Switch To")
	hwndNewTask = makeButton(idNewTask, "New Task...")
}

// layoutAppList sizes the list view and buttons to the tab control's content
// rectangle, translated into the main window's client coordinates.
func layoutAppList() {
	if hwndAppList == 0 || hwndTab == 0 {
		return
	}

	var area rect
	procGetClientRect.Call(uintptr(hwndTab), uintptr(unsafe.Pointer(&area)))
	procSendMessage.Call(uintptr(hwndTab), tcmAdjustRect, 0, uintptr(unsafe.Pointer(&area)))
	// area holds two POINTs (left,top)/(right,bottom); MapWindowPoints translates both in place.
	procMapWindowPoints.Call(uintptr(hwndTab), uintptr(appMainHwnd), uintptr(unsafe.Pointer(&area)), 2)

	buttonsTop := area.bottom - buttonAreaHeight
	procMoveWindow.Call(
		uintptr(hwndAppList),
		uintptr(area.left), uintptr(area.top),
		uintptr(area.right-area.left), uintptr(buttonsTop-area.top),
		1,
	)

	buttonY := buttonsTop + (buttonAreaHeight-buttonHeight)/2
	x := area.right - buttonWidth
	procMoveWindow.Call(uintptr(hwndNewTask), uintptr(x), uintptr(buttonY), uintptr(buttonWidth), uintptr(buttonHeight), 1)
	x -= buttonWidth + buttonGap
	procMoveWindow.Call(uintptr(hwndSwitchTo), uintptr(x), uintptr(buttonY), uintptr(buttonWidth), uintptr(buttonHeight), 1)
	x -= buttonWidth + buttonGap
	procMoveWindow.Call(uintptr(hwndEndTask), uintptr(x), uintptr(buttonY), uintptr(buttonWidth), uintptr(buttonHeight), 1)
}

// showAppList toggles the list/buttons' visibility, refreshing the list when shown.
func showAppList(visible bool) {
	if hwndAppList == 0 {
		return
	}
	cmd := swHide
	if visible {
		cmd = swShowNoActivate
	}
	for _, h := range []syscall.Handle{hwndAppList, hwndEndTask, hwndSwitchTo, hwndNewTask} {
		procShowWindow.Call(uintptr(h), uintptr(cmd))
	}
	if visible {
		refreshAppList()
	}
}

// selectedIndex returns the currently selected row, or -1 if none.
func selectedIndex() int {
	start := int32(-1)
	sel, _, _ := procSendMessage.Call(uintptr(hwndAppList), lvmGetNextItem, uintptr(start), lvniSelected)
	return int(int32(sel))
}

// selectAppRow marks row i as the selected/focused item.
func selectAppRow(i int) {
	state := uint32(lvisSelected | lvisFocused)
	item := lvItemW{state: state, stateMask: state}
	procSendMessage.Call(uintptr(hwndAppList), lvmSetItemState, uintptr(i), uintptr(unsafe.Pointer(&item)))
}

// selectedEntry returns the entry behind the current selection, if any.
func selectedEntry() (appEntry, bool) {
	idx := selectedIndex()
	if idx < 0 || idx >= len(currentEntries) {
		return appEntry{}, false
	}
	return currentEntries[idx], true
}

// refreshAppList re-enumerates top-level application windows, repopulates the list,
// and restores the previous selection (by window handle) so it survives the refresh.
func refreshAppList() {
	if hwndAppList == 0 {
		return
	}

	var selectedHwnd syscall.Handle
	if entry, ok := selectedEntry(); ok {
		selectedHwnd = entry.hwnd
	}

	pendingEntries = pendingEntries[:0]
	procEnumWindows.Call(appListCallback, 0)

	procSendMessage.Call(uintptr(hwndAppList), lvmDeleteAllItems, 0, 0)
	if appImageList != 0 {
		safeCall(procImageListRemoveAll, uintptr(appImageList))
	}
	for i, entry := range pendingEntries {
		iconIndex := int32(-1)
		if appImageList != 0 {
			hicon := getWindowIcon(entry.hwnd)
			// ImageList_ReplaceIcon returns a 32-bit int (-1 on failure); reinterpret via int32,
			// not a 32-bit mask, since Call widens the raw return value to a 64-bit uintptr.
			if img, ok := safeCall(procImageListReplaceIcon, uintptr(appImageList), uintptr(iconIndex), uintptr(hicon)); ok && int32(img) != -1 {
				iconIndex = int32(img)
			}
		}
		insertAppRow(int32(i), entry.title, iconIndex)
	}
	currentEntries = append(currentEntries[:0], pendingEntries...)

	if selectedHwnd != 0 {
		for i, entry := range currentEntries {
			if entry.hwnd == selectedHwnd {
				selectAppRow(i)
				break
			}
		}
	}
}

// enumAppWindowsProc is the EnumWindows callback; it collects titles of top-level,
// visible, non-tool-window applications, mirroring what Task Manager's Applications
// tab shows.
func enumAppWindowsProc(hwnd syscall.Handle, lParam uintptr) uintptr {
	const enumContinue = 1

	if hwnd == appMainHwnd {
		return enumContinue
	}

	visible, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
	if visible == 0 {
		return enumContinue
	}

	owner, _, _ := procGetWindow.Call(uintptr(hwnd), gwOwner)
	if owner != 0 {
		return enumContinue
	}

	exStyleIndex := int32(gwlExStyle) // runtime conversion; constant -20 can't fold into uintptr
	exStyle, _, _ := procGetWindowLongW.Call(uintptr(hwnd), uintptr(exStyleIndex))
	if uint32(exStyle)&wsExToolWindow != 0 {
		return enumContinue
	}

	length, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd))
	if length == 0 {
		return enumContinue
	}

	buf := make([]uint16, length+1)
	procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(length+1))
	pendingEntries = append(pendingEntries, appEntry{hwnd: hwnd, title: syscall.UTF16ToString(buf)})

	return enumContinue
}

// insertAppRow adds one "Task | Status" row to the list view.
func insertAppRow(index int32, title string, iconIndex int32) {
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	item := lvItemW{
		mask:    lvifText | lvifImage,
		iItem:   index,
		iImage:  iconIndex,
		pszText: titlePtr,
	}
	procSendMessage.Call(uintptr(hwndAppList), lvmInsertItemW, 0, uintptr(unsafe.Pointer(&item)))

	statusPtr, _ := syscall.UTF16PtrFromString("Running")
	status := lvItemW{
		mask:     lvifText,
		iItem:    index,
		iSubItem: 1,
		pszText:  statusPtr,
	}
	procSendMessage.Call(uintptr(hwndAppList), lvmSetItemW, 0, uintptr(unsafe.Pointer(&status)))
}

// endSelectedTask asks the selected window to close, like Task Manager's End Task.
func endSelectedTask() {
	entry, ok := selectedEntry()
	if !ok {
		return
	}
	procPostMessage.Call(uintptr(entry.hwnd), wmClose, 0, 0)
	refreshAppList()
}

// switchToSelectedTask restores and foregrounds the selected window.
func switchToSelectedTask() {
	entry, ok := selectedEntry()
	if !ok {
		return
	}
	procShowWindow.Call(uintptr(entry.hwnd), swRestore)
	procSetForegroundWindow.Call(uintptr(entry.hwnd))
}

// buildFilterBuf assembles a double-NUL-terminated GetOpenFileName filter string;
// UTF16PtrFromString can't be used directly since it rejects embedded NUL bytes.
func buildFilterBuf(parts ...string) []uint16 {
	var buf []uint16
	for _, p := range parts {
		enc, _ := syscall.UTF16FromString(p)
		buf = append(buf, enc...)
	}
	return append(buf, 0)
}

// showNewTaskDialog lets the user pick a program to launch, like Task Manager's
// "Create New Task" dialog.
func showNewTaskDialog(owner syscall.Handle) {
	var file [260]uint16
	filterBuf := buildFilterBuf("Programs", "*.exe", "All Files", "*.*")
	title, _ := syscall.UTF16PtrFromString("Create New Task")

	ofn := openFileNameW{
		hwndOwner:   owner,
		lpstrFilter: &filterBuf[0],
		lpstrFile:   &file[0],
		nMaxFile:    uint32(len(file)),
		lpstrTitle:  title,
		flags:       ofnFileMustExist | ofnPathMustExist | ofnHideReadOnly,
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))

	ok, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ok == 0 {
		return
	}

	path := syscall.UTF16ToString(file[:])
	pathPtr, _ := syscall.UTF16PtrFromString(path)
	openVerb, _ := syscall.UTF16PtrFromString("open")
	procShellExecuteW.Call(0, uintptr(unsafe.Pointer(openVerb)), uintptr(unsafe.Pointer(pathPtr)), 0, 0, swShowDefault)
}
