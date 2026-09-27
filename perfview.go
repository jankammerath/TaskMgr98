package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	bsGroupBox = 0x00000007
	ssLeft     = 0x00000000
	ssRight    = 0x00000002

	wmPaint          = 0x000F
	wmEraseBkgnd     = 0x0014
	wmUser           = 0x0400
	colorWindowFrame = 6

	psSolid = 0

	idPerfBase = 400
)

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type perfStatics struct {
	handles   syscall.Handle
	threads   syscall.Handle
	processes syscall.Handle

	physTotal syscall.Handle
	physAvail syscall.Handle
	physCache syscall.Handle

	commitTotal syscall.Handle
	commitLimit syscall.Handle
	commitPeak  syscall.Handle

	kernelTotal    syscall.Handle
	kernelPaged    syscall.Handle
	kernelNonpaged syscall.Handle
}

type performanceInformation struct {
	cb                uint32
	commitTotal       uintptr
	commitLimit       uintptr
	commitPeak        uintptr
	physicalTotal     uintptr
	physicalAvailable uintptr
	systemCache       uintptr
	kernelTotal       uintptr
	kernelPaged       uintptr
	kernelNonpaged    uintptr
	pageSize          uintptr
	handleCount       uint32
	processCount      uint32
	threadCount       uint32
}

var (
	procBeginPaint             = user32.NewProc("BeginPaint")
	procEndPaint               = user32.NewProc("EndPaint")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procCreatePen              = gdi32.NewProc("CreatePen")
	procMoveToEx               = gdi32.NewProc("MoveToEx")
	procLineTo                 = gdi32.NewProc("LineTo")
	procFillRect               = user32.NewProc("FillRect")
	procSetTextColor           = gdi32.NewProc("SetTextColor")
	procDrawTextW              = user32.NewProc("DrawTextW")

	psapiDll               = syscall.NewLazyDLL("psapi.dll")
	procGetPerformanceInfo = psapiDll.NewProc("GetPerformanceInfo")
	procGetSystemTimes     = kernel32.NewProc("GetSystemTimes")

	hwndPerfContainer syscall.Handle

	// Group Boxes
	hwndGrpCPUUsage syscall.Handle
	hwndGrpCPUHist  syscall.Handle
	hwndGrpPFUsage  syscall.Handle
	hwndGrpPFHist   syscall.Handle
	hwndGrpTotals   syscall.Handle
	hwndGrpPhysMem  syscall.Handle
	hwndGrpCommit   syscall.Handle
	hwndGrpKernel   syscall.Handle

	// Custom Graph Windows
	hwndCPUMeter syscall.Handle
	hwndCPUHist  syscall.Handle
	hwndPFMeter  syscall.Handle
	hwndPFHist   syscall.Handle

	perfLabels perfStatics
	perfStats  performanceInformation

	prevSysIdle   fileTime
	prevSysKernel fileTime
	prevSysUser   fileTime

	currentCPUUsage  = 0
	currentPFUsageMB = 0
	limitPFMB        = 0

	cpuHistory = make([]int, 0, 200)
	pfHistory  = make([]int, 0, 200)

	perfClassRegistered = false
)

func registerPerfGraphClass(hInstance uintptr) {
	if perfClassRegistered {
		return
	}
	className, _ := syscall.UTF16PtrFromString("TaskMgr98PerfGraph")
	cursor, _, _ := procLoadCursor.Call(0, uintptr(32512))

	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		style:         0,
		lpfnWndProc:   syscall.NewCallback(perfGraphWndProc),
		hInstance:     syscall.Handle(hInstance),
		hCursor:       syscall.Handle(cursor),
		hbrBackground: 0,
		lpszClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	perfClassRegistered = true
}

