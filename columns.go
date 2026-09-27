package main

import (
	"fmt"
	"strconv"
	"syscall"
	"unsafe"
)

const (
	lvmDeleteColumn     = lvmFirst + 28
	lvsExHeaderDragDrop = 0x00000010
	wsDisabled          = 0x08000000
	idColumnsOK         = 1
	idColumnsCancel     = 2
	grGdiObjects        = 0
	grUserObjects       = 1
)

// procColumn describes one selectable Processes-tab column.
type procColumn struct {
	title   string
	width   int32
	fmt     int32
	enabled bool
	locked  bool // Image Name can't be deselected
	value   func(e *procEntry) string
	less    func(a, b *procEntry) bool
}

func formatNum(n uint64) string {
	s := strconv.FormatUint(n, 10)
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// formatCPUTime renders 100ns ticks as h:mm:ss like Task Manager's CPU Time column.
func formatCPUTime(ticks uint64) string {
	secs := ticks / 10_000_000
	return fmt.Sprintf("%d:%02d:%02d", secs/3600, (secs/60)%60, secs%60)
}

func formatKBDelta(d int64) string {
	if d < 0 {
		return "-" + formatKB(uint64(-d))
	}
	return formatKB(uint64(d))
}

// formatBasePriority maps PROCESSENTRY32W.pcPriClassBase to Task Manager's names.
func formatBasePriority(p int32) string {
	switch p {
	case 4:
		return "Low"
	case 6:
		return "Below Normal"
	case 8:
		return "Normal"
	case 10:
		return "Above Normal"
	case 13:
		return "High"
	case 24:
		return "RealTime"
	case 0:
		return ""
	}
	return strconv.Itoa(int(p))
}

// procColumns is the master list backing both the list view and the Select
// Columns dialog (in the dialog's left-then-right reading order).
var procColumns = []*procColumn{
	{title: "Image Name", width: 140, fmt: lvcfmtLeft, enabled: true, locked: true,
		value: func(e *procEntry) string { return e.image },
		less:  func(a, b *procEntry) bool { return a.image < b.image }},
	{title: "User Name", width: 100, fmt: lvcfmtLeft, enabled: true,
		value: func(e *procEntry) string { return e.user },
		less:  func(a, b *procEntry) bool { return a.user < b.user }},
	{title: "PID", width: 50, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return strconv.Itoa(int(e.pid)) },
		less:  func(a, b *procEntry) bool { return a.pid < b.pid }},
	{title: "CPU", width: 45, fmt: lvcfmtRight, enabled: true,
		value: func(e *procEntry) string { return fmt.Sprintf("%02d", e.cpuPercent) },
		less:  func(a, b *procEntry) bool { return a.cpuPercent < b.cpuPercent }},
	{title: "CPU Time", width: 70, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatCPUTime(e.cpuTimeTicks) },
		less:  func(a, b *procEntry) bool { return a.cpuTimeTicks < b.cpuTimeTicks }},
	{title: "Mem Usage", width: 85, fmt: lvcfmtRight, enabled: true,
		value: func(e *procEntry) string { return formatKB(e.memKB) },
		less:  func(a, b *procEntry) bool { return a.memKB < b.memKB }},
	{title: "Mem Delta", width: 80, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatKBDelta(e.memDeltaKB) },
		less:  func(a, b *procEntry) bool { return a.memDeltaKB < b.memDeltaKB }},
	{title: "Peak Mem Usage", width: 100, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatKB(e.peakMemKB) },
		less:  func(a, b *procEntry) bool { return a.peakMemKB < b.peakMemKB }},
	{title: "Page Faults", width: 80, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatNum(uint64(e.pageFaults)) },
		less:  func(a, b *procEntry) bool { return a.pageFaults < b.pageFaults }},
	{title: "USER Objects", width: 85, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatNum(uint64(e.userObjects)) },
		less:  func(a, b *procEntry) bool { return a.userObjects < b.userObjects }},
	{title: "I/O Reads", width: 80, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatNum(e.ioReads) },
		less:  func(a, b *procEntry) bool { return a.ioReads < b.ioReads }},
	{title: "I/O Read Bytes", width: 100, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatNum(e.ioReadBytes) },
		less:  func(a, b *procEntry) bool { return a.ioReadBytes < b.ioReadBytes }},
	{title: "PF Delta", width: 70, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatNum(uint64(e.pageFaultsDelta)) },
		less:  func(a, b *procEntry) bool { return a.pageFaultsDelta < b.pageFaultsDelta }},
	{title: "VM Size", width: 85, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatKB(e.vmSizeKB) },
		less:  func(a, b *procEntry) bool { return a.vmSizeKB < b.vmSizeKB }},
	{title: "Paged Pool", width: 80, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatKB(e.pagedPoolKB) },
		less:  func(a, b *procEntry) bool { return a.pagedPoolKB < b.pagedPoolKB }},
	{title: "NP Pool", width: 70, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatKB(e.nonPagedPoolKB) },
		less:  func(a, b *procEntry) bool { return a.nonPagedPoolKB < b.nonPagedPoolKB }},
	{title: "Base Pri", width: 75, fmt: lvcfmtLeft,
		value: func(e *procEntry) string { return formatBasePriority(e.basePriority) },
		less:  func(a, b *procEntry) bool { return a.basePriority < b.basePriority }},
	{title: "Handles", width: 65, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatNum(uint64(e.handleCount)) },
		less:  func(a, b *procEntry) bool { return a.handleCount < b.handleCount }},
	{title: "Threads", width: 60, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatNum(uint64(e.threadCount)) },
		less:  func(a, b *procEntry) bool { return a.threadCount < b.threadCount }},
	{title: "GDI Objects", width: 80, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatNum(uint64(e.gdiObjects)) },
		less:  func(a, b *procEntry) bool { return a.gdiObjects < b.gdiObjects }},
	{title: "I/O Writes", width: 80, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatNum(e.ioWrites) },
		less:  func(a, b *procEntry) bool { return a.ioWrites < b.ioWrites }},
	{title: "I/O Write Bytes", width: 100, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatNum(e.ioWriteBytes) },
		less:  func(a, b *procEntry) bool { return a.ioWriteBytes < b.ioWriteBytes }},
	{title: "I/O Other", width: 80, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatNum(e.ioOther) },
		less:  func(a, b *procEntry) bool { return a.ioOther < b.ioOther }},
	{title: "I/O Other Bytes", width: 100, fmt: lvcfmtRight,
		value: func(e *procEntry) string { return formatNum(e.ioOtherBytes) },
		less:  func(a, b *procEntry) bool { return a.ioOtherBytes < b.ioOtherBytes }},
}

