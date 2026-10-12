// Package gesture turns the robot's raw input events into what a person did: a tap, a pat or a
// longer pet on the head, a stroke, the robot picked up or shaken, someone coming near, a tag or
// a remote. The robot reports only primary data (presses, releases with their length, swipes,
// readings); meaning is made here, on the app's side, once for every app.
package gesture

import (
	"time"

	"github.com/i41-eu/q41eu-raw/wire"
)

// Event is something a person did.
type Event struct {
	Name string         `json:"name"`
	Data map[string]any `json:"data,omitempty"`
	At   time.Time      `json:"at"`
}

// Names are the events, with what they mean.
var Names = []struct{ Name, Means string }{
	{"tap", "a short touch on the screen: data x, y (screen pixels, 320x240), and sprite when it hit a tappable picture"},
	{"long_press", "a long touch on the screen: data x, y"},
	{"pat", "a short touch on the head (under 0.7 s): data ms"},
	{"pet", "a longer touch on the head: data seconds"},
	{"stroke", "a finger along the head: data direction forward or back"},
	{"shaken", "the robot was shaken"},
	{"picked_up", "the robot was picked up"},
	{"near", "something came close to the front (a hand, a face)"},
	{"away", "it went away again"},
	{"tag", "an NFC tag or card held near the head: data uid, type, text (an NTAG sticker's link or text)"},
	{"tag_removed", "the tag was taken away: data uid"},
	{"remote", "an infrared remote's button: data address, command (NEC) or raw"},
	{"button", "the power button: data press short or long"},
	{"plugged", "a USB cable plugged in"},
	{"unplugged", "the USB cable taken out"},
}

// PatMs: a touch on the head shorter than this is a pat, longer a pet.
const PatMs = 700

// Recognizer reads one robot's events. Not safe for concurrent use.
type Recognizer struct{}

// Feed takes a raw event and gives what it means (often nothing).
func (r *Recognizer) Feed(ev wire.RobotEventBody, now time.Time) []Event {
	d := ev.Data
	pick := func(keys ...string) map[string]any {
		out := map[string]any{}
		for _, k := range keys {
			if v, ok := d[k]; ok {
				out[k] = v
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	one := func(name string, data map[string]any) []Event {
		return []Event{{Name: name, Data: data, At: now}}
	}
	switch ev.Name {
	case "screen_tap":
		return one("tap", pick("x", "y", "sprite", "sprite_x", "sprite_y", "asset"))
	case "screen_long_press":
		return one("long_press", pick("x", "y", "sprite"))
	case "head_release":
		ms, _ := d["ms"].(float64)
		if ms < PatMs {
			return one("pat", map[string]any{"ms": ms})
		}
		return one("pet", map[string]any{"seconds": float64(int(ms/100)) / 10})
	case "head_swipe_forward":
		return one("stroke", map[string]any{"direction": "forward"})
	case "head_swipe_backward":
		return one("stroke", map[string]any{"direction": "back"})
	case "shake":
		return one("shaken", nil)
	case "pickup":
		return one("picked_up", nil)
	case "proximity_near":
		return one("near", nil)
	case "proximity_far":
		return one("away", nil)
	case "nfc_tag":
		return one("tag", pick("uid", "type", "text"))
	case "nfc_removed":
		return one("tag_removed", pick("uid"))
	case "ir_received":
		if d["protocol"] == "nec" {
			return one("remote", pick("address", "command"))
		}
		return one("remote", pick("raw"))
	case "power_button":
		return one("button", pick("press"))
	case "usb_plugged":
		return one("plugged", nil)
	case "usb_unplugged":
		return one("unplugged", nil)
	}
	return nil
}
