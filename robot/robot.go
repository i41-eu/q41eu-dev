// Package robot is the higher-level API to a connected robot: show a card or a picture, say
// something, a face, look, nod, LEDs, sounds. Each call checks that the robot offers the command
// and turns it into the robot's own commands and messages (the q41eu-raw readme lists them).
package robot

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"time"

	"github.com/i41-eu/q41eu-dev/leds"
	"github.com/i41-eu/q41eu-dev/screen"
	"github.com/i41-eu/q41eu-dev/sound"
	"github.com/i41-eu/q41eu-raw/wire"
)

// Target is a connected robot: a *robothub.Robot.
type Target interface {
	ID() string
	Has(command string) bool
	Command(name string, args map[string]any) bool
	Binary(kind byte, payload []byte) bool
}

// ErrGone: the robot is offline, or its send queue is full.
var ErrGone = errors.New("the robot is not connected")

// NotSupported: the robot does not offer what was asked.
type NotSupported string

func (e NotSupported) Error() string { return "the robot does not offer " + string(e) }

func command(t Target, name string, args map[string]any) error {
	if !t.Has(name) {
		return NotSupported(name)
	}
	if !t.Command(name, args) {
		return ErrGone
	}
	return nil
}

// Show puts a picture on the screen (fitted to 320x240), over the face or the readings.
func Show(t Target, img image.Image) error {
	if !t.Has("image") {
		return NotSupported("pictures")
	}
	if !t.Binary(wire.BinShowJPEG, screen.JPEG(img)) {
		return ErrGone
	}
	return nil
}

// ShowCard shows a card (title, big text, note, progress).
func ShowCard(t Target, c screen.Card) error { return Show(t, c.Draw()) }

// Clear takes the picture away: the face or the readings again.
func Clear(t Target) error { return command(t, "face", nil) }

// View is the robot's own screen under pictures: "face" or "readings".
func View(t Target, name string) error {
	if name != "face" && name != "readings" {
		return fmt.Errorf("view %q: face or readings", name)
	}
	return command(t, "view", map[string]any{"name": name})
}

// Emotions the face shows.
var Emotions = []string{"neutral", "happy", "angry", "sad", "doubt", "sleepy"}

// Emotion sets the face's emotion.
func Emotion(t Target, name string) error {
	for _, e := range Emotions {
		if e == name {
			return command(t, "emotion", map[string]any{"name": name})
		}
	}
	return fmt.Errorf("emotion %q: one of %v", name, Emotions)
}

// Say shows a speech bubble over the face for some seconds (default 6; at most 80 characters).
func Say(t Target, text string, seconds float64) error {
	if text == "" {
		return errors.New("nothing to say")
	}
	if r := []rune(text); len(r) > 80 {
		text = string(r[:80])
	}
	if seconds <= 0 {
		seconds = 6
	}
	return command(t, "say", map[string]any{"text": text, "seconds": min(seconds, 30)})
}

// Look turns the head: yaw -128..128 degrees (+ to the robot's left), pitch 5..85 (up). Outside
// values are clamped (M5Stack: pitch beyond 5-85 can harm the servo).
func Look(t Target, yaw, pitch float64) error {
	yaw = math.Round(min(128, max(-128, yaw)))
	pitch = math.Round(min(85, max(5, pitch)))
	return command(t, "look", map[string]any{"yaw": yaw, "pitch": pitch})
}

// Gesture is "nod", "shake" or "home" (facing forward).
func Gesture(t Target, name string) error {
	switch name {
	case "nod", "shake", "home":
		return command(t, name, nil)
	}
	return fmt.Errorf("gesture %q: nod, shake or home", name)
}

// LEDs lights the 12 LEDs; correct: screen colours made to look right on the LEDs (leds.Correct).
func LEDs(t Target, f leds.Frame, correct bool) error { return command(t, "leds", f.Args(correct)) }

// Effects the robot animates on its LEDs itself.
var Effects = []string{"rainbow", "breathe", "chase", "blink", "off"}

// Effect runs an LED animation on the robot (speed 0.2-5, seconds 0 = until the next LEDs).
func Effect(t Target, name string, c leds.Frame, speed, seconds float64) error {
	ok := false
	for _, e := range Effects {
		ok = ok || e == name
	}
	if !ok {
		return fmt.Errorf("effect %q: one of %v", name, Effects)
	}
	if speed <= 0 {
		speed = 1
	}
	return command(t, "leds", map[string]any{"effect": name, "color": leds.Hex(leds.Correct(c[0])),
		"speed": min(5, max(0.2, speed)), "seconds": min(3600, max(0, seconds))})
}

// Volume sets the speaker volume, 0-100.
func Volume(t Target, pct float64) error {
	return command(t, "volume", map[string]any{"value": math.Round(min(100, max(0, pct)))})
}

// Play plays PCM at sound.Rate through the speaker, paced so the robot's ~3 s buffer never
// overflows; it returns when the last part is sent (about the sound's length minus 2.5 s).
func Play(ctx context.Context, t Target, pcm []int16) error {
	if !t.Has("speaker") {
		return NotSupported("speaker")
	}
	chunks := sound.Chunks(pcm)
	for i, c := range chunks {
		if !t.Binary(wire.BinSpeakerPCM, c) {
			return ErrGone
		}
		if i == len(chunks)-1 {
			break
		}
		select { // the next part shortly before this one ends
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(sound.MaxChunkMs-300) * time.Millisecond):
		}
	}
	return nil
}
