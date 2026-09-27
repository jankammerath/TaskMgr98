package main

import (
	"fmt"
	"sort"
	"strconv"
	"syscall"
	"unsafe"
)

const (
	idProcList     = 300
	idEndProcess   = 301
	idShowAllUsers = 302

	th32csSnapProcess = 0x00000002
	invalidHandle     = ^uintptr(0)

	processTerminate = 0x0001
	tokenQuery       = 0x0008
	tokenUser        = 1

	bsAutoCheckbox = 0x00000003
	bmSetCheck     = 0x00F1
	bmGetCheck     = 0x00F0
	bstChecked     = 1

	checkboxWidth   = 220
	checkboxLeftPad = 8

	wmSetRedraw    = 0x000B
	lvmGetTopIndex = lvmFirst + 39 // LVM_GETTOPINDEX
	lvmGetItemRect = lvmFirst + 14 // LVM_GETITEMRECT
	lvmScroll      = lvmFirst + 20 // LVM_SCROLL
	lvirBounds     = 0
)

// processEntry32W mirrors PROCESSENTRY32W from tlhelp32.h.
type processEntry32W struct {
	dwSize              uint32
	cntUsage            uint32
	th32ProcessID       uint32
	th32DefaultHeapID   uintptr
	th32ModuleID        uint32
	cntThreads          uint32
	th32ParentProcessID uint32
	pcPriClassBase      int32
	dwFlags             uint32
	szExeFile           [260]uint16
}

// fileTime mirrors FILETIME.
type fileTime struct {
	dwLowDateTime  uint32
	dwHighDateTime uint32
}

func (ft fileTime) ticks() uint64 {
	return uint64(ft.dwHighDateTime)<<32 | uint64(ft.dwLowDateTime)
}

// processMemoryCounters mirrors PROCESS_MEMORY_COUNTERS from psapi.h.
type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

// systemInfo mirrors the fields of SYSTEM_INFO needed for the CPU count.
type systemInfo struct {
	wProcessorArchitecture      uint16
	wReserved                   uint16
	dwPageSize                  uint32
	lpMinimumApplicationAddress uintptr
	lpMaximumApplicationAddress uintptr
	dwActiveProcessorMask       uintptr
	dwNumberOfProcessors        uint32
	dwProcessorType             uint32
	dwAllocationGranularity     uint32
	wProcessorLevel             uint16
	wProcessorRevision          uint16
}

// procTimes captures per-process values from the previous refresh for delta columns.
type procTimes struct {
	kernel, user uint64
	memKB        uint64
	pageFaults   uint32
}

// ioCounters mirrors IO_COUNTERS.
type ioCounters struct {
	readOps, writeOps, otherOps       uint64
	readBytes, writeBytes, otherBytes uint64
}

// procEntry is one row backing the Processes tab.
type procEntry struct {
	pid             uint32
	image           string
	user            string
	cpuPercent      int
	cpuTimeTicks    uint64
	memKB           uint64
	memDeltaKB      int64
	peakMemKB       uint64
	pageFaults      uint32
	pageFaultsDelta int64
	vmSizeKB        uint64
	pagedPoolKB     uint64
	nonPagedPoolKB  uint64
	basePriority    int32
	handleCount     uint32
	threadCount     uint32
	userObjects     uint32
	gdiObjects      uint32
	ioReads         uint64
	ioReadBytes     uint64
	ioWrites        uint64
	ioWriteBytes    uint64
	ioOther         uint64
	ioOtherBytes    uint64
}

