//go:build windows

package main

// Anti-aliased drawing on a double-buffered canvas using GDI+ (flat API).
// Only integer/pointer-argument GDI+ calls are used, because floating point
// arguments cannot be passed through the plain syscall interface.

import (
	"fmt"
	"math"
	"syscall"
	"unsafe"
)

var (
	gdiplus = sysDLL("gdiplus.dll")

	pGdiplusStartup               = gdiplus.NewProc("GdiplusStartup")
	pGdipCreateFromHDC            = gdiplus.NewProc("GdipCreateFromHDC")
	pGdipSetSmoothingMode         = gdiplus.NewProc("GdipSetSmoothingMode")
	pGdipSetTextRenderingHint     = gdiplus.NewProc("GdipSetTextRenderingHint")
	pGdipGraphicsClear            = gdiplus.NewProc("GdipGraphicsClear")
	pGdipCreateSolidFill          = gdiplus.NewProc("GdipCreateSolidFill")
	pGdipDeleteBrush              = gdiplus.NewProc("GdipDeleteBrush")
	pGdipFillRectangleI           = gdiplus.NewProc("GdipFillRectangleI")
	pGdipFillEllipseI             = gdiplus.NewProc("GdipFillEllipseI")
	pGdipFillPolygonI             = gdiplus.NewProc("GdipFillPolygonI")
	pGdipDrawString               = gdiplus.NewProc("GdipDrawString")
	pGdipCreateFontFromLogfontW   = gdiplus.NewProc("GdipCreateFontFromLogfontW")
	pGdipCreateStringFormat       = gdiplus.NewProc("GdipCreateStringFormat")
	pGdipSetStringFormatAlign     = gdiplus.NewProc("GdipSetStringFormatAlign")
	pGdipSetStringFormatLineAlign = gdiplus.NewProc("GdipSetStringFormatLineAlign")
)

type gdipStartupInput struct {
	version        uint32
	debugCallback  uintptr
	suppressBgThrd int32
	suppressCodecs int32
}

type gpPoint struct{ x, y int32 }
type rectF struct{ x, y, w, h float32 }

type logFont struct {
	height, width, escapement, orientation, weight int32
	italic, underline, strikeOut, charSet          byte
	outPrecision, clipPrecision, quality, pitchFam byte
	face                                           [32]uint16
}

// fonts
const (
	fTitle = iota
	fH2
	fBody
	fBodyB
	fSmall
	fBig
	fCount
)

// text alignment
const (
	alignL = iota
	alignC
	alignR
)

var gdipToken uintptr

func gdipStart() error {
	in := gdipStartupInput{version: 1}
	r, _, _ := pGdiplusStartup.Call(uintptr(unsafe.Pointer(&gdipToken)), uintptr(unsafe.Pointer(&in)), 0)
	if r != 0 {
		return fmt.Errorf("GDI+ failed to start (code %d)", r)
	}
	return nil
}

// ---- colors (0xAARRGGBB) ----

func rgb(r, g, b uint8) uint32 {
	return 0xFF000000 | uint32(r)<<16 | uint32(g)<<8 | uint32(b)
}

func withAlpha(col uint32, a uint8) uint32 { return (col & 0x00FFFFFF) | uint32(a)<<24 }

// mix blends a toward b by t (0..1), keeping the result opaque.
func mix(a, b uint32, t float64) uint32 {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	ch := func(shift uint) uint32 {
		x := float64((a >> shift) & 0xFF)
		y := float64((b >> shift) & 0xFF)
		return uint32(math.Round(x+(y-x)*t)) & 0xFF
	}
	return 0xFF000000 | ch(16)<<16 | ch(8)<<8 | ch(0)
}

// ---- canvas ----

type canvas struct {
	memDC, bmp uintptr
	g          uintptr
	w, h       int32
	scale      float64
	fonts      [fCount]uintptr
	fmts       [3]uintptr
	brushes    []uintptr
}

func newCanvas(refDC uintptr, w, h int32, scale float64) *canvas {
	c := &canvas{w: w, h: h, scale: scale}
	c.memDC, _, _ = pCreateCompatibleDC.Call(refDC)
	c.bmp, _, _ = pCreateCompatibleBitmap.Call(refDC, uintptr(w), uintptr(h))
	pSelectObject.Call(c.memDC, c.bmp)
	pGdipCreateFromHDC.Call(c.memDC, uintptr(unsafe.Pointer(&c.g)))
	pGdipSetSmoothingMode.Call(c.g, 4)     // anti-alias
	pGdipSetTextRenderingHint.Call(c.g, 4) // anti-alias text

	// size in logical px, weight
	spec := [fCount][2]int{
		fTitle: {24, 700}, fH2: {15, 600}, fBody: {13, 400},
		fBodyB: {13, 600}, fSmall: {11, 400}, fBig: {46, 300},
	}
	for i, s := range spec {
		c.fonts[i] = c.makeFont(s[0], s[1])
	}
	for i := 0; i < 3; i++ {
		// NoWrap (0x1000) | NoClip (0x4000)
		pGdipCreateStringFormat.Call(0x5000, 0, uintptr(unsafe.Pointer(&c.fmts[i])))
		pGdipSetStringFormatAlign.Call(c.fmts[i], uintptr(i))
		pGdipSetStringFormatLineAlign.Call(c.fmts[i], 1) // vertically centered
	}
	return c
}