func createPerfView(hwndParent syscall.Handle, hInstance uintptr) syscall.Handle {
	registerPerfGraphClass(hInstance)

	containerClass, _ := syscall.UTF16PtrFromString("STATIC")
	hCont, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(containerClass)),
		0,
		uintptr(wsChild),
		0, 0, 0, 0,
		uintptr(hwndParent), idPerfBase, hInstance, 0,
	)
	hwndPerfContainer = syscall.Handle(hCont)

	makeGroup := func(title string) syscall.Handle {
		btnClass, _ := syscall.UTF16PtrFromString("BUTTON")
		tPtr, _ := syscall.UTF16PtrFromString(title)
		h, _, _ := procCreateWindowEx.Call(
			0,
			uintptr(unsafe.Pointer(btnClass)),
			uintptr(unsafe.Pointer(tPtr)),
			uintptr(wsChild|wsVisible|bsGroupBox),
			0, 0, 0, 0,
			uintptr(hwndPerfContainer), 0, hInstance, 0,
		)
		return syscall.Handle(h)
	}

	hwndGrpCPUUsage = makeGroup("CPU Usage")
	hwndGrpCPUHist = makeGroup("CPU Usage History")
	hwndGrpPFUsage = makeGroup("PF Usage")
	hwndGrpPFHist = makeGroup("Page File Usage History")
	hwndGrpTotals = makeGroup("Totals")
	hwndGrpPhysMem = makeGroup("Physical Memory (K)")
	hwndGrpCommit = makeGroup("Commit Charge (K)")
	hwndGrpKernel = makeGroup("Kernel Memory (K)")

	graphClass, _ := syscall.UTF16PtrFromString("TaskMgr98PerfGraph")
	makeGraph := func(id uintptr) syscall.Handle {
		h, _, _ := procCreateWindowEx.Call(
			0x00000200, // WS_EX_CLIENTEDGE
			uintptr(unsafe.Pointer(graphClass)),
			0,
			uintptr(wsChild|wsVisible),
			0, 0, 0, 0,
			uintptr(hwndPerfContainer), id, hInstance, 0,
		)
		return syscall.Handle(h)
	}

	hwndCPUMeter = makeGraph(1)
	hwndCPUHist = makeGraph(2)
	hwndPFMeter = makeGraph(3)
	hwndPFHist = makeGraph(4)

	makeStatic := func(parent syscall.Handle, text string, align uint32) syscall.Handle {
		stClass, _ := syscall.UTF16PtrFromString("STATIC")
		tPtr, _ := syscall.UTF16PtrFromString(text)
		h, _, _ := procCreateWindowEx.Call(
			0,
			uintptr(unsafe.Pointer(stClass)),
			uintptr(unsafe.Pointer(tPtr)),
			uintptr(wsChild|wsVisible|align),
			0, 0, 0, 0,
			uintptr(parent), 0, hInstance, 0,
		)
		return syscall.Handle(h)
	}

	// Totals
	makeStatic(hwndPerfContainer, "Handles", ssLeft)
	perfLabels.handles = makeStatic(hwndPerfContainer, "0", ssRight)
	makeStatic(hwndPerfContainer, "Threads", ssLeft)
	perfLabels.threads = makeStatic(hwndPerfContainer, "0", ssRight)
	makeStatic(hwndPerfContainer, "Processes", ssLeft)
	perfLabels.processes = makeStatic(hwndPerfContainer, "0", ssRight)

	// Physical Memory
	makeStatic(hwndPerfContainer, "Total", ssLeft)
	perfLabels.physTotal = makeStatic(hwndPerfContainer, "0", ssRight)
	makeStatic(hwndPerfContainer, "Available", ssLeft)
	perfLabels.physAvail = makeStatic(hwndPerfContainer, "0", ssRight)
	makeStatic(hwndPerfContainer, "System Cache", ssLeft)
	perfLabels.physCache = makeStatic(hwndPerfContainer, "0", ssRight)

	// Commit Charge
	makeStatic(hwndPerfContainer, "Total", ssLeft)
	perfLabels.commitTotal = makeStatic(hwndPerfContainer, "0", ssRight)
	makeStatic(hwndPerfContainer, "Limit", ssLeft)
	perfLabels.commitLimit = makeStatic(hwndPerfContainer, "0", ssRight)
	makeStatic(hwndPerfContainer, "Peak", ssLeft)
	perfLabels.commitPeak = makeStatic(hwndPerfContainer, "0", ssRight)

	// Kernel Memory
	makeStatic(hwndPerfContainer, "Total", ssLeft)
	perfLabels.kernelTotal = makeStatic(hwndPerfContainer, "0", ssRight)
	makeStatic(hwndPerfContainer, "Paged", ssLeft)
	perfLabels.kernelPaged = makeStatic(hwndPerfContainer, "0", ssRight)
	makeStatic(hwndPerfContainer, "Nonpaged", ssLeft)
	perfLabels.kernelNonpaged = makeStatic(hwndPerfContainer, "0", ssRight)

	return hwndPerfContainer
}

