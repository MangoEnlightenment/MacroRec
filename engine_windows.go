//go:build windows

package main

// Recording (Raw Input) and playback (SendInput), with no console output.
// Nothing here touches another process: it only observes input through Raw
// Input and replays it through SendInput.

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

const (
	idle int32 = iota
	recording
	playing
)

const (
	hkRecord = 1
	hkPlay   = 2
	vkF8     = 0x77
	vkF9     = 0x78

	ridInput        = 0x10000003
	ridevInputSink  = 0x00000100 // receive input while another window has focus
	modNoRepeat     = 0x4000
	mouseeventfMove = 0x0001
)

var (
	le      = binary.LittleEndian
	hdrSize = int(8 + 2*unsafe.Sizeof(uintptr(0)))
)

type playParams struct {
	desktop bool
	speed   float64
	loops   int
	delay   time.Duration
	bump    int // 0 = off, 1 = stop if the real mouse moves, 2 = snap back and keep going
}

type statusEntry struct {
	text  string
	isErr bool
	at    time.Time
}

var eng struct {
	state        atomic.Int32
	stop         atomic.Int32
	count        atomic.Int32 // events recorded so far
	playPos      atomic.Int32
	playLoop     atomic.Int32
	playLoops    atomic.Int32
	phaseRunAt   atomic.Int64 // 0 while counting down, unix nanos once events are running
	countdownEnd atomic.Int64
	version      atomic.Int32 // bumps whenever the loaded macro changes
	totalUS      atomic.Int64

	bumpMode atomic.Int32 // 0 off, 1 stop, 2 snap back
	desktop  atomic.Bool
	expPos   atomic.Uint64 // where the macro expects the cursor (Desktop mode)
	stopMsg  atomic.Pointer[string]
	bumpAcc  int32     // UI thread only
	bumpLast time.Time // UI thread only

	recStart time.Time
	events   []event // changed only on the UI thread, never while playing
}

var status atomic.Pointer[statusEntry]

func setStatus(text string, isErr bool) {
	status.Store(&statusEntry{text: text, isErr: isErr, at: time.Now()})
}

func macroPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	d := filepath.Join(dir, "MacroRec")
	_ = os.MkdirAll(d, 0o755)
	return filepath.Join(d, "last-macro.txt")
}

func setEvents(evs []event) {
	eng.events = evs
	eng.count.Store(int32(realCount(evs)))
	if len(evs) > 0 {
		eng.totalUS.Store(evs[len(evs)-1].T)
	} else {
		eng.totalUS.Store(0)
	}
	eng.version.Add(1)
}

// ---- setup ----

type rawInputDevice struct {
	usagePage, usage uint16
	flags            uint32
	target           uintptr
}

func registerInput(hwnd uintptr) error {
	devs := []rawInputDevice{
		{1, 2, ridevInputSink, hwnd}, // mouse
		{1, 6, ridevInputSink, hwnd}, // keyboard
	}
	r, _, e := pRegisterRawInputDevices.Call(uintptr(unsafe.Pointer(&devs[0])), uintptr(len(devs)), unsafe.Sizeof(devs[0]))
	if r == 0 {
		return fmt.Errorf("could not listen for input: %v", e)
	}
	return nil
}

func registerHotkeys(hwnd uintptr) {
	for _, h := range []struct {
		id, vk uintptr
		name   string
	}{{hkRecord, vkF8, "F8"}, {hkPlay, vkF9, "F9"}} {
		if r, _, _ := pRegisterHotKey.Call(hwnd, h.id, modNoRepeat, h.vk); r == 0 {
			setStatus(h.name+" is used by another app. Use the on-screen buttons instead", true)
		}
	}
}

// ---- recording ----

func isControlKey(vk uint16) bool { return vk == vkF8 || vk == vkF9 }

func cursorPos() (int32, int32) {
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p.x, p.y
}

// readRaw fetches and decodes one Raw Input packet.
func readRaw(h uintptr) (rawEvent, bool) {
	var buf [64]byte
	size := uint32(len(buf))
	n, _, _ := pGetRawInputData.Call(h, ridInput,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), uintptr(hdrSize))
	if int32(n) <= 0 || int(n) > len(buf) {
		return rawEvent{}, false
	}
	return parseRaw(buf[:n], hdrSize)
}

