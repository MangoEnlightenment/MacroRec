//go:build windows

package main

// Thin bindings to the Win32 API using only the Go standard library.

import (
	"syscall"
	"unsafe"
)

var (
	user32   = sysDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = sysDLL("gdi32.dll")
	winmm    = sysDLL("winmm.dll")
	dwmapi   = sysDLL("dwmapi.dll")
	comdlg32 = sysDLL("comdlg32.dll")

	pRegisterClassEx         = user32.NewProc("RegisterClassExW")
	pCreateWindowEx          = user32.NewProc("CreateWindowExW")
	pDefWindowProc           = user32.NewProc("DefWindowProcW")
	pGetMessage              = user32.NewProc("GetMessageW")
	pTranslateMessage        = user32.NewProc("TranslateMessage")
	pDispatchMessage         = user32.NewProc("DispatchMessageW")
	pPostQuitMessage         = user32.NewProc("PostQuitMessage")
	pShowWindow              = user32.NewProc("ShowWindow")
	pInvalidateRect          = user32.NewProc("InvalidateRect")
	pBeginPaint              = user32.NewProc("BeginPaint")
	pEndPaint                = user32.NewProc("EndPaint")
	pGetClientRect           = user32.NewProc("GetClientRect")
	pSetTimer                = user32.NewProc("SetTimer")
	pLoadCursor              = user32.NewProc("LoadCursorW")
	pSetCursor               = user32.NewProc("SetCursor")
	pTrackMouseEvent         = user32.NewProc("TrackMouseEvent")
	pSetCapture              = user32.NewProc("SetCapture")
	pReleaseCapture          = user32.NewProc("ReleaseCapture")
	pSetWindowPos            = user32.NewProc("SetWindowPos")
	pAdjustWindowRectEx      = user32.NewProc("AdjustWindowRectEx")
	pMessageBox              = user32.NewProc("MessageBoxW")
	pRegisterRawInputDevices = user32.NewProc("RegisterRawInputDevices")
	pGetRawInputData         = user32.NewProc("GetRawInputData")
	pRegisterHotKey          = user32.NewProc("RegisterHotKey")
	pSendInput               = user32.NewProc("SendInput")
	pGetCursorPos            = user32.NewProc("GetCursorPos")
	pSetCursorPos            = user32.NewProc("SetCursorPos")
	pSetProcessDPIAware      = user32.NewProc("SetProcessDPIAware")
	pGetDC                   = user32.NewProc("GetDC")
	pReleaseDC               = user32.NewProc("ReleaseDC")
	pGetModuleHandle         = kernel32.NewProc("GetModuleHandleW")
	pTimeBeginPeriod         = winmm.NewProc("timeBeginPeriod")
	pDwmSetWindowAttribute   = dwmapi.NewProc("DwmSetWindowAttribute")
	pGetOpenFileName         = comdlg32.NewProc("GetOpenFileNameW")
	pGetSaveFileName         = comdlg32.NewProc("GetSaveFileNameW")

	pGetDeviceCaps          = gdi32.NewProc("GetDeviceCaps")
	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pBitBlt                 = gdi32.NewProc("BitBlt")
)

const (
	wmDestroy     = 0x0002
	wmPaint       = 0x000F
	wmEraseBkgnd  = 0x0014
	wmSetCursor   = 0x0020
	wmInput       = 0x00FF
	wmTimer       = 0x0113
	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmMouseLeave  = 0x02A3
	wmHotkey      = 0x0312

	idcArrow = 32512
	idcHand  = 32649
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
}

type winRect struct{ left, top, right, bottom int32 }

type point struct{ x, y int32 }

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     winRect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type trackMouseEvent struct {
	cbSize      uint32
	dwFlags     uint32
	hwndTrack   uintptr
	dwHoverTime uint32
}

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
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

func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func loword(l uintptr) int { return int(int16(l & 0xFFFF)) }
func hiword(l uintptr) int { return int(int16((l >> 16) & 0xFFFF)) }

func messageBox(hwnd uintptr, text, title string, flags uintptr) int {
	r, _, _ := pMessageBox.Call(hwnd, uintptr(unsafe.Pointer(u16(text))), uintptr(unsafe.Pointer(u16(title))), flags)
	return int(r)
}

// dpiScale returns the system DPI scale (1.0 = 96 dpi).
func dpiScale() float64 {
	hdc, _, _ := pGetDC.Call(0)
	if hdc == 0 {
		return 1
	}
	dpi, _, _ := pGetDeviceCaps.Call(hdc, 88) // LOGPIXELSX
	pReleaseDC.Call(0, hdc)
	if dpi < 96 {
		return 1
	}
	return float64(dpi) / 96
}

// ---- DLL loading hardening ----
//
// Windows normally looks for a library by name in the app's own folder first,
// so a harmful DLL dropped next to the exe (for example in Downloads) could be
// loaded instead of the real one. To prevent that, every library is loaded by
// its full path in the Windows system folder, and the process is told to
// search only that folder for libraries. kernel32.dll is always already loaded
// by Windows before the program starts, so it is safe to load by name.

var pGetSystemDirectory = kernel32.NewProc("GetSystemDirectoryW")
var pSetDefaultDllDirectories = kernel32.NewProc("SetDefaultDllDirectories")

func systemDir() string {
	buf := make([]uint16, 260)
	n, _, _ := pGetSystemDirectory.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || int(n) >= len(buf) {
		return `C:\Windows\System32`
	}
	return syscall.UTF16ToString(buf[:n])
}

// sysDLL returns a library that is loaded from the Windows system folder only.
func sysDLL(name string) *syscall.LazyDLL {
	return syscall.NewLazyDLL(systemDir() + `\` + name)
}

// restrictDLLSearch limits where this process looks for libraries to the
// system folder (LOAD_LIBRARY_SEARCH_SYSTEM32). It must run before the first
// call into any library other than kernel32. Very old Windows versions do not
// have this function; in that case the full-path loading above still applies.
func restrictDLLSearch() {
	if pSetDefaultDllDirectories.Find() == nil {
		pSetDefaultDllDirectories.Call(0x00000800)
	}
}