// dialog checkbox labels (full names, matching the real dialog)
var procColumnLabels = []string{
	"Image Name",
	"User Name",
	"PID (Process Identifier)",
	"CPU Usage",
	"CPU Time",
	"Memory Usage",
	"Memory Usage Delta",
	"Peak Memory Usage",
	"Page Faults",
	"USER Objects",
	"I/O Reads",
	"I/O Read Bytes",
	"Page Faults Delta",
	"Virtual Memory Size",
	"Paged Pool",
	"Non-paged Pool",
	"Base Priority",
	"Handle Count",
	"Thread Count",
	"GDI Objects",
	"I/O Writes",
	"I/O Write Bytes",
	"I/O Other",
	"I/O Other Bytes",
}

func enabledProcColumns() []*procColumn {
	var cols []*procColumn
	for _, c := range procColumns {
		if c.enabled {
			cols = append(cols, c)
		}
	}
	return cols
}

// rebuildProcColumns replaces the list view's columns with the enabled set.
func rebuildProcColumns() {
	if hwndProcList == 0 {
		return
	}
	for {
		ok, _, _ := procSendMessage.Call(uintptr(hwndProcList), lvmDeleteColumn, 0, 0)
		if ok == 0 {
			break
		}
	}
	for i, c := range enabledProcColumns() {
		text, _ := syscall.UTF16PtrFromString(c.title)
		col := lvColumnW{
			mask:     lvcfFmt | lvcfWidth | lvcfText | lvcfSubItem,
			fmt:      c.fmt,
			cx:       c.width,
			pszText:  text,
			iSubItem: int32(i),
		}
		procSendMessage.Call(uintptr(hwndProcList), lvmInsertColumnW, uintptr(i), uintptr(unsafe.Pointer(&col)))
	}
	procSortColumn = 0
	procSortAscending = true
}

var (
	columnsClassRegistered = false
	hwndColumnsDlg         syscall.Handle
	columnsDlgOwner        syscall.Handle
	columnsChecks          []syscall.Handle
)

func registerColumnsClass(hInstance uintptr) {
	if columnsClassRegistered {
		return
	}
	className, _ := syscall.UTF16PtrFromString("TaskMgr98Columns")
	cursor, _, _ := procLoadCursor.Call(0, uintptr(32512))

	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		lpfnWndProc:   syscall.NewCallback(columnsWndProc),
		hInstance:     syscall.Handle(hInstance),
		hCursor:       syscall.Handle(cursor),
		hbrBackground: syscall.Handle(colorBtnFace + 1),
		lpszClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	columnsClassRegistered = true
}

