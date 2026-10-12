// Package robothub is the robot side of an Embody Mode app: robots connect to it over the device
// wire protocol (github.com/i41-eu/q41eu-raw/wire), it checks their tokens, keeps one
// connection per robot, gives each its pairing code, and hands the app what the robot sends.
// Every app (raw, pet, focus, sbot) had its own copy of this; new apps use the hub.
//
// The hub keeps no app state: the app keeps its robots, browsers and pairings, and the hub asks
// it through Config's callbacks. Callbacks run on the robot's own goroutine, without the hub's
// lock held, so an app may lock its own mutex in them and call the hub back.
package robothub

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/i41-eu/q41eu-raw/robotauth"
	"github.com/i41-eu/q41eu-raw/wire"
)

// Liveness timing, as in q41eu-raw.
const (
	pingPeriod      = 5 * time.Second
	pongWait        = 60 * time.Second
	writeWait       = 10 * time.Second
	registerTimeout = 10 * time.Second
	maxMessageBytes = 256 << 10
)

// QueueSize is how many messages wait for a robot's socket at most; more are dropped.
const QueueSize = 64

var robotIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true }, // robots use a bearer token, not cookies
}

// Config sets a hub up. Token or Manager (or both) let robots in.
type Config struct {
	Token     string            // the shared bearer token of the owner's own robots; "" = none
	Manager   *robotauth.Client // robots set up by a Stackchan manager, each with a token of its own; nil = none
	View      string            // the robot's own screen this app starts with (Accepted {view}: "face", "readings"); "" = its default
	PublicURL string            // base URL browsers use: pairing links are PublicURL + "/pair?code=…"
	PairTTL   time.Duration     // lifetime of a pairing code; default 5 minutes
	Log       *slog.Logger

	// Connected: a robot is registered (the app may send commands now). It replaced any older
	// connection of the same robot.
	Connected func(r *Robot)
	// Disconnected: its connection ended (not called for a connection replaced by a newer one).
	Disconnected func(r *Robot)
	Event        func(r *Robot, ev wire.RobotEventBody)
	Telemetry    func(r *Robot, measurements map[string]float64)
	Binary       func(r *Robot, kind byte, payload []byte) // camera, microphone, sensor streams
	// Paired: what to tell a robot about its browsers right after it connects (wire.Paired:
	// a robot with paired browsers shows the app, not its QR screen). nil, or Viewers 0: nothing.
	Paired func(robotID string) wire.PairedBody
	// Owner: the robot's owner as its manager names it (an account key), "" for the shared token.
	Owner func(robotID, owner string)
}

// Hub accepts robots. Mount it at wire.ConnectPath (Hub.ServeHTTP answers GET there).
type Hub struct {
	cfg    Config
	log    *slog.Logger
	mu     sync.Mutex
	robots map[string]*Robot // connected now
	codes  map[string]pairCode
}

type pairCode struct {
	robotID string
	expires time.Time
}

// New makes a hub.
func New(cfg Config) *Hub {
	if cfg.PairTTL <= 0 {
		cfg.PairTTL = 5 * time.Minute
	}
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return &Hub{cfg: cfg, log: cfg.Log, robots: map[string]*Robot{}, codes: map[string]pairCode{}}
}

// Robot is one robot's connection. Its writer goroutine owns all writes; sends only queue.
type Robot struct {
	id       string
	caps     wire.RobotCapabilities
	ws       *websocket.Conn
	send     chan outMsg
	done     chan struct{}
	once     sync.Once
	mgrToken string // a token from the manager: the robot's browsers are reported there
	since    time.Time

	mu        sync.Mutex
	telemetry map[string]float64
}

type outMsg struct {
	binary bool
	data   []byte
}

// ID is the robot's id (stackchan-<mac>).
func (r *Robot) ID() string { return r.id }

// Capabilities is what the robot registered with: model, firmware, commands, measurements.
func (r *Robot) Capabilities() wire.RobotCapabilities { return r.caps }

// Has says whether the robot lists the command.
func (r *Robot) Has(command string) bool { return slices.Contains(r.caps.Commands, command) }

// Since is when it connected.
func (r *Robot) Since() time.Time { return r.since }

// Done is closed when the connection ends.
func (r *Robot) Done() <-chan struct{} { return r.done }

// Managed says whether the robot came with a token from its manager.
func (r *Robot) Managed() bool { return r.mgrToken != "" }

// Telemetry is a copy of the robot's latest measurements (sent every 2 s).
func (r *Robot) Telemetry() map[string]float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]float64, len(r.telemetry))
	for k, v := range r.telemetry {
		out[k] = v
	}
	return out
}

// Command queues a robot command (wire.RobotCommandBody). False: the robot is gone or its queue
// is full (the command is dropped).
func (r *Robot) Command(name string, args map[string]any) bool {
	return r.Frame(wire.KindRobotCommand, wire.RobotCommandBody{Command: name, Args: args})
}

// Binary queues a binary message: kind (wire.BinShowJPEG, wire.BinSpeakerPCM, …), then payload.
func (r *Robot) Binary(kind byte, payload []byte) bool {
	return r.queue(outMsg{binary: true, data: append([]byte{kind}, payload...)})
}

