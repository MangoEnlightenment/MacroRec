//go:build windows

package main

// Window behavior: layout, mouse handling, animation timing, file dialogs.
// All of this runs on the UI thread. Painting lives in ui_draw_windows.go.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	logicalW = 400
	logicalH = 688
	timerID  = 1
	nBars    = 54
)

// clickable widget ids
const (
	wRec = iota + 1
	wPlay
	wGame
	wDesktop
	wSpdM
	wSpdP
	wLoopM
	wLoopP
	wDelM
	wDelP
	wBumpOff
	wBumpStop
	wBumpFix
	wOpen
	wSave
	wPin
)

type widget struct {
	id         int
	x, y, w, h float64
}

var ui struct {
	hwnd        uintptr
	cv          *canvas
	scale       float64
	widgets     []widget
	hover, down int
	tracking    bool
	anim        map[int]float64 // per-widget hover animation, 0..1
	seg         float64         // animated mode switch, 0 = game, 1 = desktop
	phase       float64         // drives pulsing while active

	desktop  bool
	speed    float64
	loops    int
	delaySec int
	pin      bool
	bumpMode int     // 0 = off, 1 = stop, 2 = snap back and keep going
	bumpPos  float64 // animated position of the Off | Stop | Fix control

	profile   []float64
	profVer   int32
	live      []float64
	liveCount int32
	lastLive  time.Time
	prevState int32
	lastTick  time.Time

	cursorArrow, cursorHand uintptr
}

func initUI(scale float64) {
	ui.scale = scale
	ui.anim = map[int]float64{}
	ui.speed, ui.loops, ui.delaySec = 1, 1, 3
	ui.bumpMode, ui.bumpPos = 2, 2
	ui.profile = make([]float64, nBars)
	ui.live = make([]float64, nBars)
	ui.profVer = -1
	ui.lastTick = time.Now()
	ui.widgets = buildLayout()
}

func buildLayout() []widget {
	ws := []widget{
		{wRec, 20, 252, 174, 56},
		{wPlay, 206, 252, 174, 56},
		{wGame, 212, 358, 84, 32},
		{wDesktop, 296, 358, 84, 32},
	}
	for i, ids := range [][2]int{{wSpdM, wSpdP}, {wLoopM, wLoopP}, {wDelM, wDelP}} {
		y := 404 + float64(i)*46
		ws = append(ws, widget{ids[0], 262, y, 32, 32}, widget{ids[1], 348, y, 32, 32})
	}
	return append(ws,
		widget{wBumpOff, 212, 542, 56, 32},
		widget{wBumpStop, 268, 542, 56, 32},
		widget{wBumpFix, 324, 542, 56, 32},
		widget{wOpen, 20, 608, 112, 36},
		widget{wSave, 144, 608, 112, 36},
		widget{wPin, 268, 608, 112, 36})
}

func widgetByID(id int) widget {
	for _, w := range ui.widgets {
		if w.id == id {
			return w
		}
	}
	return widget{}
}

func hitTest(x, y float64) int {
	for _, w := range ui.widgets {
		if x >= w.x && x < w.x+w.w && y >= w.y && y < w.y+w.h {
			return w.id
		}
	}
	return 0
}

// ---- actions ----

func params() playParams {
	return playParams{
		desktop: ui.desktop,
		speed:   ui.speed,
		loops:   ui.loops,
		delay:   time.Duration(ui.delaySec) * time.Second,
		bump:    ui.bumpMode,
	}
}

func toggleRecord(fromClick bool) {
	switch eng.state.Load() {
	case idle:
		startRecording()
	case recording:
		stopRecording(fromClick)
	default:
		setStatus("Stop playback first", true)
	}
}

func togglePlay() {
	switch eng.state.Load() {
	case idle:
		startPlayback(params())
	case playing:
		requestStop()
	default:
		setStatus("Stop recording first", true)
	}
}

func setPin(on bool) {
	after := ^uintptr(1) // HWND_NOTOPMOST
	if on {
		after = ^uintptr(0) // HWND_TOPMOST
	}
	pSetWindowPos.Call(ui.hwnd, after, 0, 0, 0, 0, 0x13) // NOMOVE | NOSIZE | NOACTIVATE
}

