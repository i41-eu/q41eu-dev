package devapp

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // decode pictures sent to show, menu and clickmap
	_ "image/png"
	"math"
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
	Type     string   `json:"type"`            // string, number, boolean, array, object
	Items    string   `json:"items,omitempty"` // an array's items: string (when empty), object, or "string or object"
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
	Example string  `json:"-"` // a typical call's arguments, as JSON: the docs' snippets in every language
	Prompt  string  `json:"-"` // that call as a sentence to an AI agent (the docs' MCP snippet)
	Offline bool    `json:"-"` // works while the robot is offline (it answers so)
	run     func(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error)
}

// Actions are all of them, in the order the docs list them.
var Actions []Action

func init() {
	Actions = []Action{
		{Name: "state", Offline: true, Example: `{}`,
			Prompt: "How is the robot: is it online, and how full is its battery?",
			Help:   "Read the robot's state: online or not, model, firmware, its commands, its latest readings (units in their names: battery_pct, light_lux, head_yaw_deg, …) and its last events.",
			run: func(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
				info := a.robotInfo(id)
				ev, _, _ := a.eventsAfter(id, 0, nil)
				info["last_events"] = ev[max(0, len(ev)-10):]
				info["presence"] = a.presenceObject(id)
				return info, nil
			}},
		{Name: "show", Example: `{"title": "Build", "text": "All tests pass", "color": "green", "progress": 1}`,
			Prompt: "Show \"All tests pass\" on the robot, in green.",
			Help:   "Show a card on the robot's screen (a title, big text that sizes itself, a note, a progress bar), or a picture; clear takes it away.",
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
					img, err := decodePicture("image", s)
					if err != nil {
						return nil, err
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
		{Name: "say", Example: `{"text": "Hi!"}`,
			Prompt: "Say hi on the robot.",
			Help:   "A speech bubble over the robot's face (needs the face: show clear first if a card is up).",
			Params: []Param{{Name: "text", Type: "string", Required: true, Help: "at most 80 characters"}, {Name: "seconds", Type: "number", Help: "how long (default 6, at most 30)"}},
			run: func(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
				s, _ := in.num("seconds")
				return nil, robot.Say(r, in.str("text"), s)
			}},
		{Name: "face", Example: `{"emotion": "happy"}`,
			Prompt: "Make the robot smile.",
			Help:   "The robot's face: an emotion; or its own screen under cards: the face or its readings (its sensors live).",
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
		{Name: "move", Example: `{"gesture": "nod"}`,
			Prompt: "Make the robot nod.",
			Help:   "Move the head: a gesture (nod, shake, home = face forward), or look at an angle.",
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
		{Name: "leds", Example: `{"bar": 0.5, "color": "green"}`,
			Prompt: "Light half of the robot's LEDs in green.",
			Help:   "The 12 LEDs on the head (two strips of 6): one colour, a bar from the screen end back (a level), single LEDs, an effect the robot animates, or off. Colours are corrected to look on the LEDs as on a screen.",
			Params: []Param{{Name: "color", Type: "string", Help: "#rrggbb or a name; all LEDs, or the bar's / effect's colour"},
				{Name: "bar", Type: "number", Help: "0..1: light both strips from the screen end, this far"},
				{Name: "pixels", Type: "array", Help: "12 colours: left strip 0-5 from the screen end back, right strip 6-11 from the back to the screen end"},
				{Name: "effect", Type: "string", Enum: robot.Effects, Help: "an animation on the robot"},
				{Name: "speed", Type: "number", Help: "the effect's speed, 0.2..5 (default 1)"},
				{Name: "seconds", Type: "number", Help: "the effect's length (0 = until the next leds)"},
				{Name: "off", Type: "boolean", Help: "all off"}},
			run: runLEDs},
		{Name: "sound", Example: `{"chime": "tada"}`,
			Prompt: "Play the tada chime on the robot.",
			Help:   "A sound from the speaker: a named chime, or notes like \"C5:120 E5:120 G5:300\" (note:ms, - is a rest).",
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
		{Name: "ask", Example: `{"question": "Deploy now?", "answers": ["Yes", "Not yet"]}`,
			Prompt: "Ask me on the robot whether to deploy now.",
			Help:   "Ask the person at the robot: the question and 2-4 answers as buttons on its screen; returns the answer they tap, or timed_out.",
			Params: []Param{{Name: "question", Type: "string", Required: true, Help: "short: it is shown big"},
				{Name: "answers", Type: "array", Required: true, Help: "2-4 short answers"},
				{Name: "timeout_s", Type: "number", Help: "how long to wait (default 30, at most 120)"}},
			run: runAsk},
		{Name: "menu", Example: `{"title": "Snack?", "items": ["Apple", "Cake", "Carrot", "Milk"]}`,
			Prompt: "Ask me on the robot which snack I want: apple, cake, carrot or milk.",
			Help:   "A menu on the robot's screen: 2-6 tiles with labels, or pictures and labels, under a title; returns the tile the person taps, or timed_out.",
			Params: []Param{{Name: "title", Type: "string", Help: "small text at the top"},
				{Name: "items", Type: "array", Items: "string or object", Required: true, Help: "2-6 tiles: each a label (\"Apple\"), or {\"id\", \"label\", \"image\"} with a picture (JPEG or PNG, base64) above its label; the answer's item is the id, else the label"},
				{Name: "timeout_s", Type: "number", Help: "how long to wait (default 30, at most 120)"}},
			run: runMenu},
		{Name: "clickmap", Example: `{"regions": [{"id": "left", "rect": [0, 0, 160, 240]}, {"id": "right", "rect": [160, 0, 160, 240]}]}`,
			Prompt: "Wait until I tap the left or the right half of the robot's screen, and tell me which.",
			Help:   "Places to tap on the screen, over a picture or over what it shows: returns the first tap inside one (its id, x and y), or timed_out. The screen stays as it is.",
			Params: []Param{{Name: "image", Type: "string", Help: "a JPEG or PNG, base64, shown first (fitted to 320x240); without it the regions lie over what the screen shows now"},
				{Name: "regions", Type: "array", Items: "object", Required: true, Help: "1-32 places in the screen's pixels (320x240): {\"id\", \"rect\": [x, y, w, h]}, {\"id\", \"circle\": [x, y, r]} or {\"id\", \"polygon\": [[x, y], ...]} (3-32 points); where they overlap, the first listed counts"},
				{Name: "timeout_s", Type: "number", Help: "how long to wait (default 30, at most 120)"}},
			run: runClickmap},
		{Name: "events", Offline: true, Example: `{"after": 0, "seconds": 30}`,
			Prompt: "Wait up to 30 s for someone to touch the robot, and tell me what they did.",
			Help:   "What people did at the robot (tap, long_press, pat, pet, stroke, shaken, picked_up, near, away, tag, tag_removed, remote, button, plugged, unplugged), after a sequence number; waits up to seconds for the next one.",
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
		{Name: "command", Example: `{"command": "sticker", "args": {"name": "heart", "seconds": 5}}`,
			Prompt: "Put a heart sticker on the robot's face for five seconds.",
			Help:   "Any of the robot's own commands, as the Raw data app sends them (the q41eu-raw readme lists them), for what the other actions do not cover. Its apps and its manager's settings are not reachable here.",
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
	img, buttons := screen.Ask(in.str("question"), answers, leds.Names["blue"])
	if err := robot.Show(r, img); err != nil {
		return nil, err
	}
	if r.Has("speaker") {
		robot.Play(ctx, r, sound.Chime("ask"))
	}
	timeout, _ := in.num("timeout_s")
	t, err := a.waitTap(ctx, id, r, func(x, y int) int { return screen.Hit(buttons, x, y) }, timeout)
	switch {
	case err != nil:
		return nil, awayErr(r, err)
	case t.i == tapReplaced:
		return map[string]any{"answer": "", "replaced": true}, nil
	case t.i == tapTimedOut:
		robot.Clear(r)
		return map[string]any{"answer": "", "timed_out": true}, nil
	}
	a.chosen(ctx, id, r, answers[t.i])
	return map[string]any{"answer": answers[t.i], "index": t.i}, nil
}

// runMenu shows the tiles, waits for a tap on one, then takes the menu away.
func runMenu(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
	list, _ := in["items"].([]any)
	if len(list) < 2 || len(list) > screen.MaxMenuItems {
		return nil, fmt.Errorf("items: 2-%d tiles, each a label or {id, label, image}", screen.MaxMenuItems)
	}
	var ids []string
	var tiles []screen.MenuItem
	for i, v := range list {
		var tile screen.MenuItem
		itemID := ""
		switch it := v.(type) {
		case string:
			tile.Label = strings.TrimSpace(it)
		case map[string]any:
			for k := range it {
				if k != "id" && k != "label" && k != "image" {
					return nil, fmt.Errorf("items %d: unknown %q (id, label, image)", i, k)
				}
			}
			label, okL := it["label"].(string)
			itemID, _ = it["id"].(string)
			tile.Label = strings.TrimSpace(label)
			if s, isStr := it["image"].(string); isStr && s != "" {
				img, err := decodePicture(fmt.Sprintf("items %d: image", i), s)
				if err != nil {
					return nil, err
				}
				tile.Image = img
			}
			if (!okL && it["label"] != nil) || (it["id"] != nil && itemID == "") {
				return nil, fmt.Errorf("items %d: id and label are text", i)
			}
		default:
			return nil, fmt.Errorf("items %d: a label or {id, label, image}", i)
		}
		if tile.Label == "" && tile.Image == nil {
			return nil, fmt.Errorf("items %d: a label or a picture", i)
		}
		if itemID == "" {
			itemID = tile.Label
		}
		ids, tiles = append(ids, itemID), append(tiles, tile)
	}
	img, buttons := screen.Menu(in.str("title"), tiles, leds.Names["blue"])
	if err := robot.Show(r, img); err != nil {
		return nil, err
	}
	if r.Has("speaker") {
		robot.Play(ctx, r, sound.Chime("ask"))
	}
	timeout, _ := in.num("timeout_s")
	t, err := a.waitTap(ctx, id, r, func(x, y int) int { return screen.Hit(buttons, x, y) }, timeout)
	switch {
	case err != nil:
		return nil, awayErr(r, err)
	case t.i == tapReplaced:
		return map[string]any{"item": "", "replaced": true}, nil
	case t.i == tapTimedOut:
		robot.Clear(r)
		return map[string]any{"item": "", "timed_out": true}, nil
	}
	a.chosen(ctx, id, r, cmp.Or(tiles[t.i].Label, ids[t.i]))
	return map[string]any{"item": ids[t.i], "index": t.i, "label": tiles[t.i].Label}, nil
}

// runClickmap shows the picture (if any) and waits for a tap inside one of the regions; the
// screen stays as it is.
func runClickmap(ctx context.Context, a *App, id string, r *robothub.Robot, in args) (any, error) {
	regions, err := parseRegions(in["regions"])
	if err != nil {
		return nil, err
	}
	if s := in.str("image"); s != "" {
		img, err := decodePicture("image", s)
		if err != nil {
			return nil, err
		}
		if err := robot.Show(r, img); err != nil {
			return nil, err
		}
	}
	timeout, _ := in.num("timeout_s")
	t, err := a.waitTap(ctx, id, r, func(x, y int) int { return screen.HitRegion(regions, x, y) }, timeout)
	switch {
	case err != nil:
		return nil, err
	case t.i == tapReplaced:
		return map[string]any{"region": "", "replaced": true}, nil
	case t.i == tapTimedOut:
		return map[string]any{"region": "", "timed_out": true}, nil
	}
	return map[string]any{"region": regions[t.i].ID, "index": t.i, "x": t.x, "y": t.y}, nil
}

// MaxRegions is how many places a clickmap has at most; a polygon has at most as many points.
const MaxRegions = 32

// parseRegions reads a clickmap's regions: each an id and one shape, in the screen's pixels.
func parseRegions(v any) ([]screen.Region, error) {
	list, _ := v.([]any)
	if len(list) < 1 || len(list) > MaxRegions {
		return nil, fmt.Errorf("regions: 1-%d, each {id, rect: [x, y, w, h]}, {id, circle: [x, y, r]} or {id, polygon: [[x, y], ...]}", MaxRegions)
	}
	nums := func(v any, n int) ([]int, bool) {
		l, ok := v.([]any)
		if !ok || len(l) != n {
			return nil, false
		}
		out := make([]int, n)
		for i, x := range l {
			f, isNum := x.(float64)
			if !isNum {
				return nil, false
			}
			out[i] = int(math.Round(f))
		}
		return out, true
	}
	var out []screen.Region
	seen := map[string]bool{}
	for i, item := range list {
		m, ok := item.(map[string]any)
		id, _ := m["id"].(string)
		if !ok || id == "" || seen[id] {
			return nil, fmt.Errorf("regions %d: an object with an id of its own", i)
		}
		seen[id] = true
		g, shapes := screen.Region{ID: id}, 0
		for k, v := range m {
			switch k {
			case "id":
			case "rect":
				n, ok := nums(v, 4)
				if !ok || n[2] <= 0 || n[3] <= 0 {
					return nil, fmt.Errorf("regions %s: rect is [x, y, w, h], w and h above 0", id)
				}
				g.Rect, shapes = image.Rect(n[0], n[1], n[0]+n[2], n[1]+n[3]), shapes+1
			case "circle":
				n, ok := nums(v, 3)
				if !ok || n[2] <= 0 {
					return nil, fmt.Errorf("regions %s: circle is [x, y, r], r above 0", id)
				}
				g.Center, g.R, shapes = image.Pt(n[0], n[1]), n[2], shapes+1
			case "polygon":
				l, _ := v.([]any)
				if len(l) < 3 || len(l) > MaxRegions {
					return nil, fmt.Errorf("regions %s: polygon is 3-%d points [x, y]", id, MaxRegions)
				}
				for _, p := range l {
					n, ok := nums(p, 2)
					if !ok {
						return nil, fmt.Errorf("regions %s: polygon is 3-%d points [x, y]", id, MaxRegions)
					}
					g.Polygon = append(g.Polygon, image.Pt(n[0], n[1]))
				}
				shapes++
			default:
				return nil, fmt.Errorf("regions %s: unknown %q (id, and rect, circle or polygon)", id, k)
			}
		}
		if shapes != 1 {
			return nil, fmt.Errorf("regions %s: one shape: rect, circle or polygon", id)
		}
		out = append(out, g)
	}
	return out, nil
}

// What waitTap answers instead of a place.
const (
	tapReplaced = -1 // a newer ask, menu or clickmap took the screen
	tapTimedOut = -2
)

// waitTap makes hit the robot's question (an older one answers replaced) and waits for a tap on
// one of its places, up to timeout seconds (default 30, at most 120).
func (a *App) waitTap(ctx context.Context, id string, r *robothub.Robot, hit func(x, y int) int, timeout float64) (tapped, error) {
	if timeout <= 0 {
		timeout = 30
	}
	q := &question{hit: hit, answer: make(chan tapped, 1)}
	a.mu.Lock()
	rs := a.stateFor(id)
	if old := rs.ask; old != nil {
		select {
		case old.answer <- tapped{i: tapReplaced}:
		default: // it has its answer already
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
	case t := <-q.answer:
		return t, nil
	case <-time.After(time.Duration(min(120, timeout) * float64(time.Second))):
		return tapped{i: tapTimedOut}, nil
	case <-ctx.Done():
		return tapped{}, ctx.Err()
	case <-r.Done():
		return tapped{}, robot.ErrGone
	}
}

// awayErr ends an ask or a menu whose call went away (not the robot): the face again.
func awayErr(r *robothub.Robot, err error) error {
	if !errors.Is(err, robot.ErrGone) {
		robot.Clear(r)
	}
	return err
}

// chosen confirms a choice on the robot: a green card with it and a tick, a chime, then the face
// (unless another action came meanwhile: the program's next card stays).
func (a *App) chosen(ctx context.Context, id string, r *robothub.Robot, label string) {
	robot.ShowCard(r, screen.Card{Text: label, Color: leds.Names["green"], Note: "✓"})
	if r.Has("speaker") {
		robot.Play(ctx, r, sound.Chime("yes"))
	}
	a.mu.Lock()
	acts := a.stateFor(id).acts
	a.mu.Unlock()
	time.AfterFunc(1200*time.Millisecond, func() {
		a.mu.Lock()
		still := a.stateFor(id).acts == acts
		a.mu.Unlock()
		if still {
			robot.Clear(r)
		}
	})
}

// maxPicturePixels: a picture sent bigger than this is refused before it is decoded (a small
// file can unpack into a huge picture).
const maxPicturePixels = 4096 * 4096

// decodePicture reads a JPEG or PNG sent as base64 (what names the argument in errors).
func decodePicture(what, s string) (image.Image, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%s: not base64: %w", what, err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	if cfg.Width*cfg.Height > maxPicturePixels {
		return nil, fmt.Errorf("%s: %dx%d is too big (at most 16 megapixels; 320x240 is shown)", what, cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return img, nil
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
		a.mu.Lock()
		a.stateFor(id).acts++
		a.mu.Unlock()
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

// devEvents are the events Dev adds to what people did (gesture.Names).
var devEvents = []struct{ Name, Means string }{
	{"connected", "the robot connected to Dev: at its start, after a restart, or back after it was away"},
}

// allEvents are every event a program gets, with what it means: what people did, then Dev's own.
func allEvents() []struct{ Name, Means string } {
	return append(slices.Clone(gesture.Names), devEvents...)
}

// eventNames for the docs (GET /v1/actions).
func eventNames() []string {
	var out []string
	for _, n := range allEvents() {
		out = append(out, n.Name)
	}
	return out
}