func (c *canvas) makeFont(px, weight int) uintptr {
	lf := logFont{height: -int32(math.Round(float64(px) * c.scale)), weight: int32(weight), charSet: 1, quality: 5}
	face, _ := syscall.UTF16FromString("Segoe UI")
	copy(lf.face[:], face)
	var font uintptr
	pGdipCreateFontFromLogfontW.Call(c.memDC, uintptr(unsafe.Pointer(&lf)), uintptr(unsafe.Pointer(&font)))
	return font
}

// d converts logical units to device pixels.
func (c *canvas) d(v float64) int32 { return int32(math.Round(v * c.scale)) }

func (c *canvas) brush(col uint32) uintptr {
	var b uintptr
	pGdipCreateSolidFill.Call(uintptr(col), uintptr(unsafe.Pointer(&b)))
	c.brushes = append(c.brushes, b)
	return b
}

func (c *canvas) begin(bg uint32) { pGdipGraphicsClear.Call(c.g, uintptr(bg)) }

// end copies the finished frame to the window and frees per-frame brushes.
func (c *canvas) end(dst uintptr) {
	pBitBlt.Call(dst, 0, 0, uintptr(c.w), uintptr(c.h), c.memDC, 0, 0, 0x00CC0020) // SRCCOPY
	for _, b := range c.brushes {
		pGdipDeleteBrush.Call(b)
	}
	c.brushes = c.brushes[:0]
}

func (c *canvas) fillRect(x, y, w, h int32, b uintptr) {
	if w <= 0 || h <= 0 {
		return
	}
	pGdipFillRectangleI.Call(c.g, b, uintptr(x), uintptr(y), uintptr(w), uintptr(h))
}

func (c *canvas) fillEllipse(x, y, w, h int32, b uintptr) {
	if w <= 0 || h <= 0 {
		return
	}
	pGdipFillEllipseI.Call(c.g, b, uintptr(x), uintptr(y), uintptr(w), uintptr(h))
}

// rrect draws a filled rounded rectangle. Use opaque colors (shapes overlap).
func (c *canvas) rrect(x, y, w, h, r float64, col uint32) {
	X, Y, W, H, R := c.d(x), c.d(y), c.d(w), c.d(h), c.d(r)
	if R*2 > H {
		R = H / 2
	}
	if R*2 > W {
		R = W / 2
	}
	b := c.brush(col)
	if R <= 0 {
		c.fillRect(X, Y, W, H, b)
		return
	}
	c.fillRect(X+R, Y, W-2*R, H, b)
	c.fillRect(X, Y+R, R+1, H-2*R, b)
	c.fillRect(X+W-R-1, Y+R, R+1, H-2*R, b)
	c.fillEllipse(X, Y, 2*R, 2*R, b)
	c.fillEllipse(X+W-2*R, Y, 2*R, 2*R, b)
	c.fillEllipse(X, Y+H-2*R, 2*R, 2*R, b)
	c.fillEllipse(X+W-2*R, Y+H-2*R, 2*R, 2*R, b)
}

// circle draws a filled circle centered at (cx, cy). Alpha is allowed.
func (c *canvas) circle(cx, cy, r float64, col uint32) {
	b := c.brush(col)
	c.fillEllipse(c.d(cx-r), c.d(cy-r), c.d(2*r), c.d(2*r), b)
}

func (c *canvas) poly(pts [][2]float64, col uint32) {
	ps := make([]gpPoint, len(pts))
	for i, p := range pts {
		ps[i] = gpPoint{c.d(p[0]), c.d(p[1])}
	}
	b := c.brush(col)
	pGdipFillPolygonI.Call(c.g, b, uintptr(unsafe.Pointer(&ps[0])), uintptr(len(ps)), 0)
}

// text draws a single line vertically centered inside the given box.
func (c *canvas) text(s string, x, y, w, h float64, font, align int, col uint32) {
	if s == "" {
		return
	}
	r := rectF{float32(c.d(x)), float32(c.d(y)), float32(c.d(w)), float32(c.d(h))}
	b := c.brush(col)
	pGdipDrawString.Call(c.g, uintptr(unsafe.Pointer(u16(s))), ^uintptr(0),
		c.fonts[font], uintptr(unsafe.Pointer(&r)), c.fmts[align], b)
}
