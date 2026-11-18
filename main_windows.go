//go:build windows

// MacroRec: a macro recorder with a native Windows interface.
//
// It records keyboard, mouse buttons, scroll wheel and raw relative mouse
// movement (the camera movement games read), then replays them. It never
// touches another program: no process access, no memory access, no DLL
// injection. It only uses Raw Input to observe and SendInput to replay.
package main

import (
	"math"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func init() { runtime.LockOSThread() } // the window and its message loop stay on one OS thread

var (
	pLoadImage        = user32.NewProc("LoadImageW")
	pGetSystemMetrics = user32.NewProc("GetSystemMetrics")
)

// loadAppIcon loads the icon embedded in the exe (resource id 1) at the size
// Windows wants for the big (taskbar) or small (title bar) icon. If it is
// missing the window simply gets the default icon.
func loadAppIcon(hInst uintptr, small bool) uintptr {
	cx, cy := uintptr(11), uintptr(12) // SM_CXICON, SM_CYICON
	if small {
		cx, cy = 49, 50 // SM_CXSMICON, SM_CYSMICON
	}
	w, _, _ := pGetSystemMetrics.Call(cx)
	h, _, _ := pGetSystemMetrics.Call(cy)
	icon, _, _ := pLoadImage.Call(hInst, 1, 1, w, h, 0) // resource id 1, IMAGE_ICON
	return icon
}

func fail(msg string) {
	messageBox(0, msg, "MacroRec", 0x10) // error icon
	os.Exit(1)
}

func main() {
	restrictDLLSearch()        // before any library is used
	pSetProcessDPIAware.Call() // crisp on high-DPI screens
	pTimeBeginPeriod.Call(1)   // accurate timing for playback
	scale := dpiScale()
	initUI(scale)
	if err := gdipStart(); err != nil {
		fail(err.Error())
	}

	className := u16("MacroRecMain")
	hInst, _, _ := pGetModuleHandle.Call(0)
	ui.cursorArrow, _, _ = pLoadCursor.Call(0, idcArrow)
	ui.cursorHand, _, _ = pLoadCursor.Call(0, idcHand)
	wc := wndClassEx{
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     hInst,
		hIcon:         loadAppIcon(hInst, false), // window, taskbar and Alt+Tab
		hIconSm:       loadAppIcon(hInst, true),  // title bar
		hCursor:       ui.cursorArrow,
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if r, _, e := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		fail("Could not create the window class: " + e.Error())
	}

	// fixed-size window: caption + system menu + minimize button
	const style = 0x00CA0000
	rc := winRect{0, 0, int32(math.Round(logicalW * scale)), int32(math.Round(logicalH * scale))}
	pAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&rc)), style, 0, 0)
	hwnd, _, e := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(u16("MacroRec"))), style,
		0x80000000, 0x80000000, // CW_USEDEFAULT
		uintptr(rc.right-rc.left), uintptr(rc.bottom-rc.top), 0, 0, hInst, 0)
	if hwnd == 0 {
		fail("Could not create the window: " + e.Error())
	}
	ui.hwnd = hwnd

	// dark title bar (Windows 10 2004+ / 11); harmlessly ignored elsewhere
	one := int32(1)
	pDwmSetWindowAttribute.Call(hwnd, 20, uintptr(unsafe.Pointer(&one)), 4)
	pDwmSetWindowAttribute.Call(hwnd, 19, uintptr(unsafe.Pointer(&one)), 4)
	capColor := uint32(0x0014100E) // COLORREF of the app background
	txtColor := uint32(0x00F3ECE9)
	pDwmSetWindowAttribute.Call(hwnd, 35, uintptr(unsafe.Pointer(&capColor)), 4)
	pDwmSetWindowAttribute.Call(hwnd, 34, uintptr(unsafe.Pointer(&capColor)), 4)
	pDwmSetWindowAttribute.Call(hwnd, 36, uintptr(unsafe.Pointer(&txtColor)), 4)

	if err := registerInput(hwnd); err != nil {
		fail(err.Error())
	}
	registerHotkeys(hwnd)

	// bring back the last recording, if there is one
	if _, err := os.Stat(macroPath()); err == nil && loadMacro(macroPath()) == nil {
		setStatus("Loaded your last recording", false)
	}

	pSetTimer.Call(hwnd, timerID, 16, 0)
	pShowWindow.Call(hwnd, 5)
	firstRunNotice()

	var m winMsg
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
	shutdown()
}