// showColumnsDialog opens the modal "Select Columns" dialog for the Processes tab.
func showColumnsDialog(owner syscall.Handle) {
	if hwndColumnsDlg != 0 {
		procSetForegroundWindow.Call(uintptr(hwndColumnsDlg))
		return
	}
	hInstance, _, _ := procGetModuleHandle.Call(0)
	registerColumnsClass(hInstance)

	rows := (len(procColumns) + 1) / 2
	const rowH, topText = 22, 56
	dlgW := int32(410)
	dlgH := int32(topText) + int32(rows)*rowH + 96

	var ownerRect rect
	procGetWindowRect.Call(uintptr(owner), uintptr(unsafe.Pointer(&ownerRect)))
	x := ownerRect.left + (ownerRect.right-ownerRect.left-dlgW)/2
	y := ownerRect.top + (ownerRect.bottom-ownerRect.top-dlgH)/2

	className, _ := syscall.UTF16PtrFromString("TaskMgr98Columns")
	title, _ := syscall.UTF16PtrFromString("Select Columns")
	h, _, _ := procCreateWindowEx.Call(
		wsExDlgModalFrame,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(wsPopup|wsCaption|wsSysMenu),
		uintptr(x), uintptr(y), uintptr(dlgW), uintptr(dlgH),
		uintptr(owner), 0, hInstance, 0,
	)
	if h == 0 {
		return
	}
	hwndColumnsDlg = syscall.Handle(h)
	columnsDlgOwner = owner
	columnsChecks = columnsChecks[:0]

	stClass, _ := syscall.UTF16PtrFromString("STATIC")
	intro, _ := syscall.UTF16PtrFromString("Select the columns that will appear on the Process page of the Task Manager.")
	procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(stClass)),
		uintptr(unsafe.Pointer(intro)),
		uintptr(wsChild|wsVisible),
		16, 12, uintptr(dlgW-32), 32,
		uintptr(hwndColumnsDlg), 0, hInstance, 0,
	)

	btnClass, _ := syscall.UTF16PtrFromString("BUTTON")
	for i, c := range procColumns {
		style := uintptr(wsChild | wsVisible | wsTabStop | bsAutoCheckbox)
		if c.locked {
			style |= wsDisabled
		}
		colX := int32(20)
		if i >= rows {
			colX = dlgW/2 + 4
		}
		cy := int32(topText) + int32(i%rows)*rowH
		label, _ := syscall.UTF16PtrFromString(procColumnLabels[i])
		ch, _, _ := procCreateWindowEx.Call(
			0,
			uintptr(unsafe.Pointer(btnClass)),
			uintptr(unsafe.Pointer(label)),
			style,
			uintptr(colX), uintptr(cy), 180, 18,
			uintptr(hwndColumnsDlg), 0, hInstance, 0,
		)
		check := syscall.Handle(ch)
		if c.enabled {
			procSendMessage.Call(uintptr(check), bmSetCheck, bstChecked, 0)
		}
		columnsChecks = append(columnsChecks, check)
	}

	makeButton := func(text string, id uintptr, x int32) {
		t, _ := syscall.UTF16PtrFromString(text)
		procCreateWindowEx.Call(
			0,
			uintptr(unsafe.Pointer(btnClass)),
			uintptr(unsafe.Pointer(t)),
			uintptr(wsChild|wsVisible|wsTabStop|bsPushButton),
			uintptr(x), uintptr(dlgH-72), 80, 24,
			uintptr(hwndColumnsDlg), id, hInstance, 0,
		)
	}
	makeButton("OK", idColumnsOK, dlgW-16-80-8-80)
	makeButton("Cancel", idColumnsCancel, dlgW-16-80)

	if font := createMessageFont(); font != 0 {
		procEnumChildWindows.Call(uintptr(hwndColumnsDlg), perfFontCallback, font)
	}

	procEnableWindow.Call(uintptr(owner), 0)
	procShowWindow.Call(uintptr(hwndColumnsDlg), swShowDefault)
	procUpdateWindow.Call(uintptr(hwndColumnsDlg))
}

// applyColumnsDialog copies the checkbox states into procColumns and rebuilds the list.
func applyColumnsDialog() {
	for i, check := range columnsChecks {
		if procColumns[i].locked {
			continue
		}
		state, _, _ := procSendMessage.Call(uintptr(check), bmGetCheck, 0, 0)
		procColumns[i].enabled = state == bstChecked
	}
	rebuildProcColumns()
	refreshProcList()
}

func closeColumnsDialog() {
	if hwndColumnsDlg != 0 {
		procDestroyWindow.Call(uintptr(hwndColumnsDlg))
	}
}

// columnsWndProc handles the Select Columns dialog; recover locally since a panic
// can't unwind across the native DispatchMessage frame (see wndProc).
func columnsWndProc(hwnd syscall.Handle, message uint32, wParam, lParam uintptr) (result uintptr) {
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
		case idColumnsOK:
			applyColumnsDialog()
			closeColumnsDialog()
			return 0
		case idColumnsCancel:
			closeColumnsDialog()
			return 0
		}
	case wmClose:
		closeColumnsDialog()
		return 0
	case wmDestroy:
		procEnableWindow.Call(uintptr(columnsDlgOwner), 1)
		procSetForegroundWindow.Call(uintptr(columnsDlgOwner))
		hwndColumnsDlg = 0
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return ret
}