var (
	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32.NewProc("Process32NextW")
	procGetProcessTimes          = kernel32.NewProc("GetProcessTimes")
	procGetSystemInfo            = kernel32.NewProc("GetSystemInfo")
	procTerminateProcess         = kernel32.NewProc("TerminateProcess")
	procGetProcessHandleCount    = kernel32.NewProc("GetProcessHandleCount")
	procGetProcessIoCounters     = kernel32.NewProc("GetProcessIoCounters")
	procGetGuiResources          = user32.NewProc("GetGuiResources")

	psapi                    = syscall.NewLazyDLL("psapi.dll")
	procGetProcessMemoryInfo = psapi.NewProc("GetProcessMemoryInfo")

	advapi32                = syscall.NewLazyDLL("advapi32.dll")
	procOpenProcessToken    = advapi32.NewProc("OpenProcessToken")
	procGetTokenInformation = advapi32.NewProc("GetTokenInformation")
	procLookupAccountSidW   = advapi32.NewProc("LookupAccountSidW")
	procGetUserNameW        = advapi32.NewProc("GetUserNameW")

	procInvalidateRect = user32.NewProc("InvalidateRect")

	hwndProcList     syscall.Handle
	hwndEndProcess   syscall.Handle
	hwndShowAllUsers syscall.Handle

	numCPUs            uint32 = 1
	currentUserName    string
	showAllUsers       = true
	prevProcTimes      = map[uint32]procTimes{}
	currentProcEntries []procEntry

	procSortColumn    int32 = 0
	procSortAscending       = true
)

// setProcSortColumn toggles ascending/descending when the same header is clicked
// again, or switches to ascending on a newly clicked column, then re-sorts.
func setProcSortColumn(col int32) {
	if procSortColumn == col {
		procSortAscending = !procSortAscending
	} else {
		procSortColumn = col
		procSortAscending = true
	}
	refreshProcList()
}

// sortProcEntries orders entries by the clicked column, honoring
// procSortColumn/procSortAscending; the index maps into the enabled column set.
func sortProcEntries(entries []procEntry) {
	cols := enabledProcColumns()
	colLess := cols[0].less
	if int(procSortColumn) < len(cols) && procSortColumn >= 0 {
		colLess = cols[procSortColumn].less
	}
	less := func(i, j int) bool { return colLess(&entries[i], &entries[j]) }
	sort.SliceStable(entries, func(i, j int) bool {
		if procSortAscending {
			return less(i, j)
		}
		return less(j, i)
	})
}

// createProcListView creates the report-mode list view backing the Processes tab.
// It starts hidden since the Applications tab is selected first.
func createProcListView(hwndParent syscall.Handle, hInstance uintptr) syscall.Handle {
	className, _ := syscall.UTF16PtrFromString("SysListView32")
	h, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		uintptr(wsChild|lvsReport|lvsShowSelAlways),
		0, 0, 0, 0,
		uintptr(hwndParent), idProcList, hInstance, 0,
	)
	list := syscall.Handle(h)

	procSendMessage.Call(uintptr(list), lvmSetExtendedListViewStyle, 0, lvsExFullRowSelect|lvsExHeaderDragDrop)

	hwndProcList = list
	rebuildProcColumns()

	var si systemInfo
	procGetSystemInfo.Call(uintptr(unsafe.Pointer(&si)))
	if si.dwNumberOfProcessors > 0 {
		numCPUs = si.dwNumberOfProcessors
	}

	nameBuf := make([]uint16, 256)
	size := uint32(len(nameBuf))
	if ok, _, _ := procGetUserNameW.Call(uintptr(unsafe.Pointer(&nameBuf[0])), uintptr(unsafe.Pointer(&size))); ok != 0 {
		currentUserName = syscall.UTF16ToString(nameBuf)
	}

	return list
}

// createProcListButtons creates the "Show processes from all users" checkbox and
// the End Process button.
func createProcListButtons(hwndParent syscall.Handle, hInstance uintptr) {
	buttonClass, _ := syscall.UTF16PtrFromString("BUTTON")

	checkText, _ := syscall.UTF16PtrFromString("Show processes from all users")
	h, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(buttonClass)),
		uintptr(unsafe.Pointer(checkText)),
		uintptr(wsChild|wsTabStop|bsAutoCheckbox),
		0, 0, 0, 0,
		uintptr(hwndParent), idShowAllUsers, hInstance, 0,
	)
	hwndShowAllUsers = syscall.Handle(h)
	procSendMessage.Call(uintptr(hwndShowAllUsers), bmSetCheck, bstChecked, 0)

	endText, _ := syscall.UTF16PtrFromString("End Process")
	h, _, _ = procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(buttonClass)),
		uintptr(unsafe.Pointer(endText)),
		uintptr(wsChild|wsTabStop|bsPushButton),
		0, 0, 0, 0,
		uintptr(hwndParent), idEndProcess, hInstance, 0,
	)
	hwndEndProcess = syscall.Handle(h)
}

