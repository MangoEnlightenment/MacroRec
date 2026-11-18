//go:build windows

package main

// Everything that gets painted. Coordinates are in logical units (400 x 640)
// and the canvas scales them to the display DPI.

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

var (
	colBg     = rgb(14, 16, 20)
	colCard   = rgb(22, 25, 32)
	colCard2  = rgb(30, 34, 44)
	colText   = rgb(233, 236, 243)
	colDim    = rgb(139, 147, 166)
	colAccent = rgb(109, 141, 255)
	colRec    = rgb(255, 84, 104)
	colPlay   = rgb(48, 213, 160)
	colWhite  = rgb(255, 255, 255)
	colBlack  = rgb(0, 0, 0)
	colOnPlay = rgb(6, 19, 14)
)

const (
	iconDot = iota
	iconTri
	iconSquare
)

func drawUI(c *canvas) {
	c.begin(colBg)
	st := eng.state.Load()

	c.text("MacroRec", 20, 14, 230, 34, fTitle, alignL, colText)
	c.text("Keyboard · mouse · camera", 20, 46, 230, 18, fSmall, alignL, colDim)
	drawPill(c, st)
	drawHero(c, st)
	drawMainButtons(c, st)
	drawSettings(c)
	footerButton(c, wOpen, "Open…", false)
	footerButton(c, wSave, "Save as…", false)
	if ui.pin {
		footerButton(c, wPin, "Pinned on top", true)
	} else {
		footerButton(c, wPin, "Pin on top", false)
	}
	drawStatus(c)
}

func drawPill(c *canvas, st int32) {
	x, y, w, h := 262.0, 22.0, 118.0, 30.0
	fill, dot, label := colCard2, colDim, "Ready"
	switch st {
	case recording:
		fill, dot, label = mix(colCard2, colRec, 0.2), colRec, "Recording"
	case playing:
		fill, dot, label = mix(colCard2, colPlay, 0.2), colPlay, "Playing"
	}
	c.rrect(x, y, w, h, h/2, fill)
	if st != idle {
		s := 0.5 + 0.5*math.Sin(ui.phase*5)
		c.circle(x+16, y+h/2, 5+4*s, withAlpha(dot, uint8(30+60*(1-s))))
	}
	c.circle(x+16, y+h/2, 4.5, dot)
	c.text(label, x+30, y, w-36, h, fBodyB, alignL, colText)
}

func drawHero(c *canvas, st int32) {
	c.rrect(20, 84, 360, 152, 18, colCard)

	var timer, caption string
	big := colText
	n := realCount(eng.events)
	switch st {
	case recording:
		timer = formatTime(time.Since(eng.recStart).Microseconds())
		caption = fmt.Sprintf("Recording · %s events", commas(int(eng.count.Load())))
		big = mix(colText, colRec, 0.45)
	case playing:
		big = mix(colText, colPlay, 0.35)
		if eng.phaseRunAt.Load() == 0 {
			left := time.Until(time.Unix(0, eng.countdownEnd.Load())).Seconds()
			secs := int(math.Ceil(left))
			if secs < 1 {
				secs = 1
			}
			timer = strconv.Itoa(secs)
			caption = "Get ready · switch to your window"
		} else {
			timer = formatTime(playPosUS())
			loop, loops := eng.playLoop.Load(), eng.playLoops.Load()
			if loops == 0 {
				caption = fmt.Sprintf("Playing · loop %d", loop)
			} else {
				caption = fmt.Sprintf("Playing · loop %d of %d", loop, loops)
			}
		}
	default:
		timer = formatTime(eng.totalUS.Load())
		if n == 0 {
			caption = "No macro yet · press F8 to record"
		} else {
			caption = fmt.Sprintf("Ready · %s events", commas(n))
		}
	}
	c.text(timer, 20, 98, 360, 58, fBig, alignC, big)
	c.text(caption, 20, 156, 360, 22, fBody, alignC, colDim)
	drawBars(c, st)
}

// playPosUS is the macro time of the event being played right now.
func playPosUS() int64 {
	evs := eng.events // not modified while playing
	i := int(eng.playPos.Load())
	if i >= 0 && i < len(evs) {
		return evs[i].T
	}
	return 0
}

