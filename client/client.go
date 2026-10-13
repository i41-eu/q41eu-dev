// Package client calls Dev's API (/v1) from Go: a Stackchan robot for your program, with a key
// from Dev's page. FromEnv reads SDEV_URL, SDEV_ROBOT and SDEV_KEY, as the examples do.
//
//	c, err := client.FromEnv()
//	err = c.Show(ctx, client.Card{Title: "Build", Text: "All tests pass", Color: "green"})
//	answer, err := c.Ask(ctx, "Deploy now?", "Yes", "Not yet")
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client is one robot through one key.
type Client struct {
	URL   string // Dev's address, e.g. https://dev.q41.eu
	Robot string // the robot's id, e.g. stackchan-0a1b2c3d4e50
	Key   string // sdev_…, from Dev's page
	HTTP  *http.Client
}

// FromEnv makes a client from SDEV_URL, SDEV_ROBOT and SDEV_KEY. Without SDEV_ROBOT it takes the
// key's robot (a key has one).
func FromEnv() (*Client, error) {
	c := &Client{URL: strings.TrimRight(os.Getenv("SDEV_URL"), "/"), Robot: os.Getenv("SDEV_ROBOT"), Key: os.Getenv("SDEV_KEY"),
		HTTP: &http.Client{Timeout: 3 * time.Minute}}
	if c.URL == "" || c.Key == "" {
		return nil, errors.New("set SDEV_URL and SDEV_KEY (and SDEV_ROBOT): Dev's page shows them with a new key")
	}
	if c.Robot == "" {
		var out struct {
			Robots []struct {
				ID string `json:"id"`
			} `json:"robots"`
		}
		if err := c.get(context.Background(), "/v1/robots", &out); err != nil {
			return nil, err
		}
		if len(out.Robots) == 0 {
			return nil, errors.New("this key has no robot")
		}
		c.Robot = out.Robots[0].ID
	}
	return c, nil
}

// Error is what Dev answered when an action did not happen.
type Error struct {
	Status  int    // 400 a wrong argument, 401 no key, 404 not your robot, 429 too many, 501 not on this robot, 503 offline
	Message string // Dev's words
}

func (e *Error) Error() string { return fmt.Sprintf("dev: %d %s", e.Status, e.Message) }

// Do runs an action (POST /v1/robots/<robot>/<action>) with args (any JSON object, or nil) and
// decodes the answer into out (or nil).
func (c *Client) Do(ctx context.Context, action string, args, out any) error {
	if args == nil {
		args = struct{}{}
	}
	b, err := json.Marshal(args)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/v1/robots/"+url.PathEscape(c.Robot)+"/"+action, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.send(req, out)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+path, nil)
	if err != nil {
		return err
	}
	return c.send(req, out)
}

func (c *Client) send(req *http.Request, out any) error {
	req.Header.Set("Authorization", "Bearer "+c.Key)
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		json.Unmarshal(body, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(body))
		}
		return &Error{Status: res.StatusCode, Message: e.Error}
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// Card is what Show puts on the screen. Color (the text's) and Background are #rrggbb or a name
// (red, orange, yellow, green, cyan, blue, purple, pink, white); Progress 0..1 draws a bar.
type Card struct {
	Title      string   `json:"title,omitempty"`
	Text       string   `json:"text,omitempty"`
	Note       string   `json:"note,omitempty"`
	Color      string   `json:"color,omitempty"`      // the text's (and title's, bar's)
	Background string   `json:"background,omitempty"` // the card's
	Progress   *float64 `json:"progress,omitempty"`
}

// Progress is a pointer for Card.Progress.
func Progress(f float64) *float64 { return &f }

// Show puts a card on the robot's screen.
func (c *Client) Show(ctx context.Context, card Card) error { return c.Do(ctx, "show", card, nil) }

// Clear takes the card away: the robot's face (or its readings) again.
func (c *Client) Clear(ctx context.Context) error {
	return c.Do(ctx, "show", map[string]any{"clear": true}, nil)
}

// Say shows a speech bubble over the face (Clear a card first).
func (c *Client) Say(ctx context.Context, text string) error {
	return c.Do(ctx, "say", map[string]any{"text": text}, nil)
}

// Face sets an emotion: neutral, happy, angry, sad, doubt, sleepy.
func (c *Client) Face(ctx context.Context, emotion string) error {
	return c.Do(ctx, "face", map[string]any{"emotion": emotion}, nil)
}

// Gesture moves the head: nod, shake, or home (face forward).
func (c *Client) Gesture(ctx context.Context, name string) error {
	return c.Do(ctx, "move", map[string]any{"gesture": name}, nil)
}

// Look turns the head: yaw -128..128 degrees (+ to the robot's left), pitch 5..85 (up).
func (c *Client) Look(ctx context.Context, yaw, pitch float64) error {
	return c.Do(ctx, "move", map[string]any{"yaw": yaw, "pitch": pitch}, nil)
}

