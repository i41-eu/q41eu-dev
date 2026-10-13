package screen

import (
	"fmt"
	"image"
	"image/color"
	"testing"
)

// A menu of 2-6 tiles: every tile on the screen, apart from the others and from the title, hit at
// its middle and not between tiles; a picture is drawn in its tile.
func TestMenu(t *testing.T) {
	screen := image.Rect(0, 0, W, H)
	for n := 2; n <= MaxMenuItems; n++ {
		for _, title := range []string{"", "Snack?"} {
			var items []MenuItem
			for i := range n {
				items = append(items, MenuItem{Label: fmt.Sprintf("Item %d", i+1)})
			}
			img, buttons := Menu(title, items, color.RGBA{})
			if img.Bounds() != screen || len(buttons) != n {
				t.Fatalf("%d items: %v, %d buttons", n, img.Bounds(), len(buttons))
			}
			for i, b := range buttons {
				r := b.Rect
				if !r.In(screen) || r.Dx() < 80 || r.Dy() < 60 || (title != "" && r.Min.Y < 40) {
					t.Errorf("%d items, %q: tile %d at %v", n, title, i, r)
				}
				mid := r.Min.Add(r.Size().Div(2))
				if got := Hit(buttons, mid.X, mid.Y); got != i {
					t.Errorf("%d items: the middle of tile %d hits %d", n, i, got)
				}
				for j, o := range buttons[i+1:] {
					if r.Overlaps(o.Rect) {
						t.Errorf("%d items: tiles %d and %d overlap", n, i, i+1+j)
					}
				}
				if r.Max.X < W-5 && Hit(buttons, r.Max.X+2, mid.Y) >= 0 { // the gap after it
					t.Errorf("%d items: the gap right of tile %d hits a tile", n, i)
				}
			}
		}
	}
	red := image.NewRGBA(image.Rect(0, 0, 40, 20))
	Fill(red, red.Bounds(), color.RGBA{255, 0, 0, 255})
	img, buttons := Menu("", []MenuItem{{Label: "Red", Image: red}, {Label: "None"}}, color.RGBA{})
	r := buttons[0].Rect
	if c := img.RGBAAt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2-10); c.R < 200 || c.G > 60 {
		t.Errorf("the picture is not in its tile: %v", c)
	}
}

func TestRegions(t *testing.T) {
	regions := []Region{
		{ID: "box", Rect: image.Rect(0, 0, 100, 50)},
		{ID: "sun", Center: image.Pt(260, 40), R: 30},
		{ID: "v", Polygon: []image.Point{{0, 240}, {80, 120}, {160, 240}, {80, 200}}}, // concave: a notch at the bottom
		{ID: "under the box", Rect: image.Rect(0, 0, 320, 240)},
	}
	for _, c := range []struct {
		x, y int
		want string
	}{
		{10, 10, "box"}, {99, 49, "box"}, {100, 10, "under the box"},
		{260, 40, "sun"}, {285, 40, "sun"}, {285, 65, "under the box"},
		{80, 150, "v"}, {40, 200, "v"}, {40, 225, "under the box"}, {80, 225, "under the box"}, {80, 125, "v"}, {20, 140, "under the box"},
	} {
		got := ""
		if i := HitRegion(regions, c.x, c.y); i >= 0 {
			got = regions[i].ID
		}
		if got != c.want {
			t.Errorf("(%d, %d): %q, want %q", c.x, c.y, got, c.want)
		}
	}
	if HitRegion(regions[:3], 300, 200) != -1 || (Region{ID: "nothing"}).Contains(1, 1) {
		t.Error("a tap outside every region hits one")
	}
}
