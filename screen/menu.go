package screen

import (
	"image"
	"image/color"
	"image/draw"

	xdraw "golang.org/x/image/draw"
)

// MenuItem is a tile of a menu: its label, and a picture above it (nil: the label alone).
type MenuItem struct {
	Label string
	Image image.Image
}

// MaxMenuItems is how many tiles a menu has at most (one screen).
const MaxMenuItems = 6

// Menu draws a title and 2-6 tiles in a grid: 2 or 3 in a row, 4 as two rows of two, 5 or 6 as
// two rows of three (the last row centred). It gives the tiles as buttons for Hit, in the
// items' order. accent colours the title (Blue when zero).
func Menu(title string, items []MenuItem, accent color.RGBA) (*image.RGBA, []Button) {
	if accent == (color.RGBA{}) {
		accent = Blue
	}
	items = items[:min(len(items), MaxMenuItems)]
	img := New(color.RGBA{})
	top := 10
	if title != "" {
		size, lines := FitText(title, W-28, 30, true, 24, 20, 17, 14)
		Text(img, lines[0], 14, 10+int(size*0.92), size, true, accent)
		top = 46
	}
	cols, rows := len(items), 1
	switch {
	case len(items) == 4:
		cols, rows = 2, 2
	case len(items) > 4:
		cols, rows = 3, 2
	}
	const margin, gap = 12, 8
	w := (W - 2*margin - gap*(cols-1)) / max(1, cols)
	h := (H - margin - top - gap*(rows-1)) / rows
	var buttons []Button
	for i, it := range items {
		row, col := i/cols, i%cols
		inRow := min(cols, len(items)-row*cols)
		x0 := margin + (cols-inRow)*(w+gap)/2 // a shorter last row is centred
		r := image.Rect(x0+col*(w+gap), top+row*(h+gap), x0+col*(w+gap)+w, top+row*(h+gap)+h)
		RoundRect(img, r, 12, Panel)
		if it.Image != nil {
			labelH := 0
			if it.Label != "" {
				labelH = 26
			}
			pic := r.Inset(8)
			pic.Max.Y -= labelH
			contain(img, pic, it.Image)
			if labelH > 0 {
				size, lines := FitText(it.Label, r.Dx()-12, labelH, true, 20, 17, 14, 12)
				Centered(img, lines[0], r.Min.X+r.Dx()/2, r.Max.Y-8, size, true, White)
			}
		} else {
			size, lines := FitText(it.Label, r.Dx()-16, r.Dy()-12, true, 32, 28, 24, 20, 17, 14)
			lh := lineHeight(size)
			y := float64(r.Min.Y) + (float64(r.Dy())-lh*float64(len(lines)))/2 + size*0.92
			for _, l := range lines {
				Centered(img, l, r.Min.X+r.Dx()/2, int(y), size, true, White)
				y += lh
			}
		}
		buttons = append(buttons, Button{Label: it.Label, Rect: r})
	}
	return img, buttons
}

// contain draws src as large as fits in r, keeping its shape, centred (its transparency kept).
func contain(dst *image.RGBA, r image.Rectangle, src image.Image) {
	b := src.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 || r.Dx() <= 0 || r.Dy() <= 0 {
		return
	}
	scale := min(float64(r.Dx())/float64(b.Dx()), float64(r.Dy())/float64(b.Dy()))
	w, h := max(1, int(float64(b.Dx())*scale+0.5)), max(1, int(float64(b.Dy())*scale+0.5))
	at := image.Rect(r.Min.X+(r.Dx()-w)/2, r.Min.Y+(r.Dy()-h)/2, r.Min.X+(r.Dx()-w)/2+w, r.Min.Y+(r.Dy()-h)/2+h)
	xdraw.CatmullRom.Scale(dst, at, src, b, draw.Over, nil)
}

// Region is a place to tap on the screen, in its pixels: a rectangle (Rect), a circle (Center
// and R) or a polygon (3 points or more). The first of them that is set counts.
type Region struct {
	ID      string
	Rect    image.Rectangle
	Center  image.Point
	R       int
	Polygon []image.Point
}

// Contains says whether (x, y) is in the region; a polygon by the even-odd rule.
func (g Region) Contains(x, y int) bool {
	switch {
	case !g.Rect.Empty():
		return image.Pt(x, y).In(g.Rect)
	case g.R > 0:
		dx, dy := x-g.Center.X, y-g.Center.Y
		return dx*dx+dy*dy <= g.R*g.R
	case len(g.Polygon) >= 3:
		in := false
		px, py := float64(x), float64(y)
		for i, j := 0, len(g.Polygon)-1; i < len(g.Polygon); j, i = i, i+1 {
			a, b := g.Polygon[i], g.Polygon[j]
			ay, by := float64(a.Y), float64(b.Y)
			if (ay > py) != (by > py) && px < float64(b.X-a.X)*(py-ay)/(by-ay)+float64(a.X) {
				in = !in
			}
		}
		return in
	}
	return false
}

// HitRegion is the index of the first region that contains (x, y), or -1.
func HitRegion(regions []Region, x, y int) int {
	for i, g := range regions {
		if g.Contains(x, y) {
			return i
		}
	}
	return -1
}