// LEDs lights all 12 LEDs in one colour ("off" for off).
func (c *Client) LEDs(ctx context.Context, color string) error {
	if color == "off" {
		return c.Do(ctx, "leds", map[string]any{"off": true}, nil)
	}
	return c.Do(ctx, "leds", map[string]any{"color": color}, nil)
}

// LEDBar lights both strips from the screen end back, as far as fraction (0..1): a level.
func (c *Client) LEDBar(ctx context.Context, fraction float64, color string) error {
	return c.Do(ctx, "leds", map[string]any{"bar": fraction, "color": color}, nil)
}

// Chime plays a named sound (tada, chirp, done, start, hello, no, yes, ask, error, …).
func (c *Client) Chime(ctx context.Context, name string) error {
	return c.Do(ctx, "sound", map[string]any{"chime": name}, nil)
}

// Notes plays a melody: "C5:120 E5:120 G5:300" (note:ms, - a rest).
func (c *Client) Notes(ctx context.Context, notes string) error {
	return c.Do(ctx, "sound", map[string]any{"notes": notes}, nil)
}

// Ask shows a question with 2-4 answers as buttons and waits (up to 30 s) for a tap; "" when
// nobody answered.
func (c *Client) Ask(ctx context.Context, question string, answers ...string) (string, error) {
	var out struct {
		Answer string `json:"answer"`
	}
	err := c.Do(ctx, "ask", map[string]any{"question": question, "answers": answers}, &out)
	return out.Answer, err
}

// MenuItem is a tile of a menu: a label, and a picture above it (a JPEG or PNG, nil: none); the
// answer names the tile by its ID, else by its label.
type MenuItem struct {
	ID    string `json:"id,omitempty"`
	Label string `json:"label,omitempty"`
	Image []byte `json:"image,omitempty"` // sent as base64
}

// Menu shows 2-6 tiles under a title and waits (up to 30 s) for a tap on one; it gives the tile's
// ID (else its label), or "" when nobody chose.
func (c *Client) Menu(ctx context.Context, title string, items ...MenuItem) (string, error) {
	var out struct {
		Item string `json:"item"`
	}
	err := c.Do(ctx, "menu", map[string]any{"title": title, "items": items}, &out)
	return out.Item, err
}

// Region is a place to tap on the screen (320x240 pixels): a rectangle, a circle or a polygon.
type Region struct {
	ID      string   `json:"id"`
	Rect    []int    `json:"rect,omitempty"`    // x, y, w, h
	Circle  []int    `json:"circle,omitempty"`  // x, y, r
	Polygon [][2]int `json:"polygon,omitempty"` // 3 points or more
}

// Tap is where a person tapped in a clickmap: its region's ID ("" when nobody did) and the point.
type Tap struct {
	Region string `json:"region"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
}

// Clickmap shows picture (a JPEG or PNG; nil: what the screen shows now stays) and waits (up to
// 30 s) for a tap inside one of the regions; the screen stays as it is.
func (c *Client) Clickmap(ctx context.Context, picture []byte, regions ...Region) (Tap, error) {
	args := map[string]any{"regions": regions}
	if picture != nil {
		args["image"] = picture // []byte is base64 in JSON
	}
	var out Tap
	err := c.Do(ctx, "clickmap", args, &out)
	return out, err
}

// Event is something a person did at the robot: tap, long_press, pat, pet, stroke, shaken,
// picked_up, near, away, tag, tag_removed, remote, button, plugged, unplugged (and connected).
type Event struct {
	Seq  int64          `json:"seq"`
	Name string         `json:"name"`
	Data map[string]any `json:"data,omitempty"`
	At   time.Time      `json:"at"`
}

// Events are the events after seq (0: the last ones kept), waiting up to wait for the next one;
// with names, only those. It returns the last seq to ask after next time.
func (c *Client) Events(ctx context.Context, after int64, wait time.Duration, names ...string) ([]Event, int64, error) {
	q := url.Values{"after": {fmt.Sprint(after)}, "seconds": {fmt.Sprint(int(wait.Seconds()))}}
	if len(names) > 0 {
		q.Set("names", strings.Join(names, ","))
	}
	var out struct {
		Events  []Event `json:"events"`
		LastSeq int64   `json:"last_seq"`
	}
	err := c.get(ctx, "/v1/robots/"+url.PathEscape(c.Robot)+"/events?"+q.Encode(), &out)
	return out.Events, out.LastSeq, err
}

// State is the robot now: online, model, firmware, its readings (units in their names) and its
// last events.
type State struct {
	Online     bool               `json:"online"`
	Model      string             `json:"model"`
	Firmware   string             `json:"firmware"`
	Readings   map[string]float64 `json:"readings"`
	LastEvents []Event            `json:"last_events"`
}

// State reads the robot's state.
func (c *Client) State(ctx context.Context) (State, error) {
	var s State
	err := c.get(ctx, "/v1/robots/"+url.PathEscape(c.Robot), &s)
	return s, err
}
