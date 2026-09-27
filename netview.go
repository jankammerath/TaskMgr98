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
	desc      string
	linkSpeed uint64 // bits/sec
	connected bool
	havePrev  bool

	// cumulative counters and per-interval deltas
	bytesSent, bytesRecv             uint64
	sentDelta, recvDelta             uint64
	ucastSent, ucastRecv             uint64
	ucastSentDelta, ucastRecvDelta   uint64
	nucastSent, nucastRecv           uint64
	nucastSentDelta, nucastRecvDelta uint64

	prevBytesSent, prevBytesRecv   uint64
	prevUcastSent, prevUcastRecv   uint64
	prevNucastSent, prevNucastRecv uint64

	// utilization in basis points (1/100 %) plus per-direction histories
	utilBP, sentBP, recvBP int
	history                []int
	sentHistory            []int
	recvHistory            []int
}

var (
	iphlpapi         = syscall.NewLazyDLL("iphlpapi.dll")
	procGetIfTable2  = iphlpapi.NewProc("GetIfTable2")
	procFreeMibTable = iphlpapi.NewProc("FreeMibTable")

	hwndNetContainer syscall.Handle
	hwndNetList      syscall.Handle
	hwndNetGroups    [maxNetGraphs]syscall.Handle
	hwndNetGraphs    [maxNetGraphs]syscall.Handle

	netAdapters        []*netAdapter
	netClassRegistered = false
	netViewVisible     = false

	// View > Network Adapter History line toggles
	netShowBytesSent  = false // red
	netShowBytesRecv  = false // yellow
	netShowBytesTotal = true  // green
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

	btnClass, _ := syscall.UTF16PtrFromString("BUTTON")
	graphClass, _ := syscall.UTF16PtrFromString("TaskMgr98NetGraph")
	for i := 0; i < maxNetGraphs; i++ {
		g, _, _ := procCreateWindowEx.Call(
			0,
			uintptr(unsafe.Pointer(btnClass)),
			0,
			uintptr(wsChild|bsGroupBox),
			0, 0, 0, 0,
			uintptr(hwndNetContainer), 0, hInstance, 0,
		)
		hwndNetGroups[i] = syscall.Handle(g)

		h, _, _ := procCreateWindowEx.Call(
			0x00000200, // WS_EX_CLIENTEDGE
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
	procSendMessage.Call(uintptr(hwndNetList), lvmSetExtendedListViewStyle, 0, lvsExFullRowSelect|lvsExHeaderDragDrop)
	rebuildNetColumns()

	return hwndNetContainer
}

// setNetFonts applies the UI font to the adapter list and group boxes (graphs
// paint their own text).
func setNetFonts(font uintptr) {
	if hwndNetContainer == 0 || font == 0 {
		return
	}
	procEnumChildWindows.Call(uintptr(hwndNetContainer), perfFontCallback, font)
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
		y := pad
		for i := int32(0); i < n; i++ {
			procMoveWindow.Call(uintptr(hwndNetGroups[i]), uintptr(pad), uintptr(y), uintptr(w-pad*2), uintptr(graphH), 1)
			procMoveWindow.Call(uintptr(hwndNetGraphs[i]), uintptr(pad+8), uintptr(y+18), uintptr(w-pad*2-16), uintptr(graphH-26), 1)
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
		ad.desc = syscall.UTF16ToString(row.description[:])
		ad.connected = row.mediaConnectState == mediaConnected && row.operStatus == 1

		speed := max(row.transmitLinkSpeed, row.receiveLinkSpeed)
		if speed == ^uint64(0) {
			speed = 0
		}
		ad.linkSpeed = speed

		ad.sentDelta, ad.recvDelta = 0, 0
		ad.ucastSentDelta, ad.ucastRecvDelta = 0, 0
		ad.nucastSentDelta, ad.nucastRecvDelta = 0, 0
		if ad.havePrev {
			ad.sentDelta = row.outOctets - ad.prevBytesSent
			ad.recvDelta = row.inOctets - ad.prevBytesRecv
			ad.ucastSentDelta = row.outUcastPkts - ad.prevUcastSent
			ad.ucastRecvDelta = row.inUcastPkts - ad.prevUcastRecv
			ad.nucastSentDelta = row.outNUcastPkts - ad.prevNucastSent
			ad.nucastRecvDelta = row.inNUcastPkts - ad.prevNucastRecv
		}
		ad.bytesSent, ad.bytesRecv = row.outOctets, row.inOctets
		ad.ucastSent, ad.ucastRecv = row.outUcastPkts, row.inUcastPkts
		ad.nucastSent, ad.nucastRecv = row.outNUcastPkts, row.inNUcastPkts

		ad.utilBP, ad.sentBP, ad.recvBP = 0, 0, 0
		if ad.havePrev && ad.connected && speed > 0 {
			toBP := func(bytes uint64) int {
				bp := bytes * 8 * 10000 * 1000 / (speed * uint64(updateIntervalMs))
				return min(int(bp), 10000)
			}
			ad.sentBP = toBP(ad.sentDelta)
			ad.recvBP = toBP(ad.recvDelta)
			ad.utilBP = toBP(ad.sentDelta + ad.recvDelta)
		}
		ad.prevBytesSent, ad.prevBytesRecv = row.outOctets, row.inOctets
		ad.prevUcastSent, ad.prevUcastRecv = row.outUcastPkts, row.inUcastPkts
		ad.prevNucastSent, ad.prevNucastRecv = row.outNUcastPkts, row.inNUcastPkts
		ad.havePrev = true

		appendHist := func(hist []int, v int) []int {
			hist = append(hist, v)
			if len(hist) > netHistoryMax {
				hist = hist[1:]
			}
			return hist
		}
		ad.history = appendHist(ad.history, ad.utilBP)
		ad.sentHistory = appendHist(ad.sentHistory, ad.sentBP)
		ad.recvHistory = appendHist(ad.recvHistory, ad.recvBP)

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
			title, _ := syscall.UTF16PtrFromString(netAdapters[i].name)
			procSendMessage.Call(uintptr(hwndNetGroups[i]), 0x000C, 0, uintptr(unsafe.Pointer(title))) // WM_SETTEXT
			procShowWindow.Call(uintptr(hwndNetGroups[i]), swShowNoActivate)
			procShowWindow.Call(uintptr(hwndNetGraphs[i]), swShowNoActivate)
			procInvalidateRect.Call(uintptr(hwndNetGraphs[i]), 0, 0)
		} else {
			procShowWindow.Call(uintptr(hwndNetGroups[i]), swHide)
			procShowWindow.Call(uintptr(hwndNetGraphs[i]), swHide)
		}
	}
}

// insertNetRow adds one row to the list view, filling every enabled column.
func insertNetRow(index int32, ad *netAdapter) {
	cols := enabledNetColumns()
	if len(cols) == 0 {
		return
	}
	firstPtr, _ := syscall.UTF16PtrFromString(cols[0].value(ad))
	item := lvItemW{mask: lvifText, iItem: index, pszText: firstPtr}
	procSendMessage.Call(uintptr(hwndNetList), lvmInsertItemW, 0, uintptr(unsafe.Pointer(&item)))

	for sub := 1; sub < len(cols); sub++ {
		ptr, _ := syscall.UTF16PtrFromString(cols[sub].value(ad))
		subItem := lvItemW{mask: lvifText, iItem: index, iSubItem: int32(sub), pszText: ptr}
		procSendMessage.Call(uintptr(hwndNetList), lvmSetItemW, 0, uintptr(unsafe.Pointer(&subItem)))
	}
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

		blackBrush, _, _ := procCreateSolidBrush.Call(0x00000000)
		procFillRect.Call(memDC, uintptr(unsafe.Pointer(&rc)), blackBrush)
		procDeleteObject.Call(blackBrush)

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

	// Dark green grid
	gridPen, _, _ := procCreatePen.Call(psSolid, 1, 0x00005500)
	oldPen, _, _ := procSelectObject.Call(hdc, gridPen)

	gridSpacing := int32(12)
	for x := w - 1; x >= 0; x -= gridSpacing {
		procMoveToEx.Call(hdc, uintptr(x), 0, 0)
		procLineTo.Call(hdc, uintptr(x), uintptr(h))
	}
	for y := h - 1; y >= 0; y -= gridSpacing {
		procMoveToEx.Call(hdc, uintptr(0), uintptr(y), 0)
		procLineTo.Call(hdc, uintptr(w), uintptr(y))
	}

	// Series toggled via View > Network Adapter History.
	type series struct {
		vals  []int
		color uint32
	}
	var shown []series
	if netShowBytesTotal {
		shown = append(shown, series{ad.history, 0x0000FF00}) // green
	}
	if netShowBytesRecv {
		shown = append(shown, series{ad.recvHistory, 0x0000FFFF}) // yellow
	}
	if netShowBytesSent {
		shown = append(shown, series{ad.sentHistory, 0x000000FF}) // red
	}

	// Pick the smallest full-percent scale that fits the shown histories' peak.
	peak := 0
	for _, s := range shown {
		for _, v := range s.vals {
			peak = max(peak, v)
		}
	}
	scaleBP := 10000
	for _, s := range []int{100, 200, 500, 1000, 2500, 5000, 10000} {
		if peak <= s {
			scaleBP = s
			break
		}
	}

	for _, s := range shown {
		trendPen, _, _ := procCreatePen.Call(psSolid, 1, uintptr(s.color))
		procSelectObject.Call(hdc, trendPen)

		count := len(s.vals)
		for i := 0; i < count; i++ {
			val := min(max(s.vals[count-1-i], 0), scaleBP)

			x := (w - 1) - int32(i)*gridSpacing
			if x < 0 {
				break
			}
			y := (h - 1) - int32((int64(val)*int64(h-2))/int64(scaleBP))

			if i == 0 {
				procMoveToEx.Call(hdc, uintptr(x), uintptr(y), 0)
			} else {
				procLineTo.Call(hdc, uintptr(x), uintptr(y))
			}
		}

		procSelectObject.Call(hdc, oldPen)
		procDeleteObject.Call(trendPen)
	}

	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(gridPen)

	// Yellow y-axis labels overlaid on the chart's left edge
	procSetBkMode.Call(hdc, 1) // TRANSPARENT
	labelFont := logFont{lfHeight: -11, lfWeight: 400}
	faceName, _ := syscall.UTF16FromString("Segoe UI")
	copy(labelFont.lfFaceName[:], faceName)
	font, _, _ := procCreateFontIndirect.Call(uintptr(unsafe.Pointer(&labelFont)))
	oldFont, _, _ := procSelectObject.Call(hdc, font)

	procSetTextColor.Call(hdc, 0x0000FFFF) // Yellow
	axisLabel := func(bp int, y int32) {
		text := fmt.Sprintf("%d %%", bp/100)
		if bp%100 != 0 {
			text = fmt.Sprintf("%d.%02d %%", bp/100, bp%100)
		}
		buf, _ := syscall.UTF16FromString(text)
		r := rect{left: 3, top: y, right: w, bottom: y + 14}
		procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)-1), uintptr(unsafe.Pointer(&r)), 0x20) // DT_LEFT | DT_SINGLELINE
	}
	axisLabel(scaleBP, 2)
	axisLabel(scaleBP/2, h/2-7)
	axisLabel(0, h-16)

	procSelectObject.Call(hdc, oldFont)
	procDeleteObject.Call(font)
}