func drawBars(c *canvas, st int32) {
	const bw, gap = 3.6, 2.4
	x0 := 20 + (360-(nBars*bw+(nBars-1)*gap))/2
	mid := 206.0

	frac := -1.0
	if st == playing && eng.phaseRunAt.Load() != 0 {
		if total := eng.totalUS.Load(); total > 0 {
			frac = float64(playPosUS()) / float64(total)
		}
	}
	liveMax := 6.0
	for _, v := range ui.live {
		liveMax = math.Max(liveMax, v)
	}

	idle := mix(colCard, colDim, 0.35)
	for i := 0; i < nBars; i++ {
		var v float64
		col := idle
		if st == recording {
			v = math.Sqrt(ui.live[i] / liveMax)
			age := float64(i) / float64(nBars-1) // old on the left, newest on the right
			col = mix(mix(colCard, colRec, 0.25), colRec, age)
		} else {
			v = ui.profile[i]
			if frac >= 0 && float64(i)/nBars <= frac {
				col = mix(colAccent, colPlay, float64(i)/nBars)
			}
		}
		h := 4 + v*34
		c.rrect(x0+float64(i)*(bw+gap), mid-h/2, bw, h, bw/2, col)
	}
}

func drawMainButtons(c *canvas, st int32) {
	hasMacro := realCount(eng.events) > 0

	// record button
	switch {
	case st == recording:
		bigButton(c, wRec, colRec, mix(colRec, colWhite, 0.12), colWhite, colWhite, "Stop", "F8", iconSquare, true)
	case st == playing:
		bigButton(c, wRec, colCard, colCard, colDim, colDim, "Record", "F8", iconDot, false)
	default:
		bigButton(c, wRec, colCard2, mix(colCard2, colRec, 0.25), colRec, colText, "Record", "F8", iconDot, true)
	}

	// play button
	switch {
	case st == playing:
		bigButton(c, wPlay, colPlay, mix(colPlay, colWhite, 0.12), colOnPlay, colOnPlay, "Stop", "F9", iconSquare, true)
	case st == recording || !hasMacro:
		bigButton(c, wPlay, colCard, colCard, colDim, colDim, "Play", "F9", iconTri, false)
	default:
		bigButton(c, wPlay, colCard2, mix(colCard2, colPlay, 0.25), colPlay, colText, "Play", "F9", iconTri, true)
	}
}

func bigButton(c *canvas, id int, fill, hoverFill, ic, fg uint32, label, key string, icon int, enabled bool) {
	w := widgetByID(id)
	f := fill
	if enabled {
		f = mix(fill, hoverFill, ui.anim[id])
		if ui.down == id && ui.hover == id {
			f = mix(f, colBlack, 0.18)
		}
	}
	c.rrect(w.x, w.y, w.w, w.h, 16, f)

	ix, iy := w.x+30, w.y+w.h/2
	switch icon {
	case iconDot:
		c.circle(ix, iy, 7, ic)
	case iconTri:
		c.poly([][2]float64{{ix - 5, iy - 8}, {ix - 5, iy + 8}, {ix + 8, iy}}, ic)
	case iconSquare:
		c.rrect(ix-7, iy-7, 14, 14, 3, ic)
	}
	c.text(label, w.x+50, w.y, 80, w.h, fH2, alignL, fg)

	chip := mix(f, colBlack, 0.28)
	cx, cy := w.x+w.w-46, w.y+w.h/2-11
	c.rrect(cx, cy, 34, 22, 7, chip)
	c.text(key, cx, cy, 34, 22, fSmall, alignC, mix(fg, f, 0.35))
}

