// Package screen draws pictures for the robot's 320x240 screen (sent as JPEG, wire.BinShowJPEG):
// a card (a title, big text that fits itself, a note, a progress bar), a question with buttons
// to tap, and text in the Go fonts (golang.org/x/image/font/gofont, BSD) at any size.
package screen

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// W, H: the robot's screen.
const W, H = 320, 240

// Colors of the default look (dark, as the robot's own screens).
var (
	Background = color.RGBA{0x10, 0x14, 0x2a, 0xff}
	Panel      = color.RGBA{0x2a, 0x30, 0x50, 0xff}
	White      = color.RGBA{0xf2, 0xf4, 0xff, 0xff}
	Muted      = color.RGBA{0x8a, 0x93, 0xb8, 0xff}
	Blue       = color.RGBA{0x7d, 0x8c, 0xff, 0xff}
)

var (
	fontsOnce     sync.Once
	regular, bold *opentype.Font
	facesMu       sync.Mutex
	faces         = map[faceKey]font.Face{}
)

type faceKey struct {
	size float64
	bold bool
}

// Face is the Go font (regular or bold) at a size in pixels.
func Face(size float64, isBold bool) font.Face {
	fontsOnce.Do(func() {
		regular, _ = opentype.Parse(goregular.TTF)
		bold, _ = opentype.Parse(gobold.TTF)
	})
	facesMu.Lock()
	defer facesMu.Unlock()
	k := faceKey{size, isBold}
	if f, ok := faces[k]; ok {
		return f
	}
	src := regular
	if isBold {
		src = bold
	}
	f, err := opentype.NewFace(src, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err) // the embedded fonts parse
	}
	faces[k] = f
	return f
}

// Text draws s with its baseline at (x, y).
func Text(dst draw.Image, s string, x, y int, size float64, isBold bool, c color.Color) {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: Face(size, isBold), Dot: fixed.P(x, y)}
	d.DrawString(s)
}

// TextWidth is how wide s is drawn, in pixels.
func TextWidth(s string, size float64, isBold bool) int {
	return font.MeasureString(Face(size, isBold), s).Ceil()
}

// Centered draws s centred on x, baseline at y.
func Centered(dst draw.Image, s string, x, y int, size float64, isBold bool, c color.Color) {
	Text(dst, s, x-TextWidth(s, size, isBold)/2, y, size, isBold, c)
}

// Wrap breaks s into lines no wider than width (a word longer than that is cut). Newlines in s
// start new lines.
func Wrap(s string, width int, size float64, isBold bool) []string {
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, w := range strings.Fields(para) {
			for TextWidth(w, size, isBold) > width && len([]rune(w)) > 1 { // cut a long word
				r := []rune(w)
				n := len(r) - 1
				for n > 1 && TextWidth(string(r[:n]), size, isBold) > width {
					n--
				}
				if line != "" {
					lines = append(lines, line)
					line = ""
				}
				lines = append(lines, string(r[:n]))
				w = string(r[n:])
			}
			switch {
			case line == "":
				line = w
			case TextWidth(line+" "+w, size, isBold) <= width:
				line += " " + w
			default:
				lines = append(lines, line)
				line = w
			}
		}
		lines = append(lines, line)
	}
	return lines
}

// FitText picks the largest of sizes at which s, wrapped to width, fits height; it gives the
// size and the lines (the smallest size when nothing fits, its lines cut to the height).
func FitText(s string, width, height int, isBold bool, sizes ...float64) (float64, []string) {
	if len(sizes) == 0 {
		sizes = []float64{72, 60, 48, 40, 34, 28, 24, 20, 17, 14}
	}
	for _, size := range sizes {
		lines := Wrap(s, width, size, isBold)
		if float64(len(lines))*lineHeight(size) <= float64(height) {
			return size, lines
		}
	}
	size := sizes[len(sizes)-1]
	lines := Wrap(s, width, size, isBold)
	if n := int(float64(height) / lineHeight(size)); n < len(lines) {
		lines = lines[:max(1, n)]
		lines[len(lines)-1] += "…"
	}
	return size, lines
}

func lineHeight(size float64) float64 { return size * 1.18 }

// Fill paints the rectangle.
func Fill(dst draw.Image, r image.Rectangle, c color.Color) {
	draw.Draw(dst, r, image.NewUniform(c), image.Point{}, draw.Src)
}

// RoundRect paints a rectangle with rounded corners (radius rad).
func RoundRect(dst *image.RGBA, r image.Rectangle, rad int, c color.RGBA) {
	rad = min(rad, r.Dx()/2, r.Dy()/2)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			dx, dy := 0, 0
			if x < r.Min.X+rad {
				dx = r.Min.X + rad - x
			} else if x >= r.Max.X-rad {
				dx = x - (r.Max.X - rad - 1)
			}
			if y < r.Min.Y+rad {
				dy = r.Min.Y + rad - y
			} else if y >= r.Max.Y-rad {
				dy = y - (r.Max.Y - rad - 1)
			}
			if dx*dx+dy*dy <= rad*rad {
				dst.SetRGBA(x, y, c)
			}
		}
	}
}