// layoutProcList sizes the list view, checkbox, and End Process button to the tab
// control's content rectangle, translated into the main window's client coordinates.
func layoutProcList() {
	if hwndProcList == 0 || hwndTab == 0 {
		return
	}

	var area rect
	procGetClientRect.Call(uintptr(hwndTab), uintptr(unsafe.Pointer(&area)))
	procSendMessage.Call(uintptr(hwndTab), tcmAdjustRect, 0, uintptr(unsafe.Pointer(&area)))
	procMapWindowPoints.Call(uintptr(hwndTab), uintptr(appMainHwnd), uintptr(unsafe.Pointer(&area)), 2)

	buttonsTop := area.bottom - buttonAreaHeight
	procMoveWindow.Call(
		uintptr(hwndProcList),
		uintptr(area.left), uintptr(area.top),
		uintptr(area.right-area.left), uintptr(buttonsTop-area.top),
		1,
	)

	buttonY := buttonsTop + (buttonAreaHeight-buttonHeight)/2
	procMoveWindow.Call(uintptr(hwndShowAllUsers), uintptr(area.left+checkboxLeftPad), uintptr(buttonY), uintptr(checkboxWidth), uintptr(buttonHeight), 1)

	x := area.right - buttonWidth - buttonGap
	procMoveWindow.Call(uintptr(hwndEndProcess), uintptr(x), uintptr(buttonY), uintptr(buttonWidth), uintptr(buttonHeight), 1)
}

// showProcList toggles the process list/controls' visibility, refreshing when shown.
func showProcList(visible bool) {
	if hwndProcList == 0 {
		return
	}
	cmd := swHide
	if visible {
		cmd = swShowNoActivate
	}
	for _, h := range []syscall.Handle{hwndProcList, hwndShowAllUsers, hwndEndProcess} {
		procShowWindow.Call(uintptr(h), uintptr(cmd))
	}
	if visible {
		refreshProcList()
	}
}

// selectedProcIndex returns the currently selected row, or -1 if none.
func selectedProcIndex() int {
	start := int32(-1)
	sel, _, _ := procSendMessage.Call(uintptr(hwndProcList), lvmGetNextItem, uintptr(start), lvniSelected)
	return int(int32(sel))
}

// selectProcRow marks row i as the selected/focused item.
func selectProcRow(i int) {
	state := uint32(lvisSelected | lvisFocused)
	item := lvItemW{state: state, stateMask: state}
	procSendMessage.Call(uintptr(hwndProcList), lvmSetItemState, uintptr(i), uintptr(unsafe.Pointer(&item)))
}

// selectedProcEntry returns the entry behind the current selection, if any.
func selectedProcEntry() (procEntry, bool) {
	idx := selectedProcIndex()
	if idx < 0 || idx >= len(currentProcEntries) {
		return procEntry{}, false
	}
	return currentProcEntries[idx], true
}

