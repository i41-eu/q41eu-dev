package devapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // decode pictures sent to "show"
	_ "image/png"
	"slices"
	"strings"
	"time"

	"github.com/i41-eu/q41eu-dev/gesture"
	"github.com/i41-eu/q41eu-dev/leds"
	"github.com/i41-eu/q41eu-dev/robot"
	"github.com/i41-eu/q41eu-dev/robothub"
	"github.com/i41-eu/q41eu-dev/screen"
	"github.com/i41-eu/q41eu-dev/sound"
)

// Param is one argument of an action (for checking, the docs and MCP's input schemas).
type Param struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"` // string, number, boolean, array (of strings), object
	Help     string   `json:"help"`
	Required bool     `json:"required,omitempty"`
	Enum     []string `json:"enum,omitempty"`
}

// Action is something a program or an agent asks the robot to do: POST /v1/robots/{id}/{name}
// and MCP's tool robot_{name}.
type Action struct {
	Name    string  `json:"name"`
	Help    string  `json:"help"`
	Params  []Param `json:"params"`
	Offline bool    `json:"-"` // works while the robot is offline (it answers so)
	run     func(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error)
}

// Actions are all of them, in the order the docs list them.
var Actions []Action

func init() {
	Actions = []Action{
		{Name: "state", Offline: true,
			Help: "Read the robot's state: online or not, model, firmware, its commands, its latest readings (units in their names: battery_pct, light_lux, head_yaw_deg, …) and its last events.",
			run: func(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
				info := a.robotInfo(id)
				ev, _, _ := a.eventsAfter(id, 0, nil)
				info["last_events"] = ev[max(0, len(ev)-10):]
				return info, nil
			}},
		{Name: "show",
			Help: "Show a card on the robot's screen (a title, big text that sizes itself, a note, a progress bar), or a picture; clear takes it away.",
			Params: []Param{{Name: "text", Type: "string", Help: "big text in the middle"},
				{Name: "title", Type: "string", Help: "small text at the top, in the colour"},
				{Name: "note", Type: "string", Help: "small text at the bottom"},
				{Name: "color", Type: "string", Help: "the text's colour (and the title's and the bar's): #rrggbb or a name (red, orange, yellow, green, cyan, blue, purple, pink, white); default white text, a blue title"},
				{Name: "background", Type: "string", Help: "the card's background: #rrggbb or a name; default dark blue"},
				{Name: "progress", Type: "number", Help: "0..1: a progress bar"},
				{Name: "image", Type: "string", Help: "a JPEG or PNG, base64 (instead of a card; fitted to 320x240)"},
				{Name: "clear", Type: "boolean", Help: "take the card or picture away"}},
			run: func(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
				if in.bool("clear") {
					return nil, robot.Clear(r)
				}
				if s := in.str("image"); s != "" {
					b, err := base64.StdEncoding.DecodeString(s)
					if err != nil {
						return nil, fmt.Errorf("image: not base64: %w", err)
					}
					img, _, err := image.Decode(bytes.NewReader(b))
					if err != nil {
						return nil, fmt.Errorf("image: %w", err)
					}
					return nil, robot.Show(r, img)
				}
				card := screen.Card{Title: in.str("title"), Text: in.str("text"), Note: in.str("note")}
				if card.Title+card.Text+card.Note == "" {
					return nil, errors.New("text, title, note, image or clear")
				}
				if c := in.str("color"); c != "" {
					col, err := leds.Parse(c)
					if err != nil {
						return nil, err
					}
					card.Color = col
				}
				if c := in.str("background"); c != "" {
					col, err := leds.Parse(c)
					if err != nil {
						return nil, err
					}
					col.A = 255
					card.Background = col
				}
				if p, ok := in.num("progress"); ok {
					card.Progress = &p
				}
				return nil, robot.ShowCard(r, card)
			}},
		{Name: "say",
			Help:   "A speech bubble over the robot's face (needs the face: show clear first if a card is up).",
			Params: []Param{{Name: "text", Type: "string", Required: true, Help: "at most 80 characters"}, {Name: "seconds", Type: "number", Help: "how long (default 6, at most 30)"}},
			run: func(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
				s, _ := in.num("seconds")
				return nil, robot.Say(r, in.str("text"), s)
			}},
		{Name: "face",
			Help: "The robot's face: an emotion; or its own screen under cards: the face or its readings (its sensors live).",
			Params: []Param{{Name: "emotion", Type: "string", Enum: robot.Emotions, Help: "the face's emotion"},
				{Name: "view", Type: "string", Enum: []string{"face", "readings"}, Help: "the robot's own screen"}},
			run: func(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
				if v := in.str("view"); v != "" {
					if err := robot.View(r, v); err != nil {
						return nil, err
					}
				}
				if e := in.str("emotion"); e != "" {
					return nil, robot.Emotion(r, e)
				}
				return nil, nil
			}},
		{Name: "move",
			Help: "Move the head: a gesture (nod, shake, home = face forward), or look at an angle.",
			Params: []Param{{Name: "gesture", Type: "string", Enum: []string{"nod", "shake", "home"}, Help: "a gesture"},
				{Name: "yaw", Type: "number", Help: "degrees, -128..128, + to the robot's left"},
				{Name: "pitch", Type: "number", Help: "degrees, 5..85, up"}},
			run: func(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
				if g := in.str("gesture"); g != "" {
					return nil, robot.Gesture(r, g)
				}
				yaw, okY := in.num("yaw")
				pitch, okP := in.num("pitch")
				if !okY && !okP {
					return nil, errors.New("gesture, or yaw and pitch")
				}
				if !okP {
					pitch = r.Telemetry()["head_pitch_deg"]
				}
				if !okY {
					yaw = r.Telemetry()["head_yaw_deg"]
				}
				return nil, robot.Look(r, yaw, pitch)
			}},
		{Name: "leds",
			Help: "The 12 LEDs on the head (two strips of 6): one colour, a bar from the screen end back (a level), single LEDs, an effect the robot animates, or off. Colours are corrected to look on the LEDs as on a screen.",
			Params: []Param{{Name: "color", Type: "string", Help: "#rrggbb or a name; all LEDs, or the bar's / effect's colour"},
				{Name: "bar", Type: "number", Help: "0..1: light both strips from the screen end, this far"},
				{Name: "pixels", Type: "array", Help: "12 colours: left strip 0-5 from the screen end back, right strip 6-11 from the back to the screen end"},
				{Name: "effect", Type: "string", Enum: robot.Effects, Help: "an animation on the robot"},
				{Name: "speed", Type: "number", Help: "the effect's speed, 0.2..5 (default 1)"},
				{Name: "seconds", Type: "number", Help: "the effect's length (0 = until the next leds)"},
				{Name: "off", Type: "boolean", Help: "all off"}},
			run: runLEDs},
		{Name: "sound",
			Help: "A sound from the speaker: a named chime, or notes like \"C5:120 E5:120 G5:300\" (note:ms, - is a rest).",
			Params: []Param{{Name: "chime", Type: "string", Enum: chimeNames(), Help: "a ready-made sound"},
				{Name: "notes", Type: "string", Help: "a melody: note:ms ..., up to 30 s"},
				{Name: "bell", Type: "boolean", Help: "the notes struck like a bell"},
				{Name: "volume", Type: "number", Help: "the speaker's volume first, 0..100"}},
			run: func(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
				if v, ok := in.num("volume"); ok {
					if err := robot.Volume(r, v); err != nil {
						return nil, err
					}
				}
				var pcm []int16
				switch {
				case in.str("chime") != "":
					if pcm = sound.Chime(in.str("chime")); pcm == nil {
						return nil, fmt.Errorf("chime %q: one of %v", in.str("chime"), chimeNames())
					}
				case in.str("notes") != "":
					var err error
					if pcm, err = sound.Melody(in.str("notes"), 0.9, in.bool("bell")); err != nil {
						return nil, err
					}
				default:
					return nil, errors.New("chime or notes")
				}
				return map[string]any{"ms": sound.Ms(pcm)}, robot.Play(ctx, r, pcm)
			}},
		{Name: "ask",
			Help: "Ask the person at the robot: the question and 2-4 answers as buttons on its screen; returns the answer they tap, or timed_out.",
			Params: []Param{{Name: "question", Type: "string", Required: true, Help: "short: it is shown big"},
				{Name: "answers", Type: "array", Required: true, Help: "2-4 short answers"},
				{Name: "timeout_s", Type: "number", Help: "how long to wait (default 30, at most 120)"}},
			run: runAsk},
		{Name: "events", Offline: true,
			Help: "What people did at the robot (tap, long_press, pat, pet, stroke, shaken, picked_up, near, away, tag, tag_removed, remote, button, plugged, unplugged), after a sequence number; waits up to seconds for the next one.",
			Params: []Param{{Name: "after", Type: "number", Help: "only events after this seq (0: the last ones kept)"},
				{Name: "names", Type: "array", Help: "only these events"},
				{Name: "seconds", Type: "number", Help: "wait this long for one (default 0, at most 60)"}},
			run: func(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
				after, _ := in.num("after")
				wait, _ := in.num("seconds")
				names := in.strs("names")
				deadline := time.After(time.Duration(min(60, max(0, wait)) * float64(time.Second)))
				for {
					ev, last, wake := a.eventsAfter(id, int64(after), names)
					if len(ev) > 0 || wait <= 0 {
						return map[string]any{"events": ev, "last_seq": last}, nil
					}
					select {
					case <-wake:
					case <-deadline:
						return map[string]any{"events": []seqEvent{}, "last_seq": last}, nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
			}},
		{Name: "command",
			Help: "Any of the robot's own commands, as the Raw data app sends them (the q41eu-raw readme lists them), for what the other actions do not cover. Its apps and its manager's settings are not reachable here.",
			Params: []Param{{Name: "command", Type: "string", Required: true, Help: "e.g. sticker, brightness, ir_send"},
				{Name: "args", Type: "object", Help: "the command's arguments"}},
			run: func(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
				c := in.str("command")
				if slices.Contains(denied, c) {
					return nil, fmt.Errorf("%s: not through the dev API (the robot's apps and settings are its owner's, on the robot or its manager)", c)
				}
				if !r.Has(c) {
					return nil, robot.NotSupported(c)
				}
				m, _ := in["args"].(map[string]any)
				if !r.Command(c, m) {
					return nil, robot.ErrGone
				}
				return nil, nil
			}},
	}
}

func init() { // no parameters is an empty list in JSON, not null
	for i := range Actions {
		if Actions[i].Params == nil {
			Actions[i].Params = []Param{}
		}
	}
}

// denied: commands that change which apps the robot runs or how, never a program's to send.
var denied = []string{"server_switch", "server_e2e", "e2e_forget", "launch", "restart", "automation", "car_enable", "standby",
	"camera", "mic", "imu_stream", "touch_stream", "light_stream"}

func chimeNames() []string {
	var out []string
	for n := range sound.Chimes {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

func runLEDs(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
	if in.bool("off") {
		return nil, robot.LEDs(r, leds.All(leds.Names["off"]), false)
	}
	col := leds.Names["blue"]
	if c := in.str("color"); c != "" {
		var err error
		if col, err = leds.Parse(c); err != nil {
			return nil, err
		}
	}
	if e := in.str("effect"); e != "" {
		speed, _ := in.num("speed")
		secs, _ := in.num("seconds")
		return nil, robot.Effect(r, e, leds.All(col), speed, secs)
	}
	if px := in.strs("pixels"); len(px) > 0 {
		if len(px) != leds.N {
			return nil, fmt.Errorf("pixels: %d colours, want %d", len(px), leds.N)
		}
		var f leds.Frame
		for i, s := range px {
			c, err := leds.Parse(s)
			if err != nil {
				return nil, err
			}
			f[i] = c
		}
		return nil, robot.LEDs(r, f, true)
	}
	if b, ok := in.num("bar"); ok {
		return nil, robot.LEDs(r, leds.Bar(b, col), true)
	}
	if in.str("color") == "" {
		return nil, errors.New("color, bar, pixels, effect or off")
	}
	return nil, robot.LEDs(r, leds.All(col), true)
}

// runAsk shows the question, waits for a tap on an answer, then takes the question away.
func runAsk(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
	answers := in.strs("answers")
	if len(answers) < 2 || len(answers) > 4 || in.str("question") == "" {
		return nil, errors.New("a question and 2-4 answers")
	}
	timeout, _ := in.num("timeout_s")
	if timeout <= 0 {
		timeout = 30
	}
	img, buttons := screen.Ask(in.str("question"), answers, leds.Names["blue"])
	if err := robot.Show(r, img); err != nil {
		return nil, err
	}
	if r.Has("speaker") {
		robot.Play(ctx, r, sound.Chime("ask"))
	}
	q := &question{buttons: buttons, answer: make(chan int, 1)}
	a.mu.Lock()
	rs := a.stateFor(id)
	if old := rs.ask; old != nil {
		select {
		case old.answer <- -1: // a newer question ends it
		default:
		}
	}
	rs.ask = q
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		if rs.ask == q {
			rs.ask = nil
		}
		a.mu.Unlock()
	}()
	select {
	case i := <-q.answer:
		if i < 0 {
			return map[string]any{"answer": "", "replaced": true}, nil
		}
		robot.ShowCard(r, screen.Card{Text: answers[i], Color: leds.Names["green"], Note: "✓"})
		if r.Has("speaker") {
			robot.Play(ctx, r, sound.Chime("yes"))
		}
		time.AfterFunc(1200*time.Millisecond, func() { robot.Clear(r) })
		return map[string]any{"answer": answers[i], "index": i}, nil
	case <-time.After(time.Duration(min(120, timeout) * float64(time.Second))):
		robot.Clear(r)
		return map[string]any{"answer": "", "timed_out": true}, nil
	case <-ctx.Done():
		robot.Clear(r)
		return nil, ctx.Err()
	case <-r.Done():
		return nil, robot.ErrGone
	}
}

// run runs an action for someone allowed to use the robot.
func (a *App) run(ctx context.Context, acc access, id, name string, in args) (any, error) {
	if !slices.Contains(acc.robots, id) {
		return nil, errNoRobot
	}
	who := "browser:" + acc.sid
	if acc.key != nil {
		who = "key:" + acc.key.ID
	}
	if !a.actions.take(who) {
		return nil, errTooMany
	}
	i := slices.IndexFunc(Actions, func(x Action) bool { return x.Name == name })
	if i < 0 {
		return nil, fmt.Errorf("no action %q (see /v1/actions)", name)
	}
	act := Actions[i]
	if err := in.check(act.Params); err != nil {
		return nil, err
	}
	r := a.hub.Robot(id)
	if r == nil && !act.Offline {
		return nil, robot.ErrGone
	}
	by := "this page"
	if acc.key != nil {
		by = "key " + acc.key.Name
	}
	if name != "state" && name != "events" {
		a.publish("action", map[string]any{"robot": id, "action": name, "args": in, "by": by, "at": time.Now()})
	}
	return act.run(ctx, a, id, r, in)
}

var errNoRobot = errors.New("no such robot for this key or browser")

var errTooMany = fmt.Errorf("too many requests: at most %d a second (bursts of %d) per key", actionsPerSecond, actionsBurst)

// args are an action's arguments, as decoded JSON.
type args map[string]any

func (in args) str(k string) string { s, _ := in[k].(string); return strings.TrimSpace(s) }
func (in args) bool(k string) bool  { b, _ := in[k].(bool); return b }
func (in args) num(k string) (float64, bool) {
	f, ok := in[k].(float64)
	return f, ok
}
func (in args) strs(k string) []string {
	var out []string
	if l, ok := in[k].([]any); ok {
		for _, v := range l {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// check: the types are right, required ones are there, nothing unknown.
func (in args) check(params []Param) error {
	for k, v := range in {
		i := slices.IndexFunc(params, func(p Param) bool { return p.Name == k })
		if i < 0 {
			return fmt.Errorf("unknown argument %q", k)
		}
		p := params[i]
		ok := false
		switch p.Type {
		case "string":
			s, isStr := v.(string)
			ok = isStr && (len(p.Enum) == 0 || slices.Contains(p.Enum, s))
		case "number":
			_, ok = v.(float64)
		case "boolean":
			_, ok = v.(bool)
		case "array":
			_, ok = v.([]any)
		case "object":
			_, ok = v.(map[string]any)
		}
		if !ok {
			if len(p.Enum) > 0 {
				return fmt.Errorf("%s: one of %v", k, p.Enum)
			}
			return fmt.Errorf("%s: want a %s", k, p.Type)
		}
	}
	for _, p := range params {
		if _, there := in[p.Name]; p.Required && !there {
			return fmt.Errorf("%s is required", p.Name)
		}
	}
	return nil
}

// eventNames for the docs.
func eventNames() []string {
	var out []string
	for _, n := range gesture.Names {
		out = append(out, n.Name)
	}
	return out
}
