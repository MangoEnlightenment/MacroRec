package main

import "testing"

func TestPackPos(t *testing.T) {
	cases := [][2]int32{{0, 0}, {1920, 1080}, {-1280, 400}, {3000, -200}, {-5, -5}}
	for _, c := range cases {
		x, y := unpackPos(packPos(c[0], c[1]))
		if x != c[0] || y != c[1] {
			t.Errorf("packPos(%d,%d) came back as (%d,%d)", c[0], c[1], x, y)
		}
	}
}