func layoutPerfView() {
	if hwndPerfContainer == 0 || hwndTab == 0 {
		return
	}

	var area rect
	procGetClientRect.Call(uintptr(hwndTab), uintptr(unsafe.Pointer(&area)))
	procSendMessage.Call(uintptr(hwndTab), tcmAdjustRect, 0, uintptr(unsafe.Pointer(&area)))
	procMapWindowPoints.Call(uintptr(hwndTab), uintptr(appMainHwnd), uintptr(unsafe.Pointer(&area)), 2)

	w := area.right - area.left
	h := area.bottom - area.top
	procMoveWindow.Call(uintptr(hwndPerfContainer), uintptr(area.left), uintptr(area.top), uintptr(w), uintptr(h), 1)

	pad := int32(8)
	meterW := int32(76)
	topHalfH := (h - pad*4) / 2
	graphH := (topHalfH - pad*3) / 2

	// Top row: CPU Usage & History
	y1 := pad
	procMoveWindow.Call(uintptr(hwndGrpCPUUsage), uintptr(pad), uintptr(y1), uintptr(meterW), uintptr(graphH), 1)
	procMoveWindow.Call(uintptr(hwndCPUMeter), uintptr(pad+8), uintptr(y1+18), uintptr(meterW-16), uintptr(graphH-26), 1)

	histX := pad + meterW + pad
	histW := w - histX - pad
	procMoveWindow.Call(uintptr(hwndGrpCPUHist), uintptr(histX), uintptr(y1), uintptr(histW), uintptr(graphH), 1)
	procMoveWindow.Call(uintptr(hwndCPUHist), uintptr(histX+8), uintptr(y1+18), uintptr(histW-16), uintptr(graphH-26), 1)

	// Middle row: PF Usage & History
	y2 := y1 + graphH + pad
	procMoveWindow.Call(uintptr(hwndGrpPFUsage), uintptr(pad), uintptr(y2), uintptr(meterW), uintptr(graphH), 1)
	procMoveWindow.Call(uintptr(hwndPFMeter), uintptr(pad+8), uintptr(y2+18), uintptr(meterW-16), uintptr(graphH-26), 1)

	procMoveWindow.Call(uintptr(hwndGrpPFHist), uintptr(histX), uintptr(y2), uintptr(histW), uintptr(graphH), 1)
	procMoveWindow.Call(uintptr(hwndPFHist), uintptr(histX+8), uintptr(y2+18), uintptr(histW-16), uintptr(graphH-26), 1)

	// Bottom half: 4 Stat Boxes (2x2 grid)
	yBottom := y2 + graphH + pad
	boxW := (w - pad*3) / 2
	boxH := (h - yBottom - pad*2) / 2

	col1X := pad
	col2X := pad + boxW + pad
	row1Y := yBottom
	row2Y := yBottom + boxH + pad

	procMoveWindow.Call(uintptr(hwndGrpTotals), uintptr(col1X), uintptr(row1Y), uintptr(boxW), uintptr(boxH), 1)
	procMoveWindow.Call(uintptr(hwndGrpPhysMem), uintptr(col2X), uintptr(row1Y), uintptr(boxW), uintptr(boxH), 1)
	procMoveWindow.Call(uintptr(hwndGrpCommit), uintptr(col1X), uintptr(row2Y), uintptr(boxW), uintptr(boxH), 1)
	procMoveWindow.Call(uintptr(hwndGrpKernel), uintptr(col2X), uintptr(row2Y), uintptr(boxW), uintptr(boxH), 1)

	layoutStatBlock := func(boxX, boxY, bW int32, row1, row2, row3 syscall.Handle) {
		valW := int32(75)
		valX := boxX + bW - valW - 12
		lineH := int32(16)
		startY := boxY + 20

		procMoveWindow.Call(uintptr(row1), uintptr(valX), uintptr(startY), uintptr(valW), uintptr(lineH), 1)
		procMoveWindow.Call(uintptr(row2), uintptr(valX), uintptr(startY+lineH), uintptr(valW), uintptr(lineH), 1)
		procMoveWindow.Call(uintptr(row3), uintptr(valX), uintptr(startY+lineH*2), uintptr(valW), uintptr(lineH), 1)
	}

	layoutStatBlock(col1X, row1Y, boxW, perfLabels.handles, perfLabels.threads, perfLabels.processes)
	layoutStatBlock(col2X, row1Y, boxW, perfLabels.physTotal, perfLabels.physAvail, perfLabels.physCache)
	layoutStatBlock(col1X, row2Y, boxW, perfLabels.commitTotal, perfLabels.commitLimit, perfLabels.commitPeak)
	layoutStatBlock(col2X, row2Y, boxW, perfLabels.kernelTotal, perfLabels.kernelPaged, perfLabels.kernelNonpaged)
}

