//go:build windows

package main

// Thin bindings to the Win32 API using only the Go standard library.

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	winmm    = syscall.NewLazyDLL("winmm.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")

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
