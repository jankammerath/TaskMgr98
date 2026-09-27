package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	idNetBase     = 500
	idNetList     = 501
	maxNetGraphs  = 4
	netHistoryMax = 300

	netTitleHeight    = 18
	netAxisWidth      = 36
	netChartMaxHeight = 160

	lvcfmtRight = 1

	ifTypeLoopback        = 24
	mediaConnected        = 1
	flagHardwareInterface = 0x01
)

// mibIfRow2 mirrors MIB_IF_ROW2 from netioapi.h (field order/alignment must match).
type mibIfRow2 struct {
	interfaceLuid               uint64
	interfaceIndex              uint32
	interfaceGuid               [16]byte
	alias                       [257]uint16
	description                 [257]uint16
	physicalAddressLength       uint32
	physicalAddress             [32]byte
	permanentPhysicalAddress    [32]byte
	mtu                         uint32
	ifType                      uint32
	tunnelType                  uint32
	mediaType                   uint32
	physicalMediumType          uint32
	accessType                  uint32
	directionType               uint32
	interfaceAndOperStatusFlags uint8
	operStatus                  uint32
	adminStatus                 uint32
	mediaConnectState           uint32
	networkGuid                 [16]byte
	connectionType              uint32
	transmitLinkSpeed           uint64
	receiveLinkSpeed            uint64
	inOctets                    uint64
	inUcastPkts                 uint64
	inNUcastPkts                uint64
	inDiscards                  uint64
	inErrors                    uint64
	inUnknownProtos             uint64
	inUcastOctets               uint64
	inMulticastOctets           uint64
	inBroadcastOctets           uint64
	outOctets                   uint64
	outUcastPkts                uint64
	outNUcastPkts               uint64
	outDiscards                 uint64
	outErrors                   uint64
	outUcastOctets              uint64
	outMulticastOctets          uint64
	outBroadcastOctets          uint64
	outQLen                     uint64
}

// mibIfTable2 mirrors MIB_IF_TABLE2; table rows start at offset 8.
type mibIfTable2 struct {
	numEntries uint32
	_          uint32
	table      [1]mibIfRow2
}

// netAdapter is one network interface shown on the Networking tab.
type netAdapter struct {
	luid      uint64
	name      string
	linkSpeed uint64 // bits/sec
	connected bool
	prevIn    uint64
	prevOut   uint64
	havePrev  bool
	utilBP    int   // current utilization in basis points (1/100 %)
	history   []int // utilization history in basis points
}

var (
	iphlpapi         = syscall.NewLazyDLL("iphlpapi.dll")
	procGetIfTable2  = iphlpapi.NewProc("GetIfTable2")
	procFreeMibTable = iphlpapi.NewProc("FreeMibTable")

	procDrawEdge = user32.NewProc("DrawEdge")

	hwndNetContainer syscall.Handle
	hwndNetList      syscall.Handle
	hwndNetGraphs    [maxNetGraphs]syscall.Handle

	netAdapters        []*netAdapter
	netClassRegistered = false
	netViewVisible     = false
)

func registerNetGraphClass(hInstance uintptr) {
	if netClassRegistered {
		return
	}
	className, _ := syscall.UTF16PtrFromString("TaskMgr98NetGraph")
	cursor, _, _ := procLoadCursor.Call(0, uintptr(32512))

	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		lpfnWndProc:   syscall.NewCallback(netGraphWndProc),
		hInstance:     syscall.Handle(hInstance),
		hCursor:       syscall.Handle(cursor),
		lpszClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	netClassRegistered = true
}