func activate(id int) {
	switch id {
	case wRec:
		toggleRecord(true)
	case wPlay:
		togglePlay()
	case wGame:
		ui.desktop = false
	case wDesktop:
		ui.desktop = true
	case wSpdM:
		ui.speed = stepSpeed(ui.speed, -1)
	case wSpdP:
		ui.speed = stepSpeed(ui.speed, 1)
	case wLoopM:
		ui.loops = stepLoops(ui.loops, -1)
	case wLoopP:
		ui.loops = stepLoops(ui.loops, 1)
	case wDelM:
		if ui.delaySec > 0 {
			ui.delaySec--
		}
	case wDelP:
		if ui.delaySec < 10 {
			ui.delaySec++
		}
	case wOpen:
		openMacro()
	case wSave:
		saveMacroAs()
	case wBumpOff:
		ui.bumpMode = 0
	case wBumpStop:
		ui.bumpMode = 1
	case wBumpFix:
		ui.bumpMode = 2
	case wPin:
		ui.pin = !ui.pin
		setPin(ui.pin)
	}
}

func openMacro() {
	if eng.state.Load() != idle {
		setStatus("Stop what is running first", true)
		return
	}
	warn := "Only open macro files you made yourself, or that came from someone you know and trust.\n\n" +
		"A macro file is a list of key presses and mouse clicks that MacroRec performs on this PC as if you typed them. " +
		"A file from a stranger could open programs, type commands, change settings or delete files, and it would run with your permissions.\n\n" +
		"MacroRec checks that the file is a valid macro, but it cannot tell whether what the macro does is safe.\n\n" +
		"Open this file anyway?"
	// Yes/No, warning icon, and No is the default button.
	if messageBox(ui.hwnd, warn, "Open a macro file", 0x4|0x30|0x100) != 6 {
		return
	}
	path, ok := fileDialog(false)
	if !ok {
		return
	}
	if err := loadMacro(path); err != nil {
		setStatus("Could not open: "+err.Error(), true)
		return
	}
	setStatus(fmt.Sprintf("Opened %s events · %s", commas(realCount(eng.events)), formatTime(eng.totalUS.Load())), false)
}

func saveMacroAs() {
	switch {
	case eng.state.Load() == recording:
		setStatus("Stop recording first", true)
		return
	case realCount(eng.events) == 0:
		setStatus("Nothing to save yet", true)
		return
	}
	path, ok := fileDialog(true)
	if !ok {
		return
	}
	if err := saveMacro(path); err != nil {
		setStatus("Could not save: "+err.Error(), true)
		return
	}
	setStatus("Saved to "+filepath.Base(path), false)
}

