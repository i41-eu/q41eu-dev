package robot

import (
	"context"
	"errors"
	"image/color"
	"testing"

	"github.com/i41-eu/q41eu-dev/leds"
	"github.com/i41-eu/q41eu-dev/screen"
	"github.com/i41-eu/q41eu-dev/sound"
	"github.com/i41-eu/q41eu-raw/wire"
)

type fake struct {
	cmds     []string
	args     []map[string]any
	bins     []byte
	commands map[string]bool
	gone     bool
}

func (f *fake) ID() string        { return "fake" }
func (f *fake) Has(c string) bool { return f.commands[c] }
func (f *fake) Command(n string, a map[string]any) bool {
	f.cmds, f.args = append(f.cmds, n), append(f.args, a)
	return !f.gone
}
func (f *fake) Binary(k byte, p []byte) bool { f.bins = append(f.bins, k); return !f.gone }

func newFake(cmds ...string) *fake {
	f := &fake{commands: map[string]bool{}}
	for _, c := range cmds {
		f.commands[c] = true
	}
	return f
}

func TestActions(t *testing.T) {
	f := newFake("look", "say", "leds", "image", "emotion")
	if err := Look(f, 200, 99); err != nil || f.args[0]["yaw"] != 128.0 || f.args[0]["pitch"] != 85.0 {
		t.Fatalf("look clamps: %v %v", err, f.args)
	}
	if err := Gesture(f, "nod"); !errors.As(err, new(NotSupported)) {
		t.Fatalf("nod is not offered: %v", err)
	}
	if err := Emotion(f, "grumpy"); err == nil {
		t.Fatal("unknown emotion")
	}
	if err := LEDs(f, leds.Bar(0.5, color.RGBA{255, 0, 0, 255}), true); err != nil || f.cmds[len(f.cmds)-1] != "leds" {
		t.Fatalf("leds: %v", err)
	}
	if err := ShowCard(f, screen.Card{Text: "hi"}); err != nil || len(f.bins) != 1 || f.bins[0] != wire.BinShowJPEG {
		t.Fatalf("show: %v %v", err, f.bins)
	}
	f.gone = true
	if err := Say(f, "hello", 0); !errors.Is(err, ErrGone) {
		t.Fatalf("gone: %v", err)
	}
}

func TestPlay(t *testing.T) {
	f := newFake("speaker")
	if err := Play(context.Background(), f, sound.Chime("tada")); err != nil || len(f.bins) != 1 || f.bins[0] != wire.BinSpeakerPCM {
		t.Fatalf("play: %v %v", err, f.bins)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Play(ctx, f, sound.Silence(6000)); !errors.Is(err, context.Canceled) {
		t.Fatalf("a long sound stops with its context: %v", err)
	}
	if err := Play(context.Background(), newFake(), sound.Chime("tada")); err == nil {
		t.Fatal("no speaker")
	}
}
