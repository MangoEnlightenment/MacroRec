package main

import "testing"

func TestValidateEvents(t *testing.T) {
	good := []event{
		{T: 0, Kind: kindKey, A: 0x11, B: 0},
		{T: 10, Kind: kindMove, A: -3, B: 4},
		{T: 20, Kind: kindBtn, A: 0x0001},
		{T: 30, Kind: kindBtn, A: 0x0400, B: -120},
	}
	if err := validateEvents(good); err != nil {
		t.Fatal(err)
	}
	bad := map[string][]event{
		"empty":          {},
		"time goes back": {{T: 5, Kind: kindKey, A: 1}, {T: 1, Kind: kindKey, A: 1}},
		"scan zero":      {{T: 0, Kind: kindKey, A: 0}},
		"scan huge":      {{T: 0, Kind: kindKey, A: 5000}},
		"key flags":      {{T: 0, Kind: kindKey, A: 1, B: 9}},
		"move huge":      {{T: 0, Kind: kindMove, A: 99999}},
		"button flags":   {{T: 0, Kind: kindBtn, A: 0x7000}},
		"far future":     {{T: maxDurationUS + 1, Kind: kindKey, A: 1}},
		"bad kind":       {{T: 0, Kind: 'Z'}},
	}
	for name, evs := range bad {
		if validateEvents(evs) == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestActivityProfile(t *testing.T) {
	evs := []event{
		{T: 0, Kind: kindKey, A: 1},
		{T: 500, Kind: kindMove, A: 1, B: 1},
		{T: 1000, Kind: kindBtn, A: 1},
	}
	p := activityProfile(evs, 4)
	if len(p) != 4 {
		t.Fatalf("len %d", len(p))
	}
	mx := 0.0
	for _, v := range p {
		if v < 0 || v > 1 {
			t.Fatalf("out of range: %v", p)
		}
		if v > mx {
			mx = v
		}
	}
	if mx != 1 {
		t.Fatalf("expected a normalized peak of 1, got %v", p)
	}
	if got := activityProfile(nil, 3); len(got) != 3 || got[0] != 0 {
		t.Fatalf("empty input: %v", got)
	}
}

func TestFormatting(t *testing.T) {
	if got := formatTime(75_300_000); got != "1:15.3" {
		t.Fatalf("formatTime: %s", got)
	}
	if got := formatTime(-5); got != "0:00.0" {
		t.Fatalf("formatTime neg: %s", got)
	}
	cases := map[int]string{0: "0", 999: "999", 1000: "1,000", 1284: "1,284", 1234567: "1,234,567"}
	for n, want := range cases {
		if got := commas(n); got != want {
			t.Errorf("commas(%d) = %s, want %s", n, got, want)
		}
	}
	if speedLabel(1) != "1×" || speedLabel(0.25) != "0.25×" {
		t.Fatalf("speedLabel: %s %s", speedLabel(1), speedLabel(0.25))
	}
}

func TestSteppers(t *testing.T) {
	if stepSpeed(1, 1) != 1.25 || stepSpeed(1, -1) != 0.75 {
		t.Fatal("speed step")
	}
	if stepSpeed(0.25, -1) != 0.25 || stepSpeed(4, 1) != 4 {
		t.Fatal("speed clamp")
	}
	if stepLoops(1, -1) != 0 || stepLoops(0, 1) != 1 || stepLoops(0, -1) != 99 || stepLoops(99, 1) != 0 || stepLoops(5, 1) != 6 || stepLoops(5, -1) != 4 {
		t.Fatal("loop step")
	}
}
