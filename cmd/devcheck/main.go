// Command devcheck tests Dev end to end on a real robot: it runs every action through the API (as
// a program would, with SDEV_URL, SDEV_ROBOT and SDEV_KEY), takes the robot's own screenshot over
// USB after each (i41eu-usb, from i41eu-manager), checks the pixels (the text's colour, the
// LEDs as the readings screen shows them, a question's buttons) and writes a report with every
// screenshot: <dir>/index.html.
//
//	devcheck -usb ./i41eu-usb -o /tmp/devcheck [-tap]
//
// -tap also answers a question and makes a tap event with a USB tap (the robot asks once
// "Let the computer on USB use the screen?" unless it runs a test build). The robot ends on its
// face with its LEDs off.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"html"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/i41-eu/q41eu-dev/client"
)

type step struct {
	name   string
	do     func(ctx context.Context, c *client.Client) (string, error) // what was done, as a line
	wait   time.Duration                                               // before the screenshot
	checks []check
	noShot bool
}

type check struct {
	what string
	ok   func(img image.Image) (bool, string)
}

type result struct {
	name, did, err, shot string
	checks               []string
	failed               bool
}

func main() {
	usb := flag.String("usb", "i41eu-usb", "the i41eu-usb tool (screenshots and taps over USB)")
	out := flag.String("o", "devcheck-report", "the report's directory")
	tap := flag.Bool("tap", false, "answer the question and make a tap event with USB taps")
	flag.Parse()
	c, err := client.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	shoot := func(name string) (string, image.Image, error) {
		file := filepath.Join(*out, name+".jpg")
		cmd := exec.Command(*usb, "screenshot", "-o", file)
		if b, err := cmd.CombinedOutput(); err != nil {
			return "", nil, fmt.Errorf("screenshot: %v %s", err, b)
		}
		f, err := os.Open(file)
		if err != nil {
			return "", nil, err
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		return filepath.Base(file), img, err
	}
	usbTap := func(x, y int) error {
		b, err := exec.Command(*usb, "tap", "-x", fmt.Sprint(x), "-y", fmt.Sprint(y)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("tap: %v %s", err, b)
		}
		return nil
	}

	var results []result
	failures := 0
	for i, s := range steps(*tap, usbTap) {
		r := result{name: s.name}
		did, err := s.do(ctx, c)
		r.did = did
		if err != nil {
			r.err, r.failed = err.Error(), true
		}
		if !s.noShot {
			time.Sleep(s.wait)
			shot, img, err := shoot(fmt.Sprintf("%02d-%s", i+1, strings.ReplaceAll(s.name, " ", "-")))
			r.shot = shot
			if err != nil {
				r.err, r.failed = strings.TrimSpace(r.err+" "+err.Error()), true
			} else {
				for _, ch := range s.checks {
					ok, why := ch.ok(img)
					mark := "✓"
					if !ok {
						mark, r.failed = "✗", true
					}
					r.checks = append(r.checks, fmt.Sprintf("%s %s: %s", mark, ch.what, why))
				}
			}
		}
		if r.failed {
			failures++
		}
		status := "ok  "
		if r.failed {
			status = "FAIL"
		}
		fmt.Printf("%s %-28s %s %s\n", status, s.name, r.did, r.err)
		for _, ch := range r.checks {
			fmt.Println("       " + ch)
		}
		results = append(results, r)
	}
	writeReport(filepath.Join(*out, "index.html"), c, results)
	fmt.Printf("%d steps, %d failed; the report: %s\n", len(results), failures, filepath.Join(*out, "index.html"))
	if failures > 0 {
		os.Exit(1)
	}
}

// The robot's screen: 320x240. The readings screen's LED row: dots at y 32, x 71 + 13·i (+8 for
// the right strip), as the firmware draws them (readings_view.cpp).
func ledDot(i int) image.Point {
	x := 71 + 13*i
	if i >= 6 {
		x += 8
	}
	return image.Pt(x, 32)
}

func steps(tap bool, usbTap func(x, y int) error) []step {
	act := func(action string, args map[string]any) func(ctx context.Context, c *client.Client) (string, error) {
		return func(ctx context.Context, c *client.Client) (string, error) {
			return fmt.Sprintf("%s %v", action, args), c.Do(ctx, action, args, nil)
		}
	}
	green, red, black := color.RGBA{40, 220, 60, 255}, color.RGBA{255, 40, 40, 255}, color.RGBA{0, 0, 0, 255}
	s := []step{
		{name: "state", noShot: true, do: func(ctx context.Context, c *client.Client) (string, error) {
			st, err := c.State(ctx)
			if err == nil && !st.Online {
				err = fmt.Errorf("the robot is offline")
			}
			return fmt.Sprintf("online %v, firmware %s, %d readings", st.Online, st.Firmware, len(st.Readings)), err
		}},
		{name: "show white text", do: act("show", map[string]any{"title": "Check", "text": "White text", "note": "devcheck"}), wait: time.Second,
			checks: []check{share("the text is white", image.Rect(20, 60, 300, 180), color.RGBA{242, 244, 255, 255}, 0.03)}},
		{name: "show green text on black", do: act("show", map[string]any{"text": "Green", "color": "green", "background": "black"}), wait: time.Second,
			checks: []check{share("the text is green", image.Rect(20, 40, 300, 200), green, 0.05), share("the background is black", image.Rect(0, 0, 320, 20), black, 0.9)}},
		{name: "show progress", do: act("show", map[string]any{"title": "Half", "text": "50%", "color": "red", "progress": 0.5}), wait: time.Second,
			checks: []check{pixel("the bar is red at its start", image.Pt(30, 223), red), not("the bar's end is not red", image.Pt(290, 223), red)}},
		{name: "show a picture", do: showPicture, wait: time.Second,
			checks: []check{pixel("left half red", image.Pt(80, 120), color.RGBA{255, 0, 0, 255}), pixel("right half blue", image.Pt(240, 120), color.RGBA{0, 0, 255, 255})}},
		{name: "ask (a question)", wait: 2 * time.Second, do: func(ctx context.Context, c *client.Client) (string, error) {
			go c.Ask(context.Background(), "Check: tap Yes", "Yes", "No") // answered below, or it times out
			return "ask Check: tap Yes / Yes, No", nil
		}, checks: []check{share("the first button is blue", image.Rect(20, 180, 150, 230), color.RGBA{40, 90, 255, 255}, 0.3)}},
	}
	if tap {
		s = append(s, step{name: "tap the answer", wait: 500 * time.Millisecond, do: func(ctx context.Context, c *client.Client) (string, error) {
			_, last, _ := c.Events(ctx, 0, 0)
			if err := usbTap(80, 205); err != nil {
				return "", err
			}
			ev, _, err := c.Events(ctx, last, 5*time.Second, "tap")
			if err == nil && len(ev) == 0 {
				err = fmt.Errorf("no tap event")
			}
			return fmt.Sprintf("USB tap at 80,205: %d tap event(s)", len(ev)), err
		}, checks: []check{share("the answer's card is green", image.Rect(20, 60, 300, 180), green, 0.03)}})
	}
	s = append(s,
		step{name: "clear: the face", do: act("show", map[string]any{"clear": true}), wait: time.Second},
		step{name: "face happy", do: act("face", map[string]any{"emotion": "happy"}), wait: time.Second},
		step{name: "say", do: act("say", map[string]any{"text": "Hello from devcheck", "seconds": 5}), wait: 1500 * time.Millisecond},
		step{name: "sticker (command)", do: act("command", map[string]any{"command": "sticker", "args": map[string]any{"name": "heart", "seconds": 4}}), wait: 800 * time.Millisecond},
		step{name: "readings view", do: act("face", map[string]any{"view": "readings"}), wait: time.Second, checks: []check{
			pixel("the readings' background", image.Pt(3, 120), color.RGBA{0x10, 0x14, 0x2a, 255})}},
		step{name: "leds red", do: act("leds", map[string]any{"color": "red"}), wait: 800 * time.Millisecond, checks: []check{
			ledIs("LED 0 red", 0, "red"), ledIs("LED 11 red", 11, "red")}},
		step{name: "leds bar half green", do: act("leds", map[string]any{"bar": 0.5, "color": "green"}), wait: 800 * time.Millisecond, checks: []check{
			ledIs("LED 0 green", 0, "green"), ledIs("LED 3 off", 3, "off"), ledIs("LED 8 off", 8, "off"), ledIs("LED 11 green", 11, "green")}},
		step{name: "leds effect rainbow", do: act("leds", map[string]any{"effect": "rainbow", "seconds": 3}), wait: time.Second, checks: []check{
			{"the LEDs are lit", func(img image.Image) (bool, string) {
				lit := 0
				for i := 0; i < 12; i++ {
					if bright(img.At(ledDot(i).X, ledDot(i).Y)) > 60 {
						lit++
					}
				}
				return lit >= 10, fmt.Sprintf("%d of 12 lit", lit)
			}}}},
		step{name: "leds off", do: act("leds", map[string]any{"off": true}), wait: 800 * time.Millisecond, checks: []check{
			ledIs("LED 0 off", 0, "off"), ledIs("LED 6 off", 6, "off")}},
		step{name: "move nod", do: act("move", map[string]any{"gesture": "nod"}), wait: 1500 * time.Millisecond},
		step{name: "move look", do: act("move", map[string]any{"yaw": 25, "pitch": 40}), wait: 1500 * time.Millisecond},
		step{name: "move home", do: act("move", map[string]any{"gesture": "home"}), wait: 1500 * time.Millisecond},
		step{name: "sound chime", do: act("sound", map[string]any{"chime": "tada"}), wait: 200 * time.Millisecond},
		step{name: "sound notes", do: act("sound", map[string]any{"notes": "C5:100 E5:100 G5:200", "bell": true}), noShot: true},
		step{name: "bad argument refused", noShot: true, do: func(ctx context.Context, c *client.Client) (string, error) {
			err := c.Do(ctx, "move", map[string]any{"gesture": "wave"}, nil)
			if e, ok := err.(*client.Error); ok && e.Status == 400 {
				return "move wave: 400 " + e.Message, nil
			}
			return "", fmt.Errorf("move wave: want a 400, got %v", err)
		}},
		step{name: "back to the face", do: act("face", map[string]any{"view": "face", "emotion": "neutral"}), wait: time.Second},
	)
	return s
}

// showPicture sends a picture: red left, blue right.
func showPicture(ctx context.Context, c *client.Client) (string, error) {
	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 320; x++ {
			if x < 160 {
				img.Set(x, y, color.RGBA{255, 0, 0, 255})
			} else {
				img.Set(x, y, color.RGBA{0, 0, 255, 255})
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return "show image (red | blue PNG)", c.Do(ctx, "show", map[string]any{"image": base64.StdEncoding.EncodeToString(buf.Bytes())}, nil)
}

func rgb(c color.Color) (int, int, int) {
	r, g, b, _ := c.RGBA()
	return int(r >> 8), int(g >> 8), int(b >> 8)
}

func dist(a, b color.Color) float64 {
	r1, g1, b1 := rgb(a)
	r2, g2, b2 := rgb(b)
	return math.Sqrt(float64((r1-r2)*(r1-r2) + (g1-g2)*(g1-g2) + (b1-b2)*(b1-b2)))
}

func bright(c color.Color) int {
	r, g, b := rgb(c)
	return max(r, g, b)
}

const tol = 70 // JPEG and the panel's colours move them a little

func pixel(what string, p image.Point, want color.Color) check {
	return check{what, func(img image.Image) (bool, string) {
		got := img.At(p.X, p.Y)
		r, g, b := rgb(got)
		return dist(got, want) < tol, fmt.Sprintf("at %d,%d rgb(%d,%d,%d)", p.X, p.Y, r, g, b)
	}}
}

func not(what string, p image.Point, unwanted color.Color) check {
	return check{what, func(img image.Image) (bool, string) {
		got := img.At(p.X, p.Y)
		r, g, b := rgb(got)
		return dist(got, unwanted) >= tol, fmt.Sprintf("at %d,%d rgb(%d,%d,%d)", p.X, p.Y, r, g, b)
	}}
}

// share: at least min of the area's pixels are near the colour.
func share(what string, area image.Rectangle, want color.Color, min float64) check {
	return check{what, func(img image.Image) (bool, string) {
		n, all := 0, 0
		for y := area.Min.Y; y < area.Max.Y; y++ {
			for x := area.Min.X; x < area.Max.X; x++ {
				all++
				if dist(img.At(x, y), want) < tol {
					n++
				}
			}
		}
		f := float64(n) / float64(all)
		return f >= min, fmt.Sprintf("%.1f%% of the area (want ≥ %.1f%%)", 100*f, 100*min)
	}}
}

// ledIs checks an LED's dot on the readings screen: "off" dark, else the colour's hue (the dot
// shows the corrected LED colour, so the hue, not the exact value).
func ledIs(what string, i int, want string) check {
	return check{what, func(img image.Image) (bool, string) {
		p := ledDot(i)
		r, g, b := rgb(img.At(p.X, p.Y))
		desc := fmt.Sprintf("dot %d at %d,%d rgb(%d,%d,%d)", i, p.X, p.Y, r, g, b)
		switch want {
		case "off":
			return max(r, g, b) < 90, desc // the track's dark blue
		case "red":
			return r > 120 && r > 2*g && r > 2*b, desc
		case "green":
			return g > 90 && g > r+30 && g > b+30, desc
		}
		return false, desc
	}}
}

func writeReport(path string, c *client.Client, rs []result) {
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><meta charset="utf-8"><title>devcheck</title><style>body{font:15px system-ui;margin:20px;background:#f4f6ff;color:#1e2355}
.s{display:flex;gap:16px;align-items:flex-start;background:#fff;border:1px solid #dce1f5;border-radius:12px;padding:12px;margin:10px 0}
.s.fail{border-color:#e53935}img{width:320px;height:240px;border-radius:6px;background:#000}pre{white-space:pre-wrap;margin:4px 0;font-size:13px}.f{color:#c62828}</style>
<h1>devcheck</h1><p>%s · robot %s · %s</p>`, html.EscapeString(c.URL), html.EscapeString(c.Robot), time.Now().Format(time.RFC3339))
	for _, r := range rs {
		cls := "s"
		if r.failed {
			cls += " fail"
		}
		fmt.Fprintf(&b, `<div class="%s">`, cls)
		if r.shot != "" {
			fmt.Fprintf(&b, `<img src="%s" alt="">`, html.EscapeString(r.shot))
		}
		fmt.Fprintf(&b, `<div><b>%s</b><pre>%s</pre>`, html.EscapeString(r.name), html.EscapeString(r.did))
		if r.err != "" {
			fmt.Fprintf(&b, `<pre class="f">%s</pre>`, html.EscapeString(r.err))
		}
		for _, ch := range r.checks {
			fmt.Fprintf(&b, `<pre>%s</pre>`, html.EscapeString(ch))
		}
		b.WriteString(`</div></div>`)
	}
	os.WriteFile(path, []byte(b.String()), 0o644)
}
