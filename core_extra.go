package main

// More platform-independent helpers: macro validation, the activity profile
// shown in the GUI, and value formatting/stepping for the settings controls.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const maxEvents = 5_000_000
const maxDurationUS = int64(24 * 3600 * 1_000_000)

// validateEvents rejects macro files with values no real recording produces,
// so a tampered or corrupt file cannot do anything unexpected on replay.
func validateEvents(evs []event) error {
	if len(evs) == 0 {
		return fmt.Errorf("the file has no events")
	}
	if len(evs) > maxEvents {
		return fmt.Errorf("too many events (%d)", len(evs))
	}
	var prev int64
	for i, e := range evs {
		if e.T < prev || e.T > maxDurationUS {
			return fmt.Errorf("event %d has an invalid time", i+1)
		}
		prev = e.T
		switch e.Kind {
		case kindKey:
			if e.A < 1 || e.A > 0xFF || e.B < 0 || e.B > 3 {
				return fmt.Errorf("event %d has an invalid key", i+1)
			}
		case kindMove:
			if e.A < -50000 || e.A > 50000 || e.B < -50000 || e.B > 50000 {
				return fmt.Errorf("event %d has an invalid movement", i+1)
			}
		case kindBtn:
			if e.A < 1 || e.A > 0x0FFF || e.B < -32768 || e.B > 32767 {
				return fmt.Errorf("event %d has an invalid button", i+1)
			}
		case kindStart:
			if e.X < -100000 || e.X > 100000 || e.Y < -100000 || e.Y > 100000 {
				return fmt.Errorf("event %d has an invalid start position", i+1)
			}
		default:
			return fmt.Errorf("event %d has an unknown type", i+1)
		}
	}
	return nil
}

// activityProfile returns n values in 0..1 describing how busy the macro is
// across its duration. Keys and clicks weigh more than raw mouse movement.
func activityProfile(evs []event, n int) []float64 {
	out := make([]float64, n)
	if len(evs) == 0 || n == 0 {
		return out
	}
	total := evs[len(evs)-1].T
	for _, e := range evs {
		if e.Kind == kindStart {
			continue
		}
		idx := 0
		if total > 0 {
			idx = int(float64(e.T) / float64(total) * float64(n))
		}
		if idx >= n {
			idx = n - 1
		}
		if e.Kind == kindMove {
			out[idx]++
		} else {
			out[idx] += 4
		}
	}
	mx := 0.0
	for _, v := range out {
		if v > mx {
			mx = v
		}
	}
	if mx == 0 {
		return out
	}
	for i, v := range out {
		out[i] = math.Sqrt(v / mx)
	}
	return out
}

// packPos / unpackPos squeeze a screen position into one atomic value.
func packPos(x, y int32) uint64 { return uint64(uint32(x))<<32 | uint64(uint32(y)) }
func unpackPos(v uint64) (int32, int32) {
	return int32(uint32(v >> 32)), int32(uint32(v))
}

// realCount counts the recorded inputs, not the start-position marker.
func realCount(evs []event) int {
	n := 0
	for _, e := range evs {
		if e.Kind != kindStart {
			n++
		}
	}
	return n
}

// formatTime renders microseconds as m:ss.d
func formatTime(us int64) string {
	if us < 0 {
		us = 0
	}
	s := us / 1_000_000
	return fmt.Sprintf("%d:%02d.%d", s/60, s%60, (us/100_000)%10)
}

// commas formats 1234567 as 1,234,567
func commas(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return s
	}
	var b strings.Builder
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	return b.String()
}

var speedSteps = []float64{0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4}

func speedLabel(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64) + "×"
}

// stepSpeed moves to the next/previous preset speed (dir is +1 or -1).
func stepSpeed(cur float64, dir int) float64 {
	best := 0
	for i, s := range speedSteps {
		if math.Abs(s-cur) < math.Abs(speedSteps[best]-cur) {
			best = i
		}
	}
	best += dir
	if best < 0 {
		best = 0
	}
	if best >= len(speedSteps) {
		best = len(speedSteps) - 1
	}
	return speedSteps[best]
}

// stepLoops cycles 1..99 and "forever" (0).
func stepLoops(cur, dir int) int {
	if dir > 0 {
		switch {
		case cur == 0:
			return 1
		case cur < 99:
			return cur + 1
		}
		return 0
	}
	switch {
	case cur == 1:
		return 0
	case cur == 0:
		return 99
	}
	return cur - 1
}
