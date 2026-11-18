package main

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	in := []event{
		{T: 0, Kind: kindKey, A: 0x11, B: 0},
		{T: 1500, Kind: kindMove, A: -7, B: 12, X: 960, Y: 540},
		{T: 2500, Kind: kindBtn, A: 0x0001, B: 0, X: 960, Y: 540},
		{T: 3000, Kind: kindBtn, A: 0x0400, B: -120, X: 960, Y: 540},
		{T: 9000, Kind: kindKey, A: 0x11, B: 1},
	}
	var buf bytes.Buffer
	if err := writeEvents(&buf, in); err != nil {
		t.Fatal(err)
	}
	out, err := readEvents(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip mismatch:\n in: %+v\nout: %+v", in, out)
	}
}

func TestReadRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"0 Z 1 2", "0 K 1", "x K 1 2", "0 M 1 2 3"} {
		if _, err := readEvents(bytes.NewBufferString(bad)); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestNormalize(t *testing.T) {
	evs := []event{{T: 5000}, {T: 5200}, {T: 9000}}
	normalize(evs)
	if evs[0].T != 0 || evs[1].T != 200 || evs[2].T != 4000 {
		t.Fatalf("got %+v", evs)
	}
}

func rawMouseBuf(hdr int, flags, bf uint16, bd int16, dx, dy int32) []byte {
	b := make([]byte, hdr+24)
	le := binary.LittleEndian
	le.PutUint32(b[0:], 0)
	le.PutUint32(b[4:], uint32(len(b)))
	d := b[hdr:]
	le.PutUint16(d[0:], flags)
	le.PutUint16(d[4:], bf)
	le.PutUint16(d[6:], uint16(bd))
	le.PutUint32(d[12:], uint32(dx))
	le.PutUint32(d[16:], uint32(dy))
	return b
}

func rawKeyBuf(hdr int, scan, flags, vk uint16) []byte {
	b := make([]byte, hdr+16)
	le := binary.LittleEndian
	le.PutUint32(b[0:], 1)
	le.PutUint32(b[4:], uint32(len(b)))
	d := b[hdr:]
	le.PutUint16(d[0:], scan)
	le.PutUint16(d[2:], flags)
	le.PutUint16(d[6:], vk)
	return b
}

func TestParseMouse(t *testing.T) {
	for _, hdr := range []int{24, 16} { // 64-bit and 32-bit header sizes
		r, ok := parseRaw(rawMouseBuf(hdr, 0, 0x0401, -120, -5, 7), hdr)
		if !ok || r.Type != rawMouse || r.DX != -5 || r.DY != 7 || r.BtnFlags != 0x0401 || r.BtnData != -120 || r.Absolute {
			t.Fatalf("hdr=%d: unexpected %+v ok=%v", hdr, r, ok)
		}
	}
	r, _ := parseRaw(rawMouseBuf(24, 1, 0, 0, 30000, 20000), 24)
	if !r.Absolute || r.DX != 0 || r.DY != 0 {
		t.Fatalf("absolute device should not produce deltas: %+v", r)
	}
}

func TestParseKeyboard(t *testing.T) {
	r, ok := parseRaw(rawKeyBuf(24, 0x11, 0, 'W'), 24)
	if !ok || r.Type != rawKey || r.Scan != 0x11 || r.Up || r.Ext || r.Skip || r.VK != 'W' {
		t.Fatalf("W down: %+v", r)
	}
	r, _ = parseRaw(rawKeyBuf(24, 0x4B, 0x03, 0x25), 24) // left arrow release (E0 + break)
	if !r.Up || !r.Ext || r.Skip {
		t.Fatalf("left arrow up: %+v", r)
	}
	r, _ = parseRaw(rawKeyBuf(24, 0x2A, 0x02, 0xFF), 24) // fake shift
	if !r.Skip {
		t.Fatalf("fake shift should be skipped: %+v", r)
	}
	r, _ = parseRaw(rawKeyBuf(24, 0x1D, 0x04, 0x13), 24) // E1 (Pause)
	if !r.Skip {
		t.Fatalf("E1 should be skipped: %+v", r)
	}
	if _, ok := parseRaw(make([]byte, 10), 24); ok {
		t.Fatal("short buffer must be rejected")
	}
}

func TestMouseActions(t *testing.T) {
	acts := mouseActions(0x0001|0x0008, 0) // left down + right up
	if len(acts) != 2 || acts[0].Flag != 0x0002 || !acts[0].Down || acts[1].Flag != 0x0010 || acts[1].Down {
		t.Fatalf("%+v", acts)
	}
	wheel := mouseActions(0x0400, -120)
	if len(wheel) != 1 || wheel[0].Flag != 0x0800 || wheel[0].Data != 0xFFFFFF88 {
		t.Fatalf("wheel: %+v (data %#x)", wheel, wheel[0].Data)
	}
	x2 := mouseActions(0x0100, 0)
	if len(x2) != 1 || x2[0].Flag != 0x0080 || x2[0].Data != 2 {
		t.Fatalf("x2: %+v", x2)
	}
	up, ok := releaseAction(0x0001)
	if !ok || up.Flag != 0x0004 {
		t.Fatalf("release left: %+v", up)
	}
}

func TestParseFKey(t *testing.T) {
	if vk, err := parseFKey("f8"); err != nil || vk != 0x77 {
		t.Fatalf("F8: %#x %v", vk, err)
	}
	if vk, err := parseFKey("F12"); err != nil || vk != 0x7B {
		t.Fatalf("F12: %#x %v", vk, err)
	}
	for _, bad := range []string{"F0", "F13", "A", "", "F"} {
		if _, err := parseFKey(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
