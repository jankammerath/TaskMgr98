package main

import (
	"syscall"
	"unsafe"
)

// netColumn describes one selectable Networking-tab column.
type netColumn struct {
	title   string
	width   int32
	fmt     int32
	enabled bool
	locked  bool // Network Adapter Name can't be deselected
	value   func(a *netAdapter) string
}

// netColumns is the master list backing both the list view and the Select
// Columns dialog (in the dialog's left-then-right reading order).
var netColumns = []*netColumn{
	{title: "Adapter Name", width: 140, fmt: lvcfmtLeft, enabled: true, locked: true,
		value: func(a *netAdapter) string { return a.name }},
	{title: "Adapter Description", width: 160, fmt: lvcfmtLeft,
		value: func(a *netAdapter) string { return a.desc }},
	{title: "Network Utilization", width: 110, fmt: lvcfmtRight, enabled: true,
		value: func(a *netAdapter) string { return formatUtilBP(a.utilBP) }},
	{title: "Link Speed", width: 80, fmt: lvcfmtRight, enabled: true,
		value: func(a *netAdapter) string { return formatLinkSpeed(a.linkSpeed) }},
	{title: "State", width: 110, fmt: lvcfmtLeft, enabled: true,
		value: func(a *netAdapter) string {
			if a.connected {
				return "Operational"
			}
			return "Non Operational"
		}},
	{title: "Bytes Sent Throughput", width: 120, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatUtilBP(a.sentBP) }},
	{title: "Bytes Rcvd Throughput", width: 120, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatUtilBP(a.recvBP) }},
	{title: "Bytes Throughput", width: 110, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatUtilBP(a.utilBP) }},
	{title: "Bytes Sent", width: 100, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.bytesSent) }},
	{title: "Bytes Received", width: 100, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.bytesRecv) }},
	{title: "Bytes", width: 110, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.bytesSent + a.bytesRecv) }},
	{title: "Bytes Sent/Interval", width: 110, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.sentDelta) }},
	{title: "Bytes Rcvd/Interval", width: 110, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.recvDelta) }},
	{title: "Bytes/Interval", width: 100, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.sentDelta + a.recvDelta) }},
	{title: "Unicasts Sent", width: 95, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.ucastSent) }},
	{title: "Unicasts Received", width: 105, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.ucastRecv) }},
	{title: "Unicasts", width: 90, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.ucastSent + a.ucastRecv) }},
	{title: "Unicasts Sent/Interval", width: 120, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.ucastSentDelta) }},
	{title: "Unicasts Rcvd/Interval", width: 120, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.ucastRecvDelta) }},
	{title: "Unicasts/Interval", width: 105, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.ucastSentDelta + a.ucastRecvDelta) }},
	{title: "Nonunicasts Sent", width: 105, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.nucastSent) }},
	{title: "Nonunicasts Received", width: 120, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.nucastRecv) }},
	{title: "Nonunicasts", width: 95, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.nucastSent + a.nucastRecv) }},
	{title: "Nonunicasts Sent/Interval", width: 130, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.nucastSentDelta) }},
	{title: "Nonunicasts Rcvd/Interval", width: 130, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.nucastRecvDelta) }},
	{title: "Nonunicasts/Interval", width: 115, fmt: lvcfmtRight,
		value: func(a *netAdapter) string { return formatNum(a.nucastSentDelta + a.nucastRecvDelta) }},
}

// dialog checkbox labels (full names, matching the real dialog)
var netColumnLabels = []string{
	"Network Adapter Name",
	"Adapter Description",
	"Network Utilization",
	"Link Speed",
	"State",
	"Bytes Sent Throughput",
	"Bytes Received Throughput",
	"Bytes Throughput",
	"Bytes Sent",
	"Bytes Received",
	"Bytes",
	"Bytes Sent/Interval",
	"Bytes Received/Interval",
	"Bytes/Interval",
	"Unicasts Sent",
	"Unicasts Received",
	"Unicasts",
	"Unicasts Sent/Interval",
	"Unicasts Received/Interval",
	"Unicasts/Interval",
	"Nonunicasts Sent",
	"Nonunicasts Received",
	"Nonunicasts",
	"Nonunicasts Sent/Interval",
	"Nonunicasts Received/Interval",
	"Nonunicasts/Interval",
}

func enabledNetColumns() []*netColumn {
	var cols []*netColumn
	for _, c := range netColumns {
		if c.enabled {
			cols = append(cols, c)
		}
	}
	return cols
}

// rebuildNetColumns replaces the network list view's columns with the enabled set.
func rebuildNetColumns() {
	if hwndNetList == 0 {
		return
	}
	for {
		ok, _, _ := procSendMessage.Call(uintptr(hwndNetList), lvmDeleteColumn, 0, 0)
		if ok == 0 {
			break
		}
	}
	for i, c := range enabledNetColumns() {
		text, _ := syscall.UTF16PtrFromString(c.title)
		col := lvColumnW{
			mask:     lvcfFmt | lvcfWidth | lvcfText | lvcfSubItem,
			fmt:      c.fmt,
			cx:       c.width,
			pszText:  text,
			iSubItem: int32(i),
		}
		procSendMessage.Call(uintptr(hwndNetList), lvmInsertColumnW, uintptr(i), uintptr(unsafe.Pointer(&col)))
	}
}

// showNetColumnsDialog opens the modal "Select Columns" dialog for the Networking tab.
func showNetColumnsDialog(owner syscall.Handle) {
	spec := &columnsDialogSpec{
		intro:  "Select the columns that will appear on the Networking page of the Task Manager.",
		labels: netColumnLabels,
	}
	for _, c := range netColumns {
		spec.checked = append(spec.checked, c.enabled)
		spec.locked = append(spec.locked, c.locked)
	}
	spec.apply = func(states []bool) {
		for i := range netColumns {
			if !netColumns[i].locked {
				netColumns[i].enabled = states[i]
			}
		}
		rebuildNetColumns()
		refreshNetData()
	}
	showSelectColumnsDialog(owner, spec)
}