func drawSettings(c *canvas) {
	c.text("SETTINGS", 24, 322, 200, 18, fSmall, alignL, colDim)
	c.rrect(20, 344, 360, 248, 18, colCard)

	for i, l := range []string{"Mode", "Speed", "Repeat", "Start delay", "On mouse bump"} {
		c.text(l, 38, 358+float64(i)*46, 160, 32, fBody, alignL, colText)
	}

	// segmented Game | Desktop switch with a sliding thumb
	g, d := widgetByID(wGame), widgetByID(wDesktop)
	c.rrect(g.x, g.y, g.w+d.w, g.h, 16, colCard2)
	c.rrect(g.x+2+ui.seg*(d.x-g.x), g.y+2, g.w-4, g.h-4, 14, colAccent)
	gc := mix(colDim, colText, ui.anim[wGame]*0.7)
	dc := mix(colDim, colText, ui.anim[wDesktop]*0.7)
	c.text("Game", g.x, g.y, g.w, g.h, fBodyB, alignC, mix(colWhite, gc, ui.seg))
	c.text("Desktop", d.x, d.y, d.w, d.h, fBodyB, alignC, mix(dc, colWhite, ui.seg))

	loops := "∞"
	if ui.loops > 0 {
		loops = strconv.Itoa(ui.loops)
	}
	delay := "Off"
	if ui.delaySec > 0 {
		delay = strconv.Itoa(ui.delaySec) + " s"
	}
	drawStepper(c, wSpdM, wSpdP, speedLabel(ui.speed))
	drawStepper(c, wLoopM, wLoopP, loops)
	drawStepper(c, wDelM, wDelP, delay)
	drawBumpControl(c)
}

func drawStepper(c *canvas, minus, plus int, value string) {
	m, p := widgetByID(minus), widgetByID(plus)
	for _, w := range []widget{m, p} {
		col := mix(colCard2, colAccent, ui.anim[w.id]*0.55)
		if ui.down == w.id && ui.hover == w.id {
			col = mix(col, colBlack, 0.2)
		}
		c.rrect(w.x, w.y, w.w, w.h, 10, col)
	}
	c.text("−", m.x, m.y, m.w, m.h, fH2, alignC, colText)
	c.text("+", p.x, p.y, p.w, p.h, fH2, alignC, colText)
	c.text(value, m.x+m.w, m.y, p.x-m.x-m.w, m.h, fBodyB, alignC, colText)
}

// drawBumpControl is the Off | Stop | Fix selector with a sliding thumb, plus
// a one-line explanation of the selected option.
func drawBumpControl(c *canvas) {
	off, stop, fix := widgetByID(wBumpOff), widgetByID(wBumpStop), widgetByID(wBumpFix)
	c.rrect(off.x, off.y, off.w+stop.w+fix.w, off.h, 16, colCard2)
	c.rrect(off.x+2+ui.bumpPos*off.w, off.y+2, off.w-4, off.h-4, 14, colAccent)
	for i, w := range []widget{off, stop, fix} {
		sel := math.Max(0, 1-math.Abs(ui.bumpPos-float64(i))) // 1 while the thumb is on it
		base := mix(colDim, colText, ui.anim[w.id]*0.7)
		c.text([]string{"Off", "Stop", "Fix"}[i], w.x, w.y, w.w, w.h, fBodyB, alignC, mix(base, colWhite, sel))
	}
	hint := "Ignores your mouse during playback"
	switch ui.bumpMode {
	case 1:
		hint = "Stops the macro if you move or click the mouse"
	case 2:
		if ui.desktop {
			hint = "Moves the mouse back and keeps going (clicks stop it)"
		} else {
			hint = "Cancels your movement and keeps going (clicks stop it)"
		}
	}
	c.text(hint, 38, 575, 342, 16, fSmall, alignL, colDim)
}

func footerButton(c *canvas, id int, label string, active bool) {
	w := widgetByID(id)
	h := ui.anim[id]
	col := mix(colCard, colCard2, h)
	if active {
		col = mix(mix(colCard, colAccent, 0.35), colAccent, h*0.3)
	}
	if ui.down == id && ui.hover == id {
		col = mix(col, colBlack, 0.2)
	}
	c.rrect(w.x, w.y, w.w, w.h, 12, col)
	c.text(label, w.x, w.y, w.w, w.h, fBody, alignC, colText)
}

func drawStatus(c *canvas) {
	text := "F8 record  ·  F9 play  ·  works while your game is focused"
	col := mix(colBg, colDim, 0.7)
	if s := status.Load(); s != nil {
		if age := time.Since(s.at).Seconds(); age < 6 {
			text = s.text
			base := colDim
			if s.isErr {
				base = colRec
			}
			a := 1.0
			if age > 4.5 {
				a = 1 - (age-4.5)/1.5
			}
			col = mix(colBg, base, a)
		}
	}
	c.text(text, 20, 654, 360, 22, fSmall, alignC, col)
}