func createNetView(hwndParent syscall.Handle, hInstance uintptr) syscall.Handle {
	registerNetGraphClass(hInstance)

	containerClass, _ := syscall.UTF16PtrFromString("STATIC")
	hCont, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(containerClass)),
		0,
		uintptr(wsChild),
		0, 0, 0, 0,
		uintptr(hwndParent), idNetBase, hInstance, 0,
	)
	hwndNetContainer = syscall.Handle(hCont)

	graphClass, _ := syscall.UTF16PtrFromString("TaskMgr98NetGraph")
	for i := 0; i < maxNetGraphs; i++ {
		h, _, _ := procCreateWindowEx.Call(
			0,
			uintptr(unsafe.Pointer(graphClass)),
			0,
			uintptr(wsChild),
			0, 0, 0, 0,
			uintptr(hwndNetContainer), uintptr(i+1), hInstance, 0,
		)
		hwndNetGraphs[i] = syscall.Handle(h)
	}

	listClass, _ := syscall.UTF16PtrFromString("SysListView32")
	h, _, _ := procCreateWindowEx.Call(
		0x00000200, // WS_EX_CLIENTEDGE
		uintptr(unsafe.Pointer(listClass)),
		0,
		uintptr(wsChild|wsVisible|lvsReport|lvsShowSelAlways),
		0, 0, 0, 0,
		uintptr(hwndNetContainer), idNetList, hInstance, 0,
	)
	hwndNetList = syscall.Handle(h)
	procSendMessage.Call(uintptr(hwndNetList), lvmSetExtendedListViewStyle, 0, lvsExFullRowSelect)

	addColumn := func(index int32, label string, width int32, format int32) {
		text, _ := syscall.UTF16PtrFromString(label)
		col := lvColumnW{
			mask:     lvcfFmt | lvcfWidth | lvcfText | lvcfSubItem,
			fmt:      format,
			cx:       width,
			pszText:  text,
			iSubItem: index,
		}
		procSendMessage.Call(uintptr(hwndNetList), lvmInsertColumnW, uintptr(index), uintptr(unsafe.Pointer(&col)))
	}
	addColumn(0, "Adapter Name", 140, lvcfmtLeft)
	addColumn(1, "Network Utilization", 110, lvcfmtRight)
	addColumn(2, "Link Speed", 80, lvcfmtRight)
	addColumn(3, "State", 110, lvcfmtLeft)

	return hwndNetContainer
}

// setNetFonts applies the UI font to the adapter list (graphs paint their own text).
func setNetFonts(font uintptr) {
	if hwndNetList != 0 && font != 0 {
		procSendMessage.Call(uintptr(hwndNetList), wmSetFont, font, 1)
	}
}

func layoutNetView() {
	if hwndNetContainer == 0 || hwndTab == 0 {
		return
	}

	var area rect
	procGetClientRect.Call(uintptr(hwndTab), uintptr(unsafe.Pointer(&area)))
	procSendMessage.Call(uintptr(hwndTab), tcmAdjustRect, 0, uintptr(unsafe.Pointer(&area)))
	procMapWindowPoints.Call(uintptr(hwndTab), uintptr(appMainHwnd), uintptr(unsafe.Pointer(&area)), 2)

	w := area.right - area.left
	h := area.bottom - area.top
	procMoveWindow.Call(uintptr(hwndNetContainer), uintptr(area.left), uintptr(area.top), uintptr(w), uintptr(h), 1)

	pad := int32(8)
	listH := int32(110)

	n := int32(len(netAdapters))
	if n > maxNetGraphs {
		n = maxNetGraphs
	}
	if n > 0 {
		graphH := (h - listH - pad*(n+2)) / n
		maxGraphH := int32(netTitleHeight + netChartMaxHeight + 4)
		if graphH > maxGraphH {
			graphH = maxGraphH
		}
		y := pad
		for i := int32(0); i < n; i++ {
			procMoveWindow.Call(uintptr(hwndNetGraphs[i]), uintptr(pad), uintptr(y), uintptr(w-pad*2), uintptr(graphH), 1)
			y += graphH + pad
		}
	}

	procMoveWindow.Call(uintptr(hwndNetList), uintptr(pad), uintptr(h-listH-pad), uintptr(w-pad*2), uintptr(listH), 1)
}

func showNetView(visible bool) {
	if hwndNetContainer == 0 {
		return
	}
	netViewVisible = visible
	cmd := swHide
	if visible {
		cmd = swShowNoActivate
	}
	procShowWindow.Call(uintptr(hwndNetContainer), uintptr(cmd))
	if visible {
		refreshNetData()
	}
}