func showPerfView(visible bool) {
	if hwndPerfContainer == 0 {
		return
	}
	cmd := swHide
	if visible {
		cmd = swShowNoActivate
	}
	procShowWindow.Call(uintptr(hwndPerfContainer), uintptr(cmd))
	if visible {
		refreshPerfData()
	}
}

func refreshPerfData() {
	if hwndPerfContainer == 0 {
		return
	}

	// Update CPU Usage
	var idle, kernel, user fileTime
	if ok, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	); ok != 0 {
		if prevSysKernel.ticks() != 0 {
			idleDiff := idle.ticks() - prevSysIdle.ticks()
			kernelDiff := kernel.ticks() - prevSysKernel.ticks()
			userDiff := user.ticks() - prevSysUser.ticks()
			totalSys := kernelDiff + userDiff

			if totalSys > 0 {
				busy := totalSys - idleDiff
				if busy < 0 {
					busy = 0
				}
				currentCPUUsage = int((busy * 100) / totalSys)
				if currentCPUUsage > 100 {
					currentCPUUsage = 100
				}
			}
		}
		prevSysIdle = idle
		prevSysKernel = kernel
		prevSysUser = user
	}

	// Update Memory Information
	var pi performanceInformation
	pi.cb = uint32(unsafe.Sizeof(pi))
	if ok, _, _ := procGetPerformanceInfo.Call(uintptr(unsafe.Pointer(&pi)), uintptr(pi.cb)); ok != 0 {
		perfStats = pi
		pageSizeKB := uint64(pi.pageSize) / 1024

		currentPFUsageMB = int((uint64(pi.commitTotal) * pageSizeKB) / 1024)
		limitPFMB = int((uint64(pi.commitLimit) * pageSizeKB) / 1024)

		setWinText := func(h syscall.Handle, val uint64) {
			s, _ := syscall.UTF16PtrFromString(fmt.Sprintf("%d", val))
			procSendMessage.Call(uintptr(h), 0x000C, 0, uintptr(unsafe.Pointer(s))) // WM_SETTEXT
		}

		setWinText(perfLabels.handles, uint64(pi.handleCount))
		setWinText(perfLabels.threads, uint64(pi.threadCount))
		setWinText(perfLabels.processes, uint64(pi.processCount))

		setWinText(perfLabels.physTotal, uint64(pi.physicalTotal)*pageSizeKB)
		setWinText(perfLabels.physAvail, uint64(pi.physicalAvailable)*pageSizeKB)
		setWinText(perfLabels.physCache, uint64(pi.systemCache)*pageSizeKB)

		setWinText(perfLabels.commitTotal, uint64(pi.commitTotal)*pageSizeKB)
		setWinText(perfLabels.commitLimit, uint64(pi.commitLimit)*pageSizeKB)
		setWinText(perfLabels.commitPeak, uint64(pi.commitPeak)*pageSizeKB)

		setWinText(perfLabels.kernelTotal, uint64(pi.kernelTotal)*pageSizeKB)
		setWinText(perfLabels.kernelPaged, uint64(pi.kernelPaged)*pageSizeKB)
		setWinText(perfLabels.kernelNonpaged, uint64(pi.kernelNonpaged)*pageSizeKB)
	}

	// Append to history slices
	cpuHistory = append(cpuHistory, currentCPUUsage)
	if len(cpuHistory) > 300 {
		cpuHistory = cpuHistory[1:]
	}

	pfPercent := 0
	if limitPFMB > 0 {
		pfPercent = (currentPFUsageMB * 100) / limitPFMB
	}
	pfHistory = append(pfHistory, pfPercent)
	if len(pfHistory) > 300 {
		pfHistory = pfHistory[1:]
	}

	// Redraw graph windows
	procInvalidateRect.Call(uintptr(hwndCPUMeter), 0, 0)
	procInvalidateRect.Call(uintptr(hwndCPUHist), 0, 0)
	procInvalidateRect.Call(uintptr(hwndPFMeter), 0, 0)
	procInvalidateRect.Call(uintptr(hwndPFHist), 0, 0)
}