func fileDialog(save bool) (string, bool) {
	buf := make([]uint16, 1024)
	if save {
		copy(buf, utf16.Encode([]rune("macro.txt")))
	}
	filter := utf16.Encode([]rune("Macro files (*.txt)\x00*.txt\x00All files (*.*)\x00*.*\x00\x00"))
	var ofn openFileName
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	ofn.hwndOwner = ui.hwnd
	ofn.lpstrFilter = &filter[0]
	ofn.nFilterIndex = 1
	ofn.lpstrFile = &buf[0]
	ofn.nMaxFile = uint32(len(buf))
	ofn.lpstrDefExt = u16("txt")
	ofn.flags = 0x80000 | 0x800 | 0x8 | 0x4 // EXPLORER | PATHMUSTEXIST | NOCHANGEDIR | HIDEREADONLY
	var r uintptr
	if save {
		ofn.flags |= 0x2 // OVERWRITEPROMPT
		r, _, _ = pGetSaveFileName.Call(uintptr(unsafe.Pointer(&ofn)))
	} else {
		ofn.flags |= 0x1000 // FILEMUSTEXIST
		r, _, _ = pGetOpenFileName.Call(uintptr(unsafe.Pointer(&ofn)))
	}
	ui.hover, ui.down = 0, 0
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

// ---- mouse ----

func mouseLogical(lp uintptr) (float64, float64) {
	return float64(loword(lp)) / ui.scale, float64(hiword(lp)) / ui.scale
}

func onMouseMove(lp uintptr) {
	x, y := mouseLogical(lp)
	ui.hover = hitTest(x, y)
	if !ui.tracking {
		tme := trackMouseEvent{dwFlags: 2, hwndTrack: ui.hwnd} // TME_LEAVE
		tme.cbSize = uint32(unsafe.Sizeof(tme))
		pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
		ui.tracking = true
	}
}

func onMouseDown(lp uintptr) {
	x, y := mouseLogical(lp)
	ui.down = hitTest(x, y)
	pSetCapture.Call(ui.hwnd)
}

func onMouseUp(lp uintptr) {
	pReleaseCapture.Call()
	x, y := mouseLogical(lp)
	id := hitTest(x, y)
	down := ui.down
	ui.down = 0
	ui.hover = id
	if id != 0 && id == down {
		activate(id)
	}
}

// ---- animation ----

func onTick() {
	now := time.Now()
	dt := now.Sub(ui.lastTick).Seconds()
	ui.lastTick = now
	if dt > 0.1 {
		dt = 0.1
	}
	k := 1 - math.Exp(-dt*16) // frame-rate independent easing
	dirty := false
	approach := func(cur, target float64) float64 {
		nv := cur + (target-cur)*k
		if math.Abs(nv-target) < 0.003 {
			nv = target
		}
		if nv != cur {
			dirty = true
		}
		return nv
	}
	for _, w := range ui.widgets {
		t := 0.0
		if ui.hover == w.id {
			t = 1
		}
		ui.anim[w.id] = approach(ui.anim[w.id], t)
	}
	segTarget := 0.0
	if ui.desktop {
		segTarget = 1
	}
	ui.seg = approach(ui.seg, segTarget)
	ui.bumpPos = approach(ui.bumpPos, float64(ui.bumpMode))

	st := eng.state.Load()
	if st != idle {
		ui.phase += dt
		dirty = true
	}
	if st == recording {
		if ui.prevState != recording {
			for i := range ui.live {
				ui.live[i] = 0
			}
			ui.liveCount = 0
			ui.lastLive = now
		}
		if now.Sub(ui.lastLive) >= 100*time.Millisecond {
			ui.lastLive = now
			c := eng.count.Load()
			copy(ui.live, ui.live[1:])
			ui.live[nBars-1] = float64(c - ui.liveCount)
			ui.liveCount = c
		}
	}
	if st != ui.prevState {
		dirty = true
	}
	ui.prevState = st

	if v := eng.version.Load(); v != ui.profVer {
		ui.profVer = v
		ui.profile = activityProfile(eng.events, nBars)
		dirty = true
	}
	if s := status.Load(); s != nil && now.Sub(s.at) < 7*time.Second {
		dirty = true
	}
	if dirty {
		pInvalidateRect.Call(ui.hwnd, 0, 0)
	}
}

// ---- window procedure ----

func wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case wmInput:
		switch eng.state.Load() {
		case recording:
			onRawInput(lp)
		case playing:
			onRawWhilePlaying(lp)
		}
		// fall through to DefWindowProc, which Windows requires for WM_INPUT
	case wmHotkey:
		switch wp {
		case hkRecord:
			toggleRecord(false)
		case hkPlay:
			togglePlay()
		}
		return 0
	case wmTimer:
		onTick()
		return 0
	case wmPaint:
		paint(hwnd)
		return 0
	case wmEraseBkgnd:
		return 1
	case wmMouseMove:
		onMouseMove(lp)
		return 0
	case wmMouseLeave:
		ui.hover, ui.tracking = 0, false
		return 0
	case wmLButtonDown:
		onMouseDown(lp)
		return 0
	case wmLButtonUp:
		onMouseUp(lp)
		return 0
	case wmSetCursor:
		if loword(lp) == 1 && ui.hover != 0 { // HTCLIENT over a widget
			pSetCursor.Call(ui.cursorHand)
			return 1
		}
	case wmDestroy:
		if eng.state.Load() == recording {
			stopRecording(false)
		}
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

func paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if ui.cv == nil {
		var rc winRect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		ui.cv = newCanvas(hdc, rc.right-rc.left, rc.bottom-rc.top, ui.scale)
	}
	drawUI(ui.cv)
	ui.cv.end(hdc)
	pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

// firstRunNotice shows the macro-file safety popup once, the first time the
// app is opened, and remembers that it was shown.
func firstRunNotice() {
	marker := filepath.Join(filepath.Dir(macroPath()), "notice-seen")
	if _, err := os.Stat(marker); err == nil {
		return
	}
	messageBox(ui.hwnd,
		"Quick safety tip\n\n"+
			"Only open macro files you made yourself, or that came from people you know and trust.\n\n"+
			"A macro file is a list of key presses and clicks that MacroRec performs on your PC as if you typed them. "+
			"A file from a stranger could open programs, type commands or delete files.\n\n"+
			"MacroRec will remind you each time you open a file.",
		"Welcome to MacroRec", 0x40) // OK + information icon
	_ = os.WriteFile(marker, []byte("1"), 0o644)
}