// readIfTable2 fetches the interface table and returns the rows worth showing
// (hardware, non-loopback interfaces).
func readIfTable2() []mibIfRow2 {
	var table uintptr
	ret, _, _ := procGetIfTable2.Call(uintptr(unsafe.Pointer(&table)))
	if ret != 0 || table == 0 {
		return nil
	}
	defer procFreeMibTable.Call(table)

	tbl := *(**mibIfTable2)(unsafe.Pointer(&table))
	rowSize := unsafe.Sizeof(mibIfRow2{})
	base := unsafe.Pointer(&tbl.table[0])

	var rows []mibIfRow2
	for i := uintptr(0); i < uintptr(tbl.numEntries); i++ {
		row := *(*mibIfRow2)(unsafe.Pointer(uintptr(base) + i*rowSize))
		if row.ifType == ifTypeLoopback || row.interfaceAndOperStatusFlags&flagHardwareInterface == 0 {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func refreshNetData() {
	if hwndNetContainer == 0 {
		return
	}

	prevCount := len(netAdapters)
	rows := readIfTable2()

	byLuid := make(map[uint64]*netAdapter, len(netAdapters))
	for _, ad := range netAdapters {
		byLuid[ad.luid] = ad
	}

	next := make([]*netAdapter, 0, len(rows))
	for i := range rows {
		row := &rows[i]

		ad := byLuid[row.interfaceLuid]
		if ad == nil {
			ad = &netAdapter{luid: row.interfaceLuid}
		}

		ad.name = syscall.UTF16ToString(row.alias[:])
		ad.connected = row.mediaConnectState == mediaConnected && row.operStatus == 1

		speed := max(row.transmitLinkSpeed, row.receiveLinkSpeed)
		if speed == ^uint64(0) {
			speed = 0
		}
		ad.linkSpeed = speed

		ad.utilBP = 0
		if ad.havePrev && ad.connected && speed > 0 {
			bits := (row.inOctets - ad.prevIn + row.outOctets - ad.prevOut) * 8
			bp := bits * 10000 * 1000 / (speed * timerIntervalMs)
			ad.utilBP = min(int(bp), 10000)
		}
		ad.prevIn = row.inOctets
		ad.prevOut = row.outOctets
		ad.havePrev = true

		ad.history = append(ad.history, ad.utilBP)
		if len(ad.history) > netHistoryMax {
			ad.history = ad.history[1:]
		}

		next = append(next, ad)
	}
	netAdapters = next

	// Rebuild the adapter list with redraw suppressed to avoid flicker.
	procSendMessage.Call(uintptr(hwndNetList), wmSetRedraw, 0, 0)
	procSendMessage.Call(uintptr(hwndNetList), lvmDeleteAllItems, 0, 0)
	for i, ad := range netAdapters {
		insertNetRow(int32(i), ad)
	}
	procSendMessage.Call(uintptr(hwndNetList), wmSetRedraw, 1, 0)
	procInvalidateRect.Call(uintptr(hwndNetList), 0, 0)

	if len(netAdapters) != prevCount {
		layoutNetView()
	}
	for i := 0; i < maxNetGraphs; i++ {
		if i < len(netAdapters) {
			procShowWindow.Call(uintptr(hwndNetGraphs[i]), swShowNoActivate)
			procInvalidateRect.Call(uintptr(hwndNetGraphs[i]), 0, 0)
		} else {
			procShowWindow.Call(uintptr(hwndNetGraphs[i]), swHide)
		}
	}
}

func insertNetRow(index int32, ad *netAdapter) {
	namePtr, _ := syscall.UTF16PtrFromString(ad.name)
	item := lvItemW{mask: lvifText, iItem: index, pszText: namePtr}
	procSendMessage.Call(uintptr(hwndNetList), lvmInsertItemW, 0, uintptr(unsafe.Pointer(&item)))

	setSubItem := func(sub int32, text string) {
		ptr, _ := syscall.UTF16PtrFromString(text)
		subItem := lvItemW{mask: lvifText, iItem: index, iSubItem: sub, pszText: ptr}
		procSendMessage.Call(uintptr(hwndNetList), lvmSetItemW, 0, uintptr(unsafe.Pointer(&subItem)))
	}
	setSubItem(1, formatUtilBP(ad.utilBP))
	setSubItem(2, formatLinkSpeed(ad.linkSpeed))
	state := "Non Operational"
	if ad.connected {
		state = "Operational"
	}
	setSubItem(3, state)
}

func formatUtilBP(bp int) string {
	if bp == 0 {
		return "0 %"
	}
	return fmt.Sprintf("%d.%02d %%", bp/100, bp%100)
}

func formatLinkSpeed(bps uint64) string {
	switch {
	case bps >= 1_000_000_000:
		return fmt.Sprintf("%d Gbps", bps/1_000_000_000)
	case bps >= 1_000_000:
		return fmt.Sprintf("%d Mbps", bps/1_000_000)
	case bps >= 1_000:
		return fmt.Sprintf("%d Kbps", bps/1_000)
	default:
		return fmt.Sprintf("%d bps", bps)
	}
}

// netGraphWndProc paints one adapter's utilization history; recover locally since a
// panic can't unwind across the native DispatchMessage frame (see wndProc).
func netGraphWndProc(hwnd syscall.Handle, message uint32, wParam, lParam uintptr) (result uintptr) {
	defer func() {
		if r := recover(); r != nil {
			logCrash(r)
			ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
			result = ret
		}
	}()
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

		// Dialog background; the black chart area is painted by drawNetGraph.
		bgBrush, _, _ := procGetSysColorBrush.Call(colorBtnFace)
		procFillRect.Call(memDC, uintptr(unsafe.Pointer(&rc)), bgBrush)

		id, _, _ := procGetWindowLongW.Call(uintptr(hwnd), ^uintptr(11)) // GWL_ID (-12)
		idx := int(id) - 1
		if idx >= 0 && idx < len(netAdapters) {
			drawNetGraph(memDC, rc, netAdapters[idx])
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

func drawNetGraph(hdc uintptr, rc rect, ad *netAdapter) {
	w := rc.right - rc.left
	h := rc.bottom - rc.top

	// Chart area: below the title, right of the y-axis gutter, capped at 160px tall.
	chart := rect{left: netAxisWidth, top: netTitleHeight, right: w - 2, bottom: h - 2}
	if chart.bottom-chart.top > netChartMaxHeight {
		chart.bottom = chart.top + netChartMaxHeight
	}
	if chart.right <= chart.left || chart.bottom <= chart.top {
		return
	}
	chartH := chart.bottom - chart.top

	blackBrush, _, _ := procCreateSolidBrush.Call(0x00000000)
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&chart)), blackBrush)
	procDeleteObject.Call(blackBrush)

	// Dark green grid
	gridPen, _, _ := procCreatePen.Call(psSolid, 1, 0x00005500)
	oldPen, _, _ := procSelectObject.Call(hdc, gridPen)

	gridSpacing := int32(12)
	for x := chart.right - 1; x >= chart.left; x -= gridSpacing {
		procMoveToEx.Call(hdc, uintptr(x), uintptr(chart.top), 0)
		procLineTo.Call(hdc, uintptr(x), uintptr(chart.bottom))
	}
	for y := chart.bottom - 1; y >= chart.top; y -= gridSpacing {
		procMoveToEx.Call(hdc, uintptr(chart.left), uintptr(y), 0)
		procLineTo.Call(hdc, uintptr(chart.right), uintptr(y))
	}

	// Pick the smallest full-percent scale that fits the history's peak.
	peak := 0
	for _, v := range ad.history {
		peak = max(peak, v)
	}
	scaleBP := 10000
	for _, s := range []int{100, 200, 500, 1000, 2500, 5000, 10000} {
		if peak <= s {
			scaleBP = s
			break
		}
	}

	// Bright green trend curve
	trendPen, _, _ := procCreatePen.Call(psSolid, 1, 0x0000FF00)
	procSelectObject.Call(hdc, trendPen)

	count := len(ad.history)
	for i := 0; i < count; i++ {
		val := min(max(ad.history[count-1-i], 0), scaleBP)

		x := (chart.right - 1) - int32(i)*gridSpacing
		if x < chart.left {
			break
		}
		y := (chart.bottom - 1) - int32((int64(val)*int64(chartH-2))/int64(scaleBP))

		if i == 0 {
			procMoveToEx.Call(hdc, uintptr(x), uintptr(y), 0)
		} else {
			procLineTo.Call(hdc, uintptr(x), uintptr(y))
		}
	}

	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(gridPen)
	procDeleteObject.Call(trendPen)

	// Sunken frame around the chart
	frame := rect{left: chart.left - 2, top: chart.top - 2, right: chart.right + 2, bottom: chart.bottom + 2}
	procDrawEdge.Call(hdc, uintptr(unsafe.Pointer(&frame)), 0x000A, 0x000F) // EDGE_SUNKEN, BF_RECT

	// Title (blue) above the chart, y-axis scale labels (black) in the left gutter
	procSetBkMode.Call(hdc, 1) // TRANSPARENT
	labelFont := logFont{lfHeight: -11, lfWeight: 400}
	faceName, _ := syscall.UTF16FromString("Segoe UI")
	copy(labelFont.lfFaceName[:], faceName)
	font, _, _ := procCreateFontIndirect.Call(uintptr(unsafe.Pointer(&labelFont)))
	oldFont, _, _ := procSelectObject.Call(hdc, font)

	drawText := func(text string, r rect, flags uintptr) {
		buf, _ := syscall.UTF16FromString(text)
		procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)-1), uintptr(unsafe.Pointer(&r)), flags|0x20) // DT_SINGLELINE
	}

	procSetTextColor.Call(hdc, 0x00FF0000) // Blue
	drawText(ad.name, rect{left: 2, top: 1, right: w, bottom: netTitleHeight - 2}, 0)

	procSetTextColor.Call(hdc, 0x00000000) // Black
	axisLabel := func(bp int, y int32) {
		text := fmt.Sprintf("%d %%", bp/100)
		if bp%100 != 0 {
			text = fmt.Sprintf("%d.%d %%", bp/100, (bp%100)/10)
		}
		drawText(text, rect{left: 0, top: y, right: chart.left - 6, bottom: y + 14}, 0x02) // DT_RIGHT
	}
	axisLabel(scaleBP, chart.top)
	axisLabel(scaleBP/2, chart.top+chartH/2-7)
	axisLabel(0, chart.bottom-14)

	procSelectObject.Call(hdc, oldFont)
	procDeleteObject.Call(font)
}