func perfGraphWndProc(hwnd syscall.Handle, message uint32, wParam, lParam uintptr) (result uintptr) {
	switch message {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		if hdc == 0 {
			return 0
		}

		var rc rect
		procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rc)))
		w := rc.right - rc.left
		h := rc.bottom - rc.top

		// Double buffer
		memDC, _, _ := procCreateCompatibleDC.Call(hdc)
		memBmp, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(w), uintptr(h))
		oldBmp, _, _ := procSelectObject.Call(memDC, memBmp)

		// Black background
		blackBrush, _, _ := procCreateSolidBrush.Call(0x00000000)
		procFillRect.Call(memDC, uintptr(unsafe.Pointer(&rc)), blackBrush)
		procDeleteObject.Call(blackBrush)

		id, _, _ := user32.NewProc("GetWindowLongW").Call(uintptr(hwnd), ^uintptr(11)) // GWL_ID (-12)

		switch id {
		case 1: // CPU Bar Meter
			drawBarMeter(memDC, rc, currentCPUUsage, fmt.Sprintf("%d %%", currentCPUUsage))
		case 2: // CPU History Chart
			drawHistoryGraph(memDC, rc, cpuHistory, 0x0000FF00)
		case 3: // PF Bar Meter
			pfPct := 0
			if limitPFMB > 0 {
				pfPct = (currentPFUsageMB * 100) / limitPFMB
			}
			drawBarMeter(memDC, rc, pfPct, fmt.Sprintf("%d MB", currentPFUsageMB))
		case 4: // PF History Chart
			drawHistoryGraph(memDC, rc, pfHistory, 0x0000FF00)
		}

		procBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), memDC, 0, 0, 0x00CC0020) // SRCCOPY

		procSelectObject.Call(memDC, oldBmp)
		procDeleteObject.Call(memBmp)
		procDeleteDC.Call(memDC)

		procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return ret
}

