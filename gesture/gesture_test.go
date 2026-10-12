package gesture

import (
	"testing"
	"time"

	"github.com/i41-eu/q41eu-raw/wire"
)

func TestFeed(t *testing.T) {
	var r Recognizer
	now := time.Now()
	cases := []struct {
		ev   wire.RobotEventBody
		want string
		key  string
		val  any
	}{
		{wire.RobotEventBody{Name: "screen_tap", Data: map[string]any{"x": 10.0, "y": 20.0}}, "tap", "x", 10.0},
		{wire.RobotEventBody{Name: "head_release", Data: map[string]any{"ms": 300.0}}, "pat", "ms", 300.0},
		{wire.RobotEventBody{Name: "head_release", Data: map[string]any{"ms": 2340.0}}, "pet", "seconds", 2.3},
		{wire.RobotEventBody{Name: "head_swipe_backward"}, "stroke", "direction", "back"},
		{wire.RobotEventBody{Name: "nfc_tag", Data: map[string]any{"uid": "04:A2", "text": "hi", "sak": 0.0}}, "tag", "text", "hi"},
		{wire.RobotEventBody{Name: "ir_received", Data: map[string]any{"protocol": "nec", "address": 128.0, "command": 4.0, "raw": "9000"}}, "remote", "command", 4.0},
		{wire.RobotEventBody{Name: "pickup"}, "picked_up", "", nil},
	}
	for _, c := range cases {
		got := r.Feed(c.ev, now)
		if len(got) != 1 || got[0].Name != c.want {
			t.Errorf("%s: %+v, want %s", c.ev.Name, got, c.want)
			continue
		}
		if c.key != "" && got[0].Data[c.key] != c.val {
			t.Errorf("%s: %s = %v, want %v", c.ev.Name, c.key, got[0].Data[c.key], c.val)
		}
	}
	if got := r.Feed(wire.RobotEventBody{Name: "nfc_tag", Data: map[string]any{"sak": 0.0}}, now); got[0].Data["sak"] != nil {
		t.Error("only the documented data")
	}
	if got := r.Feed(wire.RobotEventBody{Name: "touch_down"}, now); got != nil {
		t.Errorf("raw touches mean nothing yet: %+v", got)
	}
}