// Frame queues any frame.
func (r *Robot) Frame(kind string, body any) bool {
	f, err := wire.Marshal(kind, wire.Meta{WorkerID: r.id}, body)
	return err == nil && r.queue(outMsg{data: f})
}

// Queued is how many messages wait to be written now (of QueueSize): bulk senders (file
// uploads) pace themselves by it so commands still get through.
func (r *Robot) Queued() int { return len(r.send) }

// Close ends the connection.
func (r *Robot) Close() {
	r.once.Do(func() {
		close(r.done)
		r.ws.Close()
	})
}

func (r *Robot) queue(m outMsg) bool {
	select {
	case <-r.done:
		return false
	case r.send <- m:
		return true
	default:
		return false
	}
}

// Robot is the robot's connection now, or nil while it is offline.
func (h *Hub) Robot(id string) *Robot {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.robots[id]
}

// Robots are the connected robots, by id.
func (h *Hub) Robots() []*Robot {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*Robot, 0, len(h.robots))
	for _, r := range h.robots {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b *Robot) int { return strings.Compare(a.id, b.id) })
	return out
}

// Redeem takes a pairing code a browser brought (from the robot's QR) and gives its robot. A code
// works once, and only while its robot is connected and the code is fresh.
func (h *Hub) Redeem(code string) (robotID string, ok bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	h.mu.Lock()
	defer h.mu.Unlock()
	pc, found := h.codes[code]
	if !found || time.Now().After(pc.expires) || h.robots[pc.robotID] == nil {
		return "", false
	}
	delete(h.codes, code)
	if r := h.robots[pc.robotID]; r != nil { // a fresh code for the next browser
		r.Frame(wire.KindPairCode, h.issueLocked(pc.robotID))
	}
	return pc.robotID, true
}

// TellPaired sends the robot its browsers now (Config.Paired), e.g. after a browser paired: the
// robot leaves its QR screen for the app.
func (h *Hub) TellPaired(robotID string) {
	r := h.Robot(robotID)
	if r == nil || h.cfg.Paired == nil {
		return
	}
	body := h.cfg.Paired(robotID)
	if body.Viewers > 0 {
		r.Frame(wire.KindPaired, body)
	}
}

// Report tells the robot's manager the browsers paired with it (every 15 s or so, and after a
// change); the answer names the browsers its owner removed there. Robots without a manager
// token: nothing to do.
func (h *Hub) Report(ctx context.Context, robotID string, pairings []robotauth.Pairing) (unpair []string, err error) {
	r := h.Robot(robotID)
	if r == nil || r.mgrToken == "" || h.cfg.Manager == nil {
		return nil, nil
	}
	auth, err := h.cfg.Manager.Seen(ctx, robotID, r.mgrToken, robotauth.Seen{Pairings: pairings})
	if err != nil {
		return nil, err
	}
	if !auth.OK {
		return nil, errors.New("the manager no longer knows this robot")
	}
	if h.cfg.Owner != nil && auth.Owner != "" {
		h.cfg.Owner(robotID, auth.Owner)
	}
	return auth.Unpair, nil
}

const codeAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

func newCode() string {
	b := make([]byte, 8)
	rand.Read(b)
	for i := range b {
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(b)
}

// issueLocked replaces the robot's pairing code. h.mu held.
func (h *Hub) issueLocked(robotID string) wire.PairCodeBody {
	for c, pc := range h.codes {
		if pc.robotID == robotID {
			delete(h.codes, c)
		}
	}
	code := newCode()
	h.codes[code] = pairCode{robotID: robotID, expires: time.Now().Add(h.cfg.PairTTL)}
	return wire.PairCodeBody{Code: code, URL: h.cfg.PublicURL + "/pair?code=" + code, ExpiresInS: int(h.cfg.PairTTL / time.Second)}
}

// tokenOK: the shared token, compared in constant time.
func (h *Hub) tokenOK(token string) bool {
	return h.cfg.Token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(h.cfg.Token)) == 1
}