// New is a blank screen in the background colour (Background when bg is zero).
func New(bg color.RGBA) *image.RGBA {
	if bg == (color.RGBA{}) {
		bg = Background
	}
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	Fill(img, img.Bounds(), bg)
	return img
}

// Card is a simple screen: a title at the top, big text in the middle that sizes itself to fit,
// a note at the bottom, and a progress bar.
type Card struct {
	Title      string     `json:"title,omitempty"`
	Text       string     `json:"text,omitempty"`
	Note       string     `json:"note,omitempty"`
	Color      color.RGBA `json:"-"`                  // the text, the title and the bar (zero: white text, a blue title and bar)
	Background color.RGBA `json:"-"`                  // Background when zero
	Progress   *float64   `json:"progress,omitempty"` // 0..1: a bar at the bottom; nil: none
}

// Draw renders the card.
func (c Card) Draw() *image.RGBA {
	img := New(c.Background)
	accent, textColor := c.Color, c.Color
	if accent == (color.RGBA{}) {
		accent, textColor = Blue, White
	}
	top, bottom := 10, H-10
	if c.Title != "" {
		Text(img, c.Title, 14, 34, 22, true, accent)
		top = 46
	}
	if c.Progress != nil {
		bar := image.Rect(14, bottom-14, W-14, bottom)
		RoundRect(img, bar, 7, Panel)
		fill := bar
		fill.Max.X = bar.Min.X + int(float64(bar.Dx())*min(1, max(0, *c.Progress)))
		if fill.Dx() > 0 {
			RoundRect(img, fill, 7, accent)
		}
		bottom = bar.Min.Y - 8
	}
	if c.Note != "" {
		Centered(img, c.Note, W/2, bottom-6, 18, false, Muted)
		bottom -= 30
	}
	if c.Text != "" {
		size, lines := FitText(c.Text, W-28, bottom-top, true)
		lh := lineHeight(size)
		y := float64(top) + (float64(bottom-top)-lh*float64(len(lines)))/2 + size*0.92
		for _, l := range lines {
			Centered(img, l, W/2, int(y), size, true, textColor)
			y += lh
		}
	}
	return img
}

// Button is a place to tap on a screen.
type Button struct {
	Label string
	Rect  image.Rectangle
}

// Hit is the index of the button at (x, y), or -1.
func Hit(buttons []Button, x, y int) int {
	for i, b := range buttons {
		if image.Pt(x, y).In(b.Rect) {
			return i
		}
	}
	return -1
}

// Ask draws a question with up to four answers as buttons (one row of two, or two rows), and
// gives the buttons for Hit.
func Ask(question string, answers []string, accent color.RGBA) (*image.RGBA, []Button) {
	if accent == (color.RGBA{}) {
		accent = Blue
	}
	answers = answers[:min(len(answers), 4)]
	img := New(color.RGBA{})
	rows := 1
	if len(answers) > 2 {
		rows = 2
	}
	const gap, bh = 8, 52
	areaTop := H - 14 - rows*bh - (rows-1)*gap
	size, lines := FitText(question, W-28, areaTop-24, true, 40, 34, 28, 24, 20, 17)
	lh := lineHeight(size)
	y := 12 + (float64(areaTop-24)-lh*float64(len(lines)))/2 + size*0.92
	for _, l := range lines {
		Centered(img, l, W/2, int(y), size, true, White)
		y += lh
	}
	var buttons []Button
	for i, a := range answers {
		row, col, cols := i/2, i%2, min(2, len(answers)-i/2*2)
		w := (W - 28 - gap*(cols-1)) / cols
		r := image.Rect(14+col*(w+gap), areaTop+row*(bh+gap), 14+col*(w+gap)+w, areaTop+row*(bh+gap)+bh)
		c := Panel
		if i == 0 {
			c = accent
		}
		RoundRect(img, r, 12, c)
		bs, bl := FitText(a, r.Dx()-16, bh-8, true, 24, 20, 17, 14)
		Centered(img, bl[0], r.Min.X+r.Dx()/2, r.Min.Y+bh/2+int(bs*0.36), bs, true, White)
		buttons = append(buttons, Button{Label: a, Rect: r})
	}
	return img, buttons
}

// Fit scales a picture to cover the screen and crops the middle.
func Fit(src image.Image) *image.RGBA {
	b := src.Bounds()
	if b.Dx() == W && b.Dy() == H {
		if rgba, ok := src.(*image.RGBA); ok {
			return rgba
		}
	}
	scale := max(float64(W)/float64(b.Dx()), float64(H)/float64(b.Dy()))
	w, h := int(float64(b.Dx())*scale+0.5), int(float64(b.Dy())*scale+0.5)
	out := image.NewRGBA(image.Rect(0, 0, W, H))
	dst := image.Rect((W-w)/2, (H-h)/2, (W-w)/2+w, (H-h)/2+h)
	xdraw.CatmullRom.Scale(out, dst, src, b, draw.Src, nil)
	return out
}

// JPEG is the picture fitted to the screen, as the robot's picture message wants it (baseline
// JPEG; send with wire.BinShowJPEG).
func JPEG(img image.Image) []byte {
	var buf bytes.Buffer
	jpeg.Encode(&buf, Fit(img), &jpeg.Options{Quality: 88})
	return buf.Bytes()
}