// ---- "stop if the mouse moves" ----

// sent remembers the mouse input playback itself just generated, as a safety
// net in case it is ever reported like physical input.
var sent struct {
	mu    sync.Mutex
	moves [64]struct {
		dx, dy int32
		at     int64
	}
	next      int
	lastClick int64
}

func noteSentMove(dx, dy int32) {
	sent.mu.Lock()
	sent.moves[sent.next] = struct {
		dx, dy int32
		at     int64
	}{dx, dy, time.Now().UnixNano()}
	sent.next = (sent.next + 1) % len(sent.moves)
	sent.mu.Unlock()
}

func noteSentClick() {
	sent.mu.Lock()
	sent.lastClick = time.Now().UnixNano()
	sent.mu.Unlock()
}

func wasSentMove(dx, dy int32, now time.Time) bool {
	sent.mu.Lock()
	defer sent.mu.Unlock()
	for _, m := range sent.moves {
		if m.dx == dx && m.dy == dy && m.at != 0 && now.UnixNano()-m.at < 100_000_000 {
			return true
		}
	}
	return false
}

func wasSentClick(now time.Time) bool {
	sent.mu.Lock()
	defer sent.mu.Unlock()
	return sent.lastClick != 0 && now.UnixNano()-sent.lastClick < 100_000_000
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func stopWith(msg string) {
	eng.stopMsg.Store(&msg)
	eng.stop.Store(1)
}

func reportStopped() {
	if m := eng.stopMsg.Swap(nil); m != nil {
		setStatus(*m, false)
	} else {
		setStatus("Playback stopped", false)
	}
}

func storeExpPos(x, y int32)     { eng.expPos.Store(packPos(x, y)) }
func loadExpPos() (int32, int32) { return unpackPos(eng.expPos.Load()) }

// onRawWhilePlaying reacts when the real mouse is touched: stop, or snap back
// and keep going. Input the macro generates itself has no device handle, so it
// is ignored.
func onRawWhilePlaying(h uintptr) {
	mode := eng.bumpMode.Load()
	if mode == 0 {
		return
	}
	r, ok := readRaw(h)
	if !ok || r.Type != rawMouse || r.Device == 0 {
		return
	}
	run := eng.phaseRunAt.Load()
	now := time.Now()
	if run == 0 || now.UnixNano()-run < 700_000_000 {
		return // still counting down, or just started
	}
	if r.BtnFlags != 0 {
		if !wasSentClick(now) {
			stopWith("Stopped: you used the mouse")
		}
		return
	}
	if r.DX == 0 && r.DY == 0 || wasSentMove(r.DX, r.DY, now) {
		return
	}
	if mode == 2 { // snap back and keep going
		if eng.desktop.Load() {
			x, y := loadExpPos()
			setCursorPos(x, y) // back to where the macro wants the cursor
		} else {
			sendMouse(-r.DX, -r.DY, 0, mouseeventfMove) // cancel the bump
		}
		return
	}
	if now.Sub(eng.bumpLast) > 250*time.Millisecond {
		eng.bumpAcc = 0
	}
	eng.bumpLast = now
	eng.bumpAcc += abs32(r.DX) + abs32(r.DY)
	if eng.bumpAcc >= 40 { // ignore tiny jitters, catch a real bump
		stopWith("Stopped: you moved the mouse")
	}
}

func onRawInput(h uintptr) {
	r, ok := readRaw(h)
	if !ok {
		return
	}
	t := time.Since(eng.recStart).Microseconds()
	switch r.Type {
	case rawMouse:
		px, py := cursorPos()
		if r.DX != 0 || r.DY != 0 || r.Absolute {
			eng.events = append(eng.events, event{T: t, Kind: kindMove, A: r.DX, B: r.DY, X: px, Y: py})
		}
		if r.BtnFlags != 0 {
			eng.events = append(eng.events, event{T: t, Kind: kindBtn, A: int32(r.BtnFlags), B: int32(r.BtnData), X: px, Y: py})
		}
	case rawKey:
		if r.Skip || isControlKey(r.VK) {
			return
		}
		var fl int32
		if r.Up {
			fl |= 1
		}
		if r.Ext {
			fl |= 2
		}
		eng.events = append(eng.events, event{T: t, Kind: kindKey, A: int32(r.Scan), B: fl})
	}
	eng.count.Store(int32(realCount(eng.events)))
}

func startRecording() {
	if eng.state.Load() != idle {
		return
	}
	x, y := cursorPos()
	eng.events = []event{{Kind: kindStart, X: x, Y: y}} // where the mouse was when recording began
	eng.count.Store(0)
	eng.recStart = time.Now()
	eng.state.Store(recording)
	setStatus("Recording. Press F8 to stop", false)
}

// trimStopClick removes the approach and click on the Stop button when
// recording was ended with the mouse instead of the hotkey.
func trimStopClick() {
	evs := eng.events
	if len(evs) == 0 {
		return
	}
	end := evs[len(evs)-1].T
	for i := len(evs) - 1; i >= 0; i-- {
		e := evs[i]
		if e.Kind == kindBtn && e.A&0x0001 != 0 && end-e.T < 2_000_000 {
			cut := e.T - 350_000
			j := i
			for j > 0 && evs[j-1].T >= cut {
				j--
			}
			eng.events = evs[:j]
			return
		}
	}
}

func stopRecording(fromClick bool) {
	if eng.state.Load() != recording {
		return
	}
	eng.state.Store(idle)
	if fromClick {
		trimStopClick()
	}
	if realCount(eng.events) == 0 {
		setStatus("Nothing was recorded", true)
		setEvents(nil)
		return
	}
	normalize(eng.events)
	setEvents(eng.events)
	msg := fmt.Sprintf("Recorded %s events · %s", commas(realCount(eng.events)), formatTime(eng.totalUS.Load()))
	if err := saveMacro(macroPath()); err != nil {
		msg += " (could not auto-save)"
	}
	setStatus(msg, false)
}

// ---- files ----

func saveMacro(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeEvents(f, eng.events)
}

func loadMacro(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.Size() > 64<<20 {
		return fmt.Errorf("file is too large to be a macro")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	evs, err := readEvents(f)
	if err != nil {
		return err
	}
	if err := validateEvents(evs); err != nil {
		return err
	}
	normalize(evs)
	setEvents(evs)
	return nil
}

// ---- playback ----

type inputRec struct {
	_   [0]uint64 // 8-byte alignment, like the Win32 INPUT struct
	typ uint32
	_   uint32
	u   [32]byte
}

func sendRaw(in *inputRec) {
	pSendInput.Call(1, uintptr(unsafe.Pointer(in)), unsafe.Sizeof(*in))
}

func sendMouse(dx, dy int32, data, flags uint32) {
	if flags&mouseeventfMove != 0 {
		noteSentMove(dx, dy)
	} else {
		noteSentClick()
	}
	var in inputRec
	in.typ = 0 // INPUT_MOUSE
	le.PutUint32(in.u[0:], uint32(dx))
	le.PutUint32(in.u[4:], uint32(dy))
	le.PutUint32(in.u[8:], data)
	le.PutUint32(in.u[12:], flags)
	sendRaw(&in)
}

// Keys are replayed by scan code, which is what games read.
func sendKey(scan uint16, up, ext bool) {
	var in inputRec
	in.typ = 1              // INPUT_KEYBOARD
	flags := uint32(0x0008) // KEYEVENTF_SCANCODE
	if up {
		flags |= 0x0002
	}
	if ext {
		flags |= 0x0001
	}
	le.PutUint16(in.u[2:], scan)
	le.PutUint32(in.u[4:], flags)
	sendRaw(&in)
}

func setCursorPos(x, y int32) { pSetCursorPos.Call(uintptr(x), uintptr(y)) }

// held tracks what is pressed so playback never leaves keys or buttons stuck.
type held struct {
	keys map[uint32]bool
	btns map[uint16]bool
}

func (h *held) dispatch(e event, desktop bool) {
	switch e.Kind {
	case kindKey:
		up, ext := e.B&1 != 0, e.B&2 != 0
		sendKey(uint16(e.A), up, ext)
		id := uint32(e.A)
		if ext {
			id |= 1 << 16
		}
		if up {
			delete(h.keys, id)
		} else {
			h.keys[id] = true
		}
	case kindStart:
		if desktop {
			storeExpPos(e.X, e.Y)
		}
		setCursorPos(e.X, e.Y) // begin every run from the recorded mouse position
	case kindMove:
		if desktop {
			storeExpPos(e.X, e.Y)
			setCursorPos(e.X, e.Y)
		} else if e.A != 0 || e.B != 0 {
			sendMouse(e.A, e.B, 0, mouseeventfMove)
		}
	case kindBtn:
		if desktop {
			storeExpPos(e.X, e.Y)
			setCursorPos(e.X, e.Y)
		}
		for _, a := range mouseActions(uint16(e.A), int16(e.B)) {
			sendMouse(0, 0, a.Data, a.Flag)
			if a.DownBit != 0 {
				if a.Down {
					h.btns[a.DownBit] = true
				} else {
					delete(h.btns, a.DownBit)
				}
			}
		}
	}
}

func (h *held) releaseAll() {
	for id := range h.keys {
		sendKey(uint16(id&0xFFFF), true, id>>16 != 0)
	}
	for bit := range h.btns {
		if a, ok := releaseAction(bit); ok {
			sendMouse(0, 0, a.Data, a.Flag)
		}
	}
}

// sleepUntil waits for t, spinning for the last couple of ms for accuracy.
// It returns false if playback was cancelled.
func sleepUntil(t time.Time) bool {
	for {
		if eng.stop.Load() != 0 {
			return false
		}
		d := time.Until(t)
		if d <= 0 {
			return true
		}
		if d > 3*time.Millisecond {
			if d > 50*time.Millisecond {
				d = 50 * time.Millisecond
			}
			time.Sleep(d - 2*time.Millisecond)
		} else {
			runtime.Gosched()
		}
	}
}

func startPlayback(p playParams) {
	if eng.state.Load() != idle {
		return
	}
	if realCount(eng.events) == 0 {
		setStatus("Nothing to play yet. Record a macro first", true)
		return
	}
	eng.stop.Store(0)
	eng.stopMsg.Store(nil)
	eng.bumpMode.Store(int32(p.bump))
	eng.desktop.Store(p.desktop)
	if x, y := cursorPos(); p.desktop {
		storeExpPos(x, y)
	}
	eng.bumpAcc = 0
	eng.playPos.Store(0)
	eng.playLoop.Store(1)
	eng.playLoops.Store(int32(p.loops))
	eng.phaseRunAt.Store(0)
	eng.countdownEnd.Store(time.Now().Add(p.delay).UnixNano())
	eng.state.Store(playing)
	setStatus("Playing. Press F9 to stop", false)
	go play(eng.events, p)
}

func requestStop() { eng.stop.Store(1) }

func play(evs []event, p playParams) {
	h := &held{keys: map[uint32]bool{}, btns: map[uint16]bool{}}
	defer func() {
		h.releaseAll() // never leave a key or button stuck down
		eng.state.Store(idle)
	}()
	if p.delay > 0 && !sleepUntil(time.Now().Add(p.delay)) {
		setStatus("Playback cancelled", false)
		return
	}
	eng.phaseRunAt.Store(time.Now().UnixNano())
	for loop := 1; p.loops == 0 || loop <= p.loops; loop++ {
		eng.playLoop.Store(int32(loop))
		start := time.Now()
		for i, e := range evs {
			eng.playPos.Store(int32(i))
			at := start.Add(time.Duration(float64(e.T) / p.speed * 1000))
			if !sleepUntil(at) {
				reportStopped()
				return
			}
			h.dispatch(e, p.desktop)
		}
	}
	setStatus("Playback finished", false)
}

// shutdown stops playback and waits briefly so held keys get released.
func shutdown() {
	requestStop()
	for i := 0; i < 200 && eng.state.Load() == playing; i++ {
		time.Sleep(5 * time.Millisecond)
	}
}
