package main

import (
	"syscall"
	"unsafe"
)

// GUID mirrors the Windows GUID structure.
type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// propertyKey mirrors PROPERTYKEY from wtypes.h
type propertyKey struct {
	fmtid guid
	pid   uint32
}

// propVariant mirrors PROPVARIANT (first 24 bytes on x64).
type propVariant struct {
	vt         uint16
	wReserved1 uint16
	wReserved2 uint16
	wReserved3 uint16
	valPtr     uintptr
	valPtr2    uintptr
}

type win32Size struct {
	cx int32
	cy int32
}

// iconInfo mirrors ICONINFO.
type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  syscall.Handle
	hbmColor syscall.Handle
}

const (
	vtLpwstr = 31

	sifMedium     = 0x00
	sifSmall      = 0x01
	sifFormatMask = 0x02

	coInitApartmentThreaded = 0x2
)

var (
	ole32                           = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx              = ole32.NewProc("CoInitializeEx")
	procPropVariantClear            = ole32.NewProc("PropVariantClear")
	procSHGetPropertyStoreForWindow = shell32.NewProc("SHGetPropertyStoreForWindow")
	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")
	procCreateIconIndirect          = user32.NewProc("CreateIconIndirect")
	procDeleteObject                = gdi32.NewProc("DeleteObject")

	// PKEY_AppUserModel_ID: {9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3}, 5
	pkeyAppUserModelID = propertyKey{
		fmtid: guid{0x9F4C2855, 0x9F79, 0x4B39, [8]byte{0xA8, 0xD0, 0xE1, 0xD4, 0x2D, 0xE1, 0xD5, 0xF3}},
		pid:   5,
	}

	// IID_IPropertyStore: {886d8eeb-8cf2-4446-8d02-cdba1dbdcf99}
	iidIPropertyStore = guid{0x886d8eeb, 0x8cf2, 0x4446, [8]byte{0x8d, 0x02, 0xcd, 0xba, 0x1d, 0xbd, 0xcf, 0x99}}

	// IID_IShellItemImageFactory: {bcc18b79-ba16-442f-80c4-8a59c30c463b}
	iidIShellItemImageFactory = guid{0xbcc18b79, 0xba16, 0x442f, [8]byte{0x80, 0xc4, 0x8a, 0x59, 0xc3, 0x0c, 0x46, 0x3b}}
)

// getUwpWindowIcon extracts the modern shell icon for packaged apps (e.g. Calculator)
// using the window's AppUserModelID and IShellItemImageFactory.
func getUwpWindowIcon(hwnd syscall.Handle) syscall.Handle {
	procCoInitializeEx.Call(0, coInitApartmentThreaded)

	var pStore uintptr
	hr, _, _ := procSHGetPropertyStoreForWindow.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(&iidIPropertyStore)),
		uintptr(unsafe.Pointer(&pStore)),
	)
	if hr != 0 || pStore == 0 {
		return 0
	}

	// IPropertyStore vtable: QueryInterface(0), AddRef(1), Release(2), GetCount(3), GetAt(4), GetValue(5), SetValue(6), Commit(7)
	storeVtbl := *(**[8]uintptr)(unsafe.Pointer(&pStore))
	releaseStore := func() {
		syscall.SyscallN(storeVtbl[2], pStore)
	}
	defer releaseStore()

	var pv propVariant
	hr, _, _ = syscall.SyscallN(
		storeVtbl[5],
		pStore,
		uintptr(unsafe.Pointer(&pkeyAppUserModelID)),
		uintptr(unsafe.Pointer(&pv)),
	)
	if hr != 0 || pv.vt != vtLpwstr || pv.valPtr == 0 {
		procPropVariantClear.Call(uintptr(unsafe.Pointer(&pv)))
		return 0
	}
	defer procPropVariantClear.Call(uintptr(unsafe.Pointer(&pv)))

	// Construct shell:AppsFolder\<AppUserModelID>
	strPtr := *(**[1024]uint16)(unsafe.Pointer(&pv.valPtr))
	aumidStr := syscall.UTF16ToString(strPtr[:])
	parsingName, _ := syscall.UTF16PtrFromString("shell:AppsFolder\\" + aumidStr)

	var pImageFactory uintptr
	hr, _, _ = procSHCreateItemFromParsingName.Call(
		uintptr(unsafe.Pointer(parsingName)),
		0,
		uintptr(unsafe.Pointer(&iidIShellItemImageFactory)),
		uintptr(unsafe.Pointer(&pImageFactory)),
	)
	if hr != 0 || pImageFactory == 0 {
		return 0
	}

	// IShellItemImageFactory vtable: QueryInterface(0), AddRef(1), Release(2), GetImage(3)
	factoryVtbl := *(**[4]uintptr)(unsafe.Pointer(&pImageFactory))
	defer syscall.SyscallN(factoryVtbl[2], pImageFactory)

	// Request standard 16x16 / 32x32 small icon dimensions
	iconSize := win32Size{cx: 16, cy: 16}
	var hBmp uintptr
	hr, _, _ = syscall.SyscallN(
		factoryVtbl[3],
		pImageFactory,
		*(*uintptr)(unsafe.Pointer(&iconSize)),
		sifSmall,
		uintptr(unsafe.Pointer(&hBmp)),
	)
	if hr != 0 || hBmp == 0 {
		return 0
	}
	defer procDeleteObject.Call(hBmp)

	// Convert the HBITMAP to an HICON
	ii := iconInfo{
		fIcon:    1,
		hbmColor: syscall.Handle(hBmp),
		hbmMask:  syscall.Handle(hBmp),
	}
	hIcon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	return syscall.Handle(hIcon)
}

