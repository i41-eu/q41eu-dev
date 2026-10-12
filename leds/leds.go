// Package leds makes colours for the robot's 12 LEDs (the "leds" command's "pixels"): two
// strips on top of the head, as you face the screen the left strip 0-5 from the screen end to the
// back, and the right strip 6-11 from the back to the screen end (one chain, snaking).
//
// Correct turns a screen colour into what looks like it on the LEDs: they are linear (screen
// colours are gamma encoded), their green is much brighter than red and blue, and at full power
// the diffuser washes colours out (tuned on the pet, by eye).
package leds

import (
	"fmt"
	"image/color"
	"math"
	"strings"
)

// N is the number of LEDs.
const N = 12

// Frame is a colour per LED, in the robot's order.
type Frame [N]color.RGBA

// Names are colours by name (screen colours; Correct them for the LEDs).
var Names = map[string]color.RGBA{
	"off": {}, "black": {},
	"white": {255, 255, 255, 255}, "red": {255, 40, 40, 255}, "orange": {255, 140, 0, 255},
	"yellow": {255, 220, 0, 255}, "green": {40, 220, 60, 255}, "cyan": {0, 220, 220, 255},
	"blue": {40, 90, 255, 255}, "purple": {150, 60, 255, 255}, "pink": {255, 80, 180, 255},
}

// byEye are LED colours set by looking at the robot where Correct was off (the pet's).
var byEye = map[color.RGBA]color.RGBA{
	Names["yellow"]: {0xb3, 0x8f, 0x00, 255},
	Names["purple"]: {0x50, 0x00, 0xb3, 255},
	Names["white"]:  {0xb3, 0xb3, 0xb3, 255},
}

const (
	gamma = 2.2
	green = 0.65 // the green channel's share
	level = 0.7  // the strongest channel, of full power
)

// Correct is the LED colour that looks like the screen colour c. Black stays off.
func Correct(c color.RGBA) color.RGBA {
	if c.R == 0 && c.G == 0 && c.B == 0 {
		return color.RGBA{}
	}
	if e, ok := byEye[color.RGBA{c.R, c.G, c.B, 255}]; ok {
		return e
	}
	lin := func(v uint8) float64 { return math.Pow(float64(v)/255, gamma) }
	r, g, b := lin(c.R), lin(c.G)*green, lin(c.B)
	k := 255 * level / max(r, g, b, 1e-6)
	ch := func(v float64) uint8 { return uint8(math.Round(v * k)) }
	return color.RGBA{ch(r), ch(g), ch(b), 255}
}

// Dim scales a colour (0..1): a room light, not a lamp.
func Dim(c color.RGBA, f float64) color.RGBA {
	f = min(1, max(0, f))
	return color.RGBA{uint8(float64(c.R) * f), uint8(float64(c.G) * f), uint8(float64(c.B) * f), 255}
}

// Parse reads "#rrggbb", "rrggbb" or a name from Names.
func Parse(s string) (color.RGBA, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if c, ok := Names[s]; ok {
		return c, nil
	}
	var r, g, b uint8
	if _, err := fmt.Sscanf(strings.TrimPrefix(s, "#"), "%02x%02x%02x", &r, &g, &b); err != nil || len(strings.TrimPrefix(s, "#")) != 6 {
		return color.RGBA{}, fmt.Errorf("colour %q: #rrggbb or a name (%s)", s, strings.Join(names(), ", "))
	}
	return color.RGBA{r, g, b, 255}, nil
}

func names() []string {
	out := []string{}
	for n := range Names {
		out = append(out, n)
	}
	return out
}

// Hex is "#rrggbb".
func Hex(c color.RGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// All is every LED in c.
func All(c color.RGBA) Frame {
	var f Frame
	for i := range f {
		f[i] = c
	}
	return f
}

// Bar lights both strips from the screen end back, as far as fraction (0..1) of their length:
// a progress bar or a level.
func Bar(fraction float64, c color.RGBA) Frame {
	var f Frame
	n := int(math.Ceil(6 * min(1, max(0, fraction))))
	for i := range n {
		f[i] = c    // left, from the screen end
		f[11-i] = c // right, from the screen end
	}
	return f
}

// Left and Right are the strips' LEDs from the screen end back (for drawing them on a page).
func (f Frame) Left() [6]color.RGBA  { return [6]color.RGBA{f[0], f[1], f[2], f[3], f[4], f[5]} }
func (f Frame) Right() [6]color.RGBA { return [6]color.RGBA{f[11], f[10], f[9], f[8], f[7], f[6]} }

// Args is the "leds" command's arguments; correct: Correct every colour first.
func (f Frame) Args(correct bool) map[string]any {
	px := make([]any, N)
	for i, c := range f {
		if correct {
			c = Correct(c)
		}
		px[i] = Hex(c)
	}
	return map[string]any{"pixels": px}
}
