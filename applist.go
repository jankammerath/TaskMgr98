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
	lvmDeleteAllItems           = lvmFirst + 9
	lvmSetItemW                 = lvmFirst + 76
	lvmInsertItemW              = lvmFirst + 77
	lvmSetExtendedListViewStyle = lvmFirst + 54
	lvmInsertColumnW            = lvmFirst + 97

	lvcfFmt     = 0x0001
	lvcfWidth   = 0x0002
	lvcfText    = 0x0004
	lvcfSubItem = 0x0008
	lvcfmtLeft  = 0

	lvifText = 0x0001

	gwOwner        = 4
	gwlExStyle     = -20
	wsExToolWindow = 0x00000080

	swHide           = 0
	swShowNoActivate = 4

	idAppList = 200
)

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

var (
	procEnumWindows          = user32.NewProc("EnumWindows")
	procIsWindowVisible      = user32.NewProc("IsWindowVisible")
	procGetWindow            = user32.NewProc("GetWindow")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procGetWindowLongW       = user32.NewProc("GetWindowLongW")

	// appMainHwnd is excluded from the enumerated list so the app doesn't list itself.
	appMainHwnd syscall.Handle
	hwndAppList syscall.Handle

	appListCallback = syscall.NewCallback(enumAppWindowsProc)
	pendingTitles   []string
)

// createAppListView creates the report-mode list view backing the Applications tab.
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

	return list
}

// layoutAppList sizes the list view to the tab control's content rectangle, excluding
// the tab strip itself.
func layoutAppList() {
	if hwndAppList == 0 || hwndTab == 0 {
		return
	}
	var area rect
	procGetClientRect.Call(uintptr(hwndTab), uintptr(unsafe.Pointer(&area)))
	procSendMessage.Call(uintptr(hwndTab), tcmAdjustRect, 0, uintptr(unsafe.Pointer(&area)))
	procMoveWindow.Call(
		uintptr(hwndAppList),
		uintptr(area.left), uintptr(area.top),
		uintptr(area.right-area.left), uintptr(area.bottom-area.top),
		1,
	)
}

// showAppList toggles the list view's visibility, refreshing it whenever it becomes visible.
func showAppList(visible bool) {
	if hwndAppList == 0 {
		return
	}
	cmd := swHide
	if visible {
		cmd = swShowNoActivate
	}
	procShowWindow.Call(uintptr(hwndAppList), uintptr(cmd))
	if visible {
		refreshAppList()
	}
}

// refreshAppList re-enumerates top-level application windows and repopulates the list.
func refreshAppList() {
	if hwndAppList == 0 {
		return
	}

	pendingTitles = pendingTitles[:0]
	procEnumWindows.Call(appListCallback, 0)

	procSendMessage.Call(uintptr(hwndAppList), lvmDeleteAllItems, 0, 0)
	for i, title := range pendingTitles {
		insertAppRow(int32(i), title)
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
	pendingTitles = append(pendingTitles, syscall.UTF16ToString(buf))

	return enumContinue
}

// insertAppRow adds one "Task | Status" row to the list view.
func insertAppRow(index int32, title string) {
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	item := lvItemW{
		mask:    lvifText,
		iItem:   index,
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