// refreshProcList re-enumerates running processes and repopulates the list,
// restoring the previous selection (by PID) and scroll position so they survive the
// refresh, with redraw suppressed around the rebuild to avoid visible flicker.
func refreshProcList() {
	if hwndProcList == 0 {
		return
	}

	if hwndShowAllUsers != 0 {
		state, _, _ := procSendMessage.Call(uintptr(hwndShowAllUsers), bmGetCheck, 0, 0)
		showAllUsers = state == bstChecked
	}

	var selectedPid uint32
	if entry, ok := selectedProcEntry(); ok {
		selectedPid = entry.pid
	}
	topIndex, _, _ := procSendMessage.Call(uintptr(hwndProcList), lvmGetTopIndex, 0, 0)

	entries := enumerateProcesses()
	if !showAllUsers && currentUserName != "" {
		filtered := entries[:0]
		for _, e := range entries {
			if e.user == currentUserName {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}
	sortProcEntries(entries)

	procSendMessage.Call(uintptr(hwndProcList), wmSetRedraw, 0, 0)
	procSendMessage.Call(uintptr(hwndProcList), lvmDeleteAllItems, 0, 0)
	for i, e := range entries {
		insertProcRow(int32(i), e)
	}
	currentProcEntries = entries

	if selectedPid != 0 {
		for i, e := range currentProcEntries {
			if e.pid == selectedPid {
				selectProcRow(i)
				break
			}
		}
	}
	restoreProcListTop(int32(topIndex), len(entries))
	procSendMessage.Call(uintptr(hwndProcList), wmSetRedraw, 1, 0)
	procInvalidateRect.Call(uintptr(hwndProcList), 0, 1)
}

// restoreProcListTop scrolls the list so the row that was on top before the refresh
// (rows are always re-inserted starting at the top) is on top again, computing the
// pixel offset from one row's height since LVM_SCROLL only takes pixel deltas.
func restoreProcListTop(topIndex int32, count int) {
	if topIndex <= 0 || int(topIndex) >= count {
		return
	}
	var r rect
	r.left = lvirBounds
	if ok, _, _ := procSendMessage.Call(uintptr(hwndProcList), lvmGetItemRect, 0, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return
	}
	rowHeight := r.bottom - r.top
	if rowHeight <= 0 {
		return
	}
	procSendMessage.Call(uintptr(hwndProcList), lvmScroll, 0, uintptr(topIndex*rowHeight))
}

// enumerateProcesses snapshots running processes via CreateToolhelp32Snapshot and
// fills in CPU%/memory/owner for each, best-effort (unreadable processes get blanks).
func enumerateProcesses() []procEntry {
	snap, _, _ := procCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if snap == 0 || snap == invalidHandle {
		return nil
	}
	defer procCloseHandle.Call(snap)

	var pe processEntry32W
	pe.dwSize = uint32(unsafe.Sizeof(pe))

	var entries []procEntry
	seen := map[uint32]bool{}

	ok, _, _ := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&pe)))
	for ok != 0 {
		pid := pe.th32ProcessID
		seen[pid] = true
		entry := buildProcEntry(pid, syscall.UTF16ToString(pe.szExeFile[:]))
		entry.threadCount = pe.cntThreads
		entry.basePriority = pe.pcPriClassBase
		entries = append(entries, entry)
		ok, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&pe)))
	}

	for pid := range prevProcTimes {
		if !seen[pid] {
			delete(prevProcTimes, pid)
		}
	}

	return entries
}

// buildProcEntry queries CPU time, memory, and owner for one process, tolerating
// access-denied failures (common for other users'/system processes when unelevated).
func buildProcEntry(pid uint32, image string) procEntry {
	entry := procEntry{pid: pid, image: image}

	hProc, ok := safeCall(procOpenProcess, processQueryLimitedInformation, 0, uintptr(pid))
	if !ok || hProc == 0 {
		return entry
	}
	defer procCloseHandle.Call(hProc)

	prev, hasPrev := prevProcTimes[pid]

	var creation, exit, kernel, user fileTime
	if ret, _, _ := procGetProcessTimes.Call(hProc,
		uintptr(unsafe.Pointer(&creation)), uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)),
	); ret != 0 {
		total := kernel.ticks() + user.ticks()
		entry.cpuTimeTicks = total
		if hasPrev && numCPUs > 0 {
			delta := total - (prev.kernel + prev.user)
			intervalTicks := uint64(updateIntervalMs) * 10000
			pct := int(delta * 100 / (intervalTicks * uint64(numCPUs)))
			if pct > 100 {
				pct = 100
			}
			entry.cpuPercent = pct
		}
	}

	var mc processMemoryCounters
	mc.cb = uint32(unsafe.Sizeof(mc))
	if ret, _, _ := procGetProcessMemoryInfo.Call(hProc, uintptr(unsafe.Pointer(&mc)), uintptr(mc.cb)); ret != 0 {
		entry.memKB = uint64(mc.workingSetSize) / 1024
		entry.peakMemKB = uint64(mc.peakWorkingSetSize) / 1024
		entry.pageFaults = mc.pageFaultCount
		entry.vmSizeKB = uint64(mc.pagefileUsage) / 1024
		entry.pagedPoolKB = uint64(mc.quotaPagedPoolUsage) / 1024
		entry.nonPagedPoolKB = uint64(mc.quotaNonPagedPoolUsage) / 1024
		if hasPrev {
			entry.memDeltaKB = int64(entry.memKB) - int64(prev.memKB)
			entry.pageFaultsDelta = int64(entry.pageFaults) - int64(prev.pageFaults)
		}
	}

	prevProcTimes[pid] = procTimes{
		kernel:     kernel.ticks(),
		user:       user.ticks(),
		memKB:      entry.memKB,
		pageFaults: entry.pageFaults,
	}

	var handles uint32
	procGetProcessHandleCount.Call(hProc, uintptr(unsafe.Pointer(&handles)))
	entry.handleCount = handles

	userObjs, _, _ := procGetGuiResources.Call(hProc, grUserObjects)
	gdiObjs, _, _ := procGetGuiResources.Call(hProc, grGdiObjects)
	entry.userObjects = uint32(userObjs)
	entry.gdiObjects = uint32(gdiObjs)

	var io ioCounters
	if ret, _, _ := procGetProcessIoCounters.Call(hProc, uintptr(unsafe.Pointer(&io))); ret != 0 {
		entry.ioReads = io.readOps
		entry.ioReadBytes = io.readBytes
		entry.ioWrites = io.writeOps
		entry.ioWriteBytes = io.writeBytes
		entry.ioOther = io.otherOps
		entry.ioOtherBytes = io.otherBytes
	}

	entry.user = lookupProcessOwner(hProc)
	return entry
}

