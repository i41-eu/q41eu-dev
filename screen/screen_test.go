package screen

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// save writes the picture to $SHOTS (to look at), if set.
func save(t *testing.T, name string, img image.Image) {
	if dir := os.Getenv("SHOTS"); dir != "" {
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		png.Encode(f, img)
	}
}

func count(img *image.RGBA, r image.Rectangle, c color.RGBA) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if img.RGBAAt(x, y) == c {
				n++
			}
		}
	}
	return n
}

func TestCard(t *testing.T) {
	half := 0.5
	img := Card{Title: "Build", Text: "All tests pass", Note: "main · 2 min ago", Progress: &half}.Draw()
	save(t, "card", img)
	if count(img, image.Rect(0, 60, W, 180), White) < 200 {
		t.Fatal("no big text in the middle")
	}
	if img.RGBAAt(20, H-17) != Blue || img.RGBAAt(W-30, H-17) != Panel {
		t.Fatalf("the bar is not half full: %v %v", img.RGBAAt(20, H-17), img.RGBAAt(W-30, H-17))
	}
	long := Card{Text: "A much longer message that has to wrap over several lines and shrink to fit the screen of the robot"}.Draw()
	save(t, "card_long", long)
	if count(long, image.Rect(0, 0, W, H), White) < 200 {
		t.Fatal("no text")
	}
}

func TestFitText(t *testing.T) {
	size, lines := FitText("Hi", 300, 200, true)
	if size != 72 || len(lines) != 1 {
		t.Fatalf("a short word goes big: %v %v", size, lines)
	}
	size, lines = FitText("one two three four five six seven eight nine ten eleven twelve", 300, 100, true)
	if size > 34 || len(lines) < 2 {
		t.Fatalf("a long text shrinks and wraps: %v %v", size, lines)
	}
}

func TestAsk(t *testing.T) {
	img, buttons := Ask("Coffee or tea?", []string{"Coffee", "Tea", "Water"}, color.RGBA{})
	save(t, "ask", img)
	if len(buttons) != 3 {
		t.Fatalf("buttons %v", buttons)
	}
	for i, b := range buttons {
		c := b.Rect.Min.Add(b.Rect.Max).Div(2)
		if Hit(buttons, c.X, c.Y) != i {
			t.Fatalf("button %d not hit at its centre", i)
		}
	}
	if Hit(buttons, 5, 5) != -1 {
		t.Fatal("a hit outside the buttons")
	}
	if buttons[2].Rect.Dx() <= buttons[0].Rect.Dx() {
		t.Fatal("a lone third answer gets the whole row")
	}
}

func TestJPEG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 640, 360))
	b := JPEG(src)
	if len(b) < 100 || b[0] != 0xFF || b[1] != 0xD8 {
		t.Fatal("not a JPEG")
	}
	if Fit(src).Bounds() != image.Rect(0, 0, W, H) {
		t.Fatal("not fitted to the screen")
	}
}

// The colour colours the text (the title and the bar too); the background is the card's.
func TestCardColors(t *testing.T) {
	green := color.RGBA{40, 220, 60, 255}
	img := Card{Text: "All tests pass", Color: green, Background: color.RGBA{0, 0, 0, 255}}.Draw()
	save(t, "card_green", img)
	if count(img, image.Rect(0, 40, W, 200), green) < 300 {
		t.Fatal("the text is not green")
	}
	if count(img, image.Rect(0, 40, W, 200), White) > 0 {
		t.Fatal("white text left")
	}
	if img.RGBAAt(2, 2) != (color.RGBA{0, 0, 0, 255}) {
		t.Fatalf("the background: %v", img.RGBAAt(2, 2))
	}
	plain := Card{Text: "Hi"}.Draw()
	if count(plain, image.Rect(0, 40, W, 200), White) < 300 {
		t.Fatal("without a colour the text is white")
	}
}
