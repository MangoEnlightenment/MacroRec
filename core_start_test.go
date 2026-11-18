package main

import (
	"bytes"
	"reflect"
	"testing"
)

func TestStartPosition(t *testing.T) {
	in := []event{
		{T: 0, Kind: kindStart, X: 1280, Y: -40},
		{T: 8000, Kind: kindKey, A: 0x11},
		{T: 9000, Kind: kindKey, A: 0x11, B: 1},
	}

	// timing is measured from the first real input; the marker stays at 0
	normalize(in)
	if in[0].T != 0 || in[1].T != 0 || in[2].T != 1000 {
		t.Fatalf("normalize: %+v", in)
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
		t.Fatalf("round trip:\n in: %+v\nout: %+v", in, out)
	}
	if err := validateEvents(out); err != nil {
		t.Fatal(err)
	}
	if realCount(out) != 2 {
		t.Fatalf("realCount = %d", realCount(out))
	}
	if p := activityProfile(out, 4); len(p) != 4 {
		t.Fatalf("profile: %v", p)
	}

	// a start marker far off screen is rejected
	if validateEvents([]event{{Kind: kindStart, X: 9_000_000}}) == nil {
		t.Fatal("expected an error for an absurd start position")
	}
}