// ServeHTTP is the robots' endpoint (GET wire.ConnectPath).
func (h *Hub) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	token, _ := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	id := wire.DeviceID(req.Header)
	if !robotIDPattern.MatchString(id) {
		http.Error(w, "missing or invalid "+wire.DeviceIDHeader, http.StatusBadRequest)
		return
	}
	mgrToken, owner := "", ""
	switch {
	case h.tokenOK(token):
	case h.cfg.Manager != nil && token != "":
		auth, err := h.cfg.Manager.Check(req.Context(), id, token)
		if err != nil || !auth.OK {
			h.log.Warn("robot refused", "robot", id, "err", err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mgrToken, owner = token, auth.Owner
	default:
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ws, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(maxMessageBytes)
	r := &Robot{id: id, ws: ws, send: make(chan outMsg, QueueSize), done: make(chan struct{}), mgrToken: mgrToken,
		since: time.Now(), telemetry: map[string]float64{}}
	defer r.Close()
	reg, reason := readRegister(ws, id)
	if reason != "" {
		h.log.Warn("robot rejected", "robot", id, "reason", reason)
		if f, err := wire.Marshal(wire.KindRejected, wire.Meta{}, wire.RejectedBody{Reason: reason}); err == nil {
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			ws.WriteMessage(websocket.TextMessage, f)
		}
		return
	}
	r.caps = reg.Capabilities
	if h.cfg.Owner != nil {
		h.cfg.Owner(id, owner)
	}

	h.mu.Lock()
	old := h.robots[id]
	h.robots[id] = r
	// Accepted names the app's own screen on the robot; the view field is wire protocol §4.
	r.Frame(wire.KindAccepted, struct {
		View string `json:"view,omitempty"`
	}{h.cfg.View})
	r.Frame(wire.KindPairCode, h.issueLocked(id))
	h.mu.Unlock()
	if old != nil {
		old.Close()
	}
	h.log.Info("robot connected", "robot", id, "firmware", reg.Capabilities.Firmware, "managed", mgrToken != "")
	if h.cfg.Paired != nil {
		if body := h.cfg.Paired(id); body.Viewers > 0 {
			body.Reconnect = true
			r.Frame(wire.KindPaired, body)
		}
	}
	if r.Has("sprite") {
		r.Command("sprite_clear", nil) // sprites stay on the robot across apps
	}
	if h.cfg.Connected != nil {
		h.cfg.Connected(r)
	}
	go h.writeLoop(r)
	h.readLoop(r)

	h.mu.Lock()
	current := h.robots[id] == r
	if current {
		delete(h.robots, id)
		for c, pc := range h.codes {
			if pc.robotID == id {
				delete(h.codes, c)
			}
		}
	}
	h.mu.Unlock()
	h.log.Info("robot disconnected", "robot", id)
	if current && h.cfg.Disconnected != nil {
		h.cfg.Disconnected(r)
	}
}

func readRegister(ws *websocket.Conn, id string) (wire.RegisterBody, string) {
	var reg wire.RegisterBody
	ws.SetReadDeadline(time.Now().Add(registerTimeout))
	_, data, err := ws.ReadMessage()
	if err != nil {
		return reg, "no Register frame"
	}
	frames, err := wire.Parse(data)
	if err != nil || len(frames) == 0 || frames[0].Kind != wire.KindRegister {
		return reg, "first frame must be Register"
	}
	f := frames[0]
	if f.Meta.WorkerID != "" && f.Meta.WorkerID != id {
		return reg, "meta.worker_id does not match " + wire.DeviceIDHeader
	}
	if err := f.Decode(&reg); err != nil {
		return reg, "invalid Register body"
	}
	if reg.Class != wire.ClassRobot {
		return reg, "unknown worker class"
	}
	return reg, ""
}

// writeLoop owns all writes: queued messages, pings, a fresh pairing code every PairTTL.
func (h *Hub) writeLoop(r *Robot) {
	ping := time.NewTicker(pingPeriod)
	defer ping.Stop()
	rotate := time.NewTicker(h.cfg.PairTTL)
	defer rotate.Stop()
	for {
		select {
		case <-r.done:
			return
		case m := <-r.send:
			kind := websocket.TextMessage
			if m.binary {
				kind = websocket.BinaryMessage
			}
			r.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := r.ws.WriteMessage(kind, m.data); err != nil {
				r.Close()
				return
			}
		case <-ping.C:
			if err := r.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				r.Close()
				return
			}
		case <-rotate.C:
			h.mu.Lock()
			body := h.issueLocked(r.id)
			h.mu.Unlock()
			r.Frame(wire.KindPairCode, body)
		}
	}
}

func (h *Hub) readLoop(r *Robot) {
	reset := func() { r.ws.SetReadDeadline(time.Now().Add(pongWait)) }
	reset()
	r.ws.SetPongHandler(func(string) error { reset(); return nil })
	r.ws.SetPingHandler(func(data string) error {
		reset()
		return r.ws.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(writeWait))
	})
	for {
		kind, data, err := r.ws.ReadMessage()
		if err != nil {
			return
		}
		reset()
		if kind == websocket.BinaryMessage {
			if len(data) > 0 && h.cfg.Binary != nil {
				h.cfg.Binary(r, data[0], data[1:])
			}
			continue
		}
		frames, err := wire.Parse(data)
		if err != nil {
			continue
		}
		for _, f := range frames {
			switch f.Kind {
			case wire.KindRobotEvent:
				var ev wire.RobotEventBody
				if f.Decode(&ev) == nil && ev.Name != "" && h.cfg.Event != nil {
					h.cfg.Event(r, ev)
				}
			case wire.KindRobotTelemetry:
				var t wire.RobotTelemetryBody
				if f.Decode(&t) != nil {
					continue
				}
				r.mu.Lock()
				for k, v := range t.Measurements {
					r.telemetry[k] = v
				}
				r.mu.Unlock()
				if h.cfg.Telemetry != nil {
					h.cfg.Telemetry(r, t.Measurements)
				}
			}
		}
	}
}
