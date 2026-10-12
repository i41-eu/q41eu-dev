package leds

import (
	"image/color"
	"testing"
)

func TestCorrect(t *testing.T) {
	if Correct(color.RGBA{}) != (color.RGBA{}) {
		t.Fatal("black stays off")
	}
	g := Correct(color.RGBA{0, 255, 0, 255})
	r := Correct(color.RGBA{255, 0, 0, 255})
	if g.G != r.R || g.G != uint8(255*level+0.5) {
		t.Fatalf("every colour gets the same power: green %v, red %v", g, r)
	}
	if Correct(Names["yellow"]) != (color.RGBA{0xb3, 0x8f, 0x00, 255}) {
		t.Fatal("yellow as tuned by eye")
	}
}

func TestParse(t *testing.T) {
	for s, want := range map[string]color.RGBA{"#ff8000": {255, 128, 0, 255}, "00ff00": {0, 255, 0, 255}, "Red": Names["red"]} {
		got, err := Parse(s)
		if err != nil || got != want {
			t.Errorf("%s: %v %v", s, got, err)
		}
	}
	for _, bad := range []string{"#12", "nope", "#gggggg"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%s: want an error", bad)
		}
	}
}

func TestBar(t *testing.T) {
	c := color.RGBA{1, 2, 3, 255}
	f := Bar(0.5, c)
	// three on each strip, from the screen end: left 0-2, right 11-9
	for i, want := range []bool{true, true, true, false, false, false, false, false, false, true, true, true} {
		if (f[i] == c) != want {
			t.Fatalf("LED %d: %v", i, f[i])
		}
	}
	if f.Left()[0] != c || f.Right()[0] != c || f.Right()[5] == c {
		t.Fatal("strips from the screen end")
	}
	args := Bar(1, Names["red"]).Args(false)["pixels"].([]any)
	if len(args) != N || args[6] != "#ff2828" {
		t.Fatalf("args %v", args)
	}
}