func drawBarMeter(hdc uintptr, rc rect, percent int, label string) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	w := rc.right - rc.left
	h := rc.bottom - rc.top

	// Reserve bottom space for text readout
	labelHeight := int32(18)
	meterTop := int32(4)
	meterBottom := h - labelHeight
	meterHeight := meterBottom - meterTop
	meterLeft := int32(6)
	meterRight := w - int32(6)

	// Draw segments in LED bar style
	numSegments := int32(12)
	segGap := int32(2)
	segHeight := (meterHeight - (numSegments-1)*segGap) / numSegments

	litSegments := int32((int64(percent)*int64(numSegments) + 50) / 100)

	litBrush, _, _ := procCreateSolidBrush.Call(0x0000E000)  // Bright Green
	darkBrush, _, _ := procCreateSolidBrush.Call(0x00003000) // Faint Green

	for i := int32(0); i < numSegments; i++ {
		// Index 0 is bottom-most segment
		segY2 := meterBottom - i*(segHeight+segGap)
		segY1 := segY2 - segHeight

		segRect := rect{left: meterLeft, top: segY1, right: meterRight, bottom: segY2}
		if i < litSegments {
			procFillRect.Call(hdc, uintptr(unsafe.Pointer(&segRect)), litBrush)
		} else {
			procFillRect.Call(hdc, uintptr(unsafe.Pointer(&segRect)), darkBrush)
		}
	}

	procDeleteObject.Call(litBrush)
	procDeleteObject.Call(darkBrush)

	// Draw percentage/MB label at bottom
	procSetBkMode.Call(hdc, 1) // TRANSPARENT
	procSetTextColor.Call(hdc, 0x0000FF00)

	textRect := rect{left: 0, top: meterBottom + 2, right: w, bottom: h}
	labelPtr, _ := syscall.UTF16PtrFromString(label)
	dtCenter := uintptr(0x00000001 | 0x00000004 | 0x00000020) // DT_CENTER | DT_VCENTER | DT_SINGLELINE
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(labelPtr)), uintptr(int32(len(label))), uintptr(unsafe.Pointer(&textRect)), dtCenter)
}

func drawHistoryGraph(hdc uintptr, rc rect, values []int, lineColor uint32) {
	w := rc.right - rc.left
	h := rc.bottom - rc.top

	// Dark green grid pen
	gridPen, _, _ := procCreatePen.Call(psSolid, 1, 0x00005500)
	oldPen, _, _ := procSelectObject.Call(hdc, gridPen)

	gridSpacing := int32(12)

	// Vertical grid lines
	for x := w - 1; x >= 0; x -= gridSpacing {
		procMoveToEx.Call(hdc, uintptr(x), 0, 0)
		procLineTo.Call(hdc, uintptr(x), uintptr(h))
	}

	// Horizontal grid lines
	for y := h - 1; y >= 0; y -= gridSpacing {
		procMoveToEx.Call(hdc, uintptr(0), uintptr(y), 0)
		procLineTo.Call(hdc, uintptr(w), uintptr(y))
	}

	// Draw bright green trend curve
	trendPen, _, _ := procCreatePen.Call(psSolid, 1, uintptr(lineColor))
	procSelectObject.Call(hdc, trendPen)

	step := gridSpacing
	count := len(values)

	for i := 0; i < count; i++ {
		val := values[count-1-i]
		if val < 0 {
			val = 0
		}
		if val > 100 {
			val = 100
		}

		x := (w - 1) - int32(i)*step
		if x < 0 {
			break
		}
		y := (h - 1) - int32((int64(val)*int64(h-2))/100)

		if i == 0 {
			procMoveToEx.Call(hdc, uintptr(x), uintptr(y), 0)
		} else {
			procLineTo.Call(hdc, uintptr(x), uintptr(y))
		}
	}

	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(gridPen)
	procDeleteObject.Call(trendPen)
}