// lookupProcessOwner resolves an open process handle's token SID to an account name.
func lookupProcessOwner(hProc uintptr) string {
	var hToken uintptr
	if ok, _, _ := procOpenProcessToken.Call(hProc, tokenQuery, uintptr(unsafe.Pointer(&hToken))); ok == 0 {
		return ""
	}
	defer procCloseHandle.Call(hToken)

	buf := make([]byte, 512)
	var retLen uint32
	ok, _, _ := procGetTokenInformation.Call(
		hToken, tokenUser,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
		uintptr(unsafe.Pointer(&retLen)),
	)
	if ok == 0 {
		return ""
	}
	// The SID pointer is the first field of the TOKEN_USER buffer (SID_AND_ATTRIBUTES.Sid).
	sidPtr := *(*uintptr)(unsafe.Pointer(&buf[0]))

	nameBuf := make([]uint16, 256)
	domainBuf := make([]uint16, 256)
	nameLen := uint32(len(nameBuf))
	domainLen := uint32(len(domainBuf))
	var use int32
	ok, _, _ = procLookupAccountSidW.Call(
		0, sidPtr,
		uintptr(unsafe.Pointer(&nameBuf[0])), uintptr(unsafe.Pointer(&nameLen)),
		uintptr(unsafe.Pointer(&domainBuf[0])), uintptr(unsafe.Pointer(&domainLen)),
		uintptr(unsafe.Pointer(&use)),
	)
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(nameBuf[:nameLen])
}

// endSelectedProcess forcibly terminates the selected process, like Task Manager's
// "End Process".
func endSelectedProcess() {
	entry, ok := selectedProcEntry()
	if !ok {
		return
	}
	hProc, ok := safeCall(procOpenProcess, processTerminate, 0, uintptr(entry.pid))
	if !ok || hProc == 0 {
		return
	}
	defer procCloseHandle.Call(hProc)
	procTerminateProcess.Call(hProc, 1)
	refreshProcList()
}

// formatKB formats a KB count with thousands separators, e.g. "4,128 K".
func formatKB(kb uint64) string {
	s := strconv.FormatUint(kb, 10)
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return fmt.Sprintf("%s K", out)
}

// insertProcRow adds one row to the list view, filling every enabled column.
func insertProcRow(index int32, e procEntry) {
	cols := enabledProcColumns()
	if len(cols) == 0 {
		return
	}
	firstPtr, _ := syscall.UTF16PtrFromString(cols[0].value(&e))
	item := lvItemW{mask: lvifText, iItem: index, pszText: firstPtr}
	procSendMessage.Call(uintptr(hwndProcList), lvmInsertItemW, 0, uintptr(unsafe.Pointer(&item)))

	for sub := 1; sub < len(cols); sub++ {
		ptr, _ := syscall.UTF16PtrFromString(cols[sub].value(&e))
		subItem := lvItemW{mask: lvifText, iItem: index, iSubItem: int32(sub), pszText: ptr}
		procSendMessage.Call(uintptr(hwndProcList), lvmSetItemW, 0, uintptr(unsafe.Pointer(&subItem)))
	}
}