// getWindowIcon fetches a window's small icon, checking messages, class icons,
// modern UWP/Shell properties (for Calculator), and executable file associations.
func getWindowIcon(hwnd syscall.Handle) syscall.Handle {
	// 1. Try standard window messages
	tryMessage := func(wParam uintptr) syscall.Handle {
		var result uintptr
		ret, ok := safeCall(
			procSendMessageTimeout,
			uintptr(hwnd),
			wmGeticon,
			wParam,
			0,
			smtoAbortIfHung,
			100,
			uintptr(unsafe.Pointer(&result)),
		)
		if ok && ret != 0 && result != 0 {
			return syscall.Handle(result)
		}
		return 0
	}

	if h := tryMessage(iconSmall2); h != 0 {
		return h
	}
	if h := tryMessage(iconSmall); h != 0 {
		return h
	}
	if h := tryMessage(iconBig); h != 0 {
		return h
	}

	// 2. Try window class icons (registered in WNDCLASSEX)
	tryClassIcon := func(index int32) syscall.Handle {
		h, ok := safeCall(procGetClassLongPtrW, uintptr(hwnd), uintptr(index))
		if !ok {
			return 0
		}
		return syscall.Handle(h)
	}

	if h := tryClassIcon(gclpHiconsm); h != 0 {
		return h
	}
	if h := tryClassIcon(gclpHicon); h != 0 {
		return h
	}

	// 3. Try packaged app extraction (UWP / WinUI Calculator, Settings, etc.)
	if h := getUwpWindowIcon(hwnd); h != 0 {
		return h
	}

	// 4. Fallback: Extract from the executable file path
	var pid uint32
	procGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pid)))
	if pid != 0 {
		if hProc, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid)); hProc != 0 {
			defer procCloseHandle.Call(hProc)

			var buf [1024]uint16
			size := uint32(len(buf))
			if ok, _, _ := procQueryFullProcessImageNameW.Call(hProc, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); ok != 0 {
				var sfi shFileInfoW
				flags := shgfiIcon | shgfiSmallIcon
				ret, _, _ := procSHGetFileInfoW.Call(
					uintptr(unsafe.Pointer(&buf[0])),
					0,
					uintptr(unsafe.Pointer(&sfi)),
					unsafe.Sizeof(sfi),
					uintptr(flags),
				)
				if ret != 0 && sfi.hIcon != 0 {
					return sfi.hIcon
				}
			}
		}
	}

	// 5. Default generic application icon (32512)
	if fallbackIcon == 0 {
		h, _, _ := procLoadIconW.Call(0, uintptr(32512))
		fallbackIcon = syscall.Handle(h)
	}
	return fallbackIcon
}
