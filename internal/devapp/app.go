// Package devapp is the dev app: an Embody Mode app like the others (robots connect to it, a
// browser pairs by the robot's QR code), whose page gives the person keys for the higher-level
// API: the same actions over HTTP (/v1) for scripts in any language, and as MCP tools (/mcp) for
// AI agents. Keys belong to one robot each; only a browser paired with the robot makes them.
package devapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/i41-eu/q41eu-dev/gesture"
	"github.com/i41-eu/q41eu-dev/robot"
	"github.com/i41-eu/q41eu-dev/robothub"
	"github.com/i41-eu/q41eu-dev/screen"
	"github.com/i41-eu/q41eu-raw/robotauth"
	"github.com/i41-eu/q41eu-raw/sso"
	"github.com/i41-eu/q41eu-raw/statestore"
	"github.com/i41-eu/q41eu-raw/wire"
)

// Config sets the app up.
type Config struct {
	RobotToken   string            // the shared token of the owner's own robots; "" = none
	Manager      *robotauth.Client // robots set up by a Stackchan manager; nil = none
	SignIn       *sso.Client       // sign-in through that manager: the owner's browsers get their robots without the QR code
	SignInAlways bool              // try a silent sign-in without the manager's hint cookie (sso.SilentTry): this app on another domain than the manager's -sign-in-hint-domain
	ManagerURL   string            // the manager's page, linked from Dev's ("" = none)
	PublicURL    string            // base URL browsers and API clients use
	State        statestore.Store  // pairings and keys; nil = in memory only
	UIDir        string            // development: render the site from this directory (internal/devapp/site) on every request
	// TrustedProxies: reverse proxies in front that append the client's address to
	// X-Forwarded-For (limits per address use it); 0 trusts no header.
	TrustedProxies int
	Log            *slog.Logger
	Version        string
}

// App holds everything; one mutex guards it (robot sockets only queue messages).
type App struct {
	cfg Config
	log *slog.Logger
	hub *robothub.Hub

	mu     sync.Mutex
	st     state
	dirty  bool
	robots map[string]*robotState
	subs   map[*subscriber]struct{}

	actions   *buckets       // per key or browser
	pairFails *buckets       // wrong pairing codes per address
	streams   map[string]int // open page streams per address (a.mu)

	site    *site // the pages, rendered at start (site.go)
	siteErr error
}

// state is what is saved.
type state struct {
	Sessions map[string]*session `json:"sessions"` // browser cookie -> its robots
	Keys     map[string]*apiKey  `json:"keys"`     // sha256 of the key (hex) -> the key
	Owners   map[string]string   `json:"owners"`   // robot id -> its owner's account key (from the manager)
}

type session struct {
	Robots   []string     `json:"robots"`
	Device   string       `json:"device,omitempty"`
	Since    time.Time    `json:"since"`
	LastSeen time.Time    `json:"last_seen"`
	Account  *sso.Account `json:"account,omitempty"` // signed in through the manager (signin.go)
	Handle   string       `json:"handle,omitempty"`
}

type apiKey struct {
	ID       string    `json:"id"` // the first 8 hex characters of its hash: shown, never secret
	Robot    string    `json:"robot"`
	Name     string    `json:"name"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used,omitzero"`
}

// robotState is what the app knows of a robot while it runs (not saved).
type robotState struct {
	rec    gesture.Recognizer
	events []seqEvent // the last maxEvents
	seq    int64
	wake   chan struct{} // closed and replaced on every new event
	ask    *question
	acts   int64 // actions run on it (not state or events): a delayed step of an older one is dropped
}

type seqEvent struct {
	Seq int64 `json:"seq"`
	gesture.Event
}

const maxEvents = 100

// question is an ask, a menu or a clickmap waiting for a tap on one of its places.
type question struct {
	hit    func(x, y int) int // the place at (x, y), or -1
	answer chan tapped
}

// tapped is the place tapped (i) and where; i < 0: a newer question took the screen.
type tapped struct{ i, x, y int }

// New makes the app and loads its state.
func New(cfg Config) *App {
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	a := &App{cfg: cfg, log: cfg.Log, robots: map[string]*robotState{}, subs: map[*subscriber]struct{}{},
		actions: newBuckets(actionsPerSecond, actionsBurst), pairFails: newBuckets(pairFailsPer10m/600.0, pairFailsPer10m),
		streams: map[string]int{},
		st:      state{Sessions: map[string]*session{}, Keys: map[string]*apiKey{}, Owners: map[string]string{}}}
	a.hub = robothub.New(robothub.Config{
		Token: cfg.RobotToken, Manager: cfg.Manager, PublicURL: cfg.PublicURL, Log: cfg.Log,
		Connected:    a.connected,
		Disconnected: func(r *robothub.Robot) { a.publish("robot", a.robotInfo(r.ID())) },
		Event:        a.robotEvent,
		Paired:       a.paired,
		Owner:        a.setOwner,
	})
	if err := a.load(); err != nil {
		a.log.Error("state not loaded: starting empty", "err", err)
	}
	files, _ := fs.Sub(siteFiles, "site")
	if a.site, a.siteErr = buildSite(files); a.siteErr != nil {
		a.log.Error("the site's pages", "err", a.siteErr)
	}
	return a
}

func (a *App) load() error {
	if a.cfg.State == nil {
		return nil
	}
	b, err := a.cfg.State.Load(context.Background())
	if err != nil || b == nil {
		return err
	}
	var st state
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	if st.Sessions == nil {
		st.Sessions = map[string]*session{}
	}
	if st.Keys == nil {
		st.Keys = map[string]*apiKey{}
	}
	if st.Owners == nil {
		st.Owners = map[string]string{}
	}
	a.st = st
	return nil
}

// Save writes the state when it changed.
func (a *App) Save(ctx context.Context) error {
	a.mu.Lock()
	if !a.dirty || a.cfg.State == nil {
		a.mu.Unlock()
		return nil
	}
	b, err := json.Marshal(a.st)
	a.dirty = false
	a.mu.Unlock()
	if err != nil {
		return err
	}
	return a.cfg.State.Save(ctx, b)
}

// Run saves the state and reports pairings to the robots' managers until ctx ends.
func (a *App) Run(ctx context.Context) {
	save := time.NewTicker(5 * time.Second)
	defer save.Stop()
	report := time.NewTicker(15 * time.Second)
	defer report.Stop()
	go a.runSignInCheck(ctx)
	for {
		select {
		case <-ctx.Done():
			a.Save(context.Background())
			return
		case <-save.C:
			if err := a.Save(ctx); err != nil {
				a.log.Warn("state not saved", "err", err)
			}
		case <-report.C:
			for _, r := range a.hub.Robots() {
				a.report(ctx, r.ID())
			}
		}
	}
}

// report tells the robot's manager its paired browsers and drops those its owner removed there.
func (a *App) report(ctx context.Context, robotID string) {
	a.mu.Lock()
	var pairings []robotauth.Pairing
	for sid, s := range a.st.Sessions {
		if slices.Contains(s.Robots, robotID) {
			pairings = append(pairings, robotauth.Pairing{ID: pairingID(sid), Device: s.Device, Paired: s.Since, LastSeen: s.LastSeen,
				Watching: a.watchingLocked(sid)})
		}
	}
	for _, k := range a.st.Keys { // keys too: the owner sees them on the manager and may remove them
		if k.Robot == robotID {
			pairings = append(pairings, robotauth.Pairing{ID: "key-" + k.ID, Device: "API key: " + k.Name, Paired: k.Created, LastSeen: k.LastUsed})
		}
	}
	a.mu.Unlock()
	unpair, err := a.hub.Report(ctx, robotID, pairings)
	if err != nil || len(unpair) == 0 {
		return
	}
	a.mu.Lock()
	for sid, s := range a.st.Sessions {
		if slices.Contains(unpair, pairingID(sid)) {
			s.Robots = slices.DeleteFunc(s.Robots, func(id string) bool { return id == robotID })
			a.dirty = true
		}
	}
	for h, k := range a.st.Keys {
		if k.Robot == robotID && slices.Contains(unpair, "key-"+k.ID) {
			delete(a.st.Keys, h)
			a.dirty = true
			a.log.Info("key removed on the manager", "robot", robotID, "key", k.ID)
		}
	}
	a.mu.Unlock()
}

// pairingID is a browser's id for the manager: stable, not its cookie.
func pairingID(sid string) string {
	h := sha256.Sum256([]byte("pairing:" + sid))
	return hex.EncodeToString(h[:8])
}

// Handler is everything: the robots' endpoint, the site, the API, MCP.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET "+wire.ConnectPath, a.hub)
	for _, p := range sitePages {
		pattern, page := "GET "+p.Path, a.handlePage
		switch p.Path {
		case "/robots": // the person's robots: a browser signed in at the manager gets them silently
			page = a.trySignIn(page, true)
		case "/": // home shows them too, but goes to the manager only for a browser its hint cookie names
			pattern, page = "GET /{$}", a.trySignIn(page, false)
		} // the docs pages answer everyone at once, as they are
		mux.HandleFunc(pattern, page)
	}
	mux.HandleFunc("GET /site/{file}", a.handleSiteFile)
	mux.HandleFunc("GET /site/img/{file}", a.handleSitePicture)
	mux.HandleFunc("GET /docs/search.json", a.handleSearch)
	mux.HandleFunc("GET /pair", a.handlePair)
	mux.HandleFunc("GET /auth/login", a.handleLogin)
	mux.HandleFunc("GET /auth/sso", a.handleSSOReturn)
	mux.HandleFunc("POST /auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/state", a.handleState)
	mux.HandleFunc("GET /api/events", a.handleEvents)
	mux.HandleFunc("POST /api/keys", a.handleNewKey)
	mux.HandleFunc("DELETE /api/keys/{id}", a.handleDeleteKey)
	mux.HandleFunc("POST /api/unpair", a.handleUnpair)
	mux.HandleFunc("GET /v1/robots", a.handleRobots)
	mux.HandleFunc("GET /v1/robots/{id}", a.handleRobot)
	mux.HandleFunc("GET /v1/robots/{id}/events", a.handleRobotEvents)
	mux.HandleFunc("POST /v1/robots/{id}/{action}", a.handleAction)
	mux.HandleFunc("GET /v1/actions", a.handleActions)
	mux.HandleFunc("GET /v1/openapi.json", a.handleOpenAPI)
	mux.HandleFunc("GET /examples/{path...}", a.handleExample)
	mux.HandleFunc("POST /mcp", a.handleMCP)
	mux.HandleFunc("GET /mcp", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "MCP: POST JSON-RPC here (no event stream)", http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	return mux
}

/* ---------------------------------- robots --------------------------------- */

func (a *App) stateFor(id string) *robotState {
	rs := a.robots[id]
	if rs == nil {
		rs = &robotState{wake: make(chan struct{})}
		a.robots[id] = rs
	}
	return rs
}

// connected: the robot shows that it is on Dev, unless a browser drives it.
func (a *App) connected(r *robothub.Robot) {
	a.mu.Lock()
	a.stateFor(r.ID())
	keys := 0
	for _, k := range a.st.Keys {
		if k.Robot == r.ID() {
			keys++
		}
	}
	a.mu.Unlock()
	note := "pair on this robot's QR code"
	if keys > 0 {
		note = "waiting for your programs"
	}
	robot.ShowCard(r, screen.Card{Title: "Dev", Text: "Ready", Note: note})
	a.publish("robot", a.robotInfo(r.ID()))
	a.addEvent(r.ID(), gesture.Event{Name: "connected", At: time.Now()})
}

// robotEvent: what the robot reported, as what a person did; a tap may answer a question.
func (a *App) robotEvent(r *robothub.Robot, ev wire.RobotEventBody) {
	a.mu.Lock()
	rs := a.stateFor(r.ID())
	events := rs.rec.Feed(ev, time.Now())
	if q := rs.ask; q != nil {
		for _, e := range events {
			if e.Name != "tap" {
				continue
			}
			x, _ := e.Data["x"].(float64)
			y, _ := e.Data["y"].(float64)
			if i := q.hit(int(x), int(y)); i >= 0 {
				select {
				case q.answer <- tapped{i, int(x), int(y)}:
				default:
				}
			}
		}
	}
	a.mu.Unlock()
	for _, e := range events {
		a.addEvent(r.ID(), e)
	}
}

func (a *App) addEvent(robotID string, e gesture.Event) {
	a.mu.Lock()
	rs := a.stateFor(robotID)
	rs.seq++
	se := seqEvent{Seq: rs.seq, Event: e}
	rs.events = append(rs.events, se)
	if len(rs.events) > maxEvents {
		rs.events = rs.events[len(rs.events)-maxEvents:]
	}
	close(rs.wake)
	rs.wake = make(chan struct{})
	a.mu.Unlock()
	a.publish("event", map[string]any{"robot": robotID, "event": se})
}

// eventsAfter are the robot's events after seq (with names, only those), and a channel closed
// at the next one.
func (a *App) eventsAfter(robotID string, after int64, names []string) ([]seqEvent, int64, <-chan struct{}) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rs := a.stateFor(robotID)
	var out []seqEvent
	for _, e := range rs.events {
		if e.Seq > after && (len(names) == 0 || slices.Contains(names, e.Name)) {
			out = append(out, e)
		}
	}
	return out, rs.seq, rs.wake
}

// paired: the robot's browsers (its QR screen gives way to the app when there are any).
func (a *App) paired(robotID string) wire.PairedBody {
	a.mu.Lock()
	defer a.mu.Unlock()
	n, watching := 0, 0
	for sid, s := range a.st.Sessions {
		if slices.Contains(s.Robots, robotID) {
			n++
			if a.watchingLocked(sid) {
				watching++
			}
		}
	}
	for _, k := range a.st.Keys { // a program with a key counts as someone using it
		if k.Robot == robotID {
			n++
			break
		}
	}
	return wire.PairedBody{Viewers: n, Watching: watching}
}

// robotInfo is a robot as the page and the API show it.
func (a *App) robotInfo(id string) map[string]any {
	info := map[string]any{"id": id, "online": false}
	if r := a.hub.Robot(id); r != nil {
		c := r.Capabilities()
		info["online"] = true
		info["model"] = c.Model
		info["firmware"] = c.Firmware
		info["commands"] = c.Commands
		info["since"] = r.Since()
		info["readings"] = r.Telemetry()
	}
	return info
}

/* ------------------------------- browsers, keys ------------------------------ */

const cookieName = "dev_session"

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// sessionOf is the browser's session (nil without one). a.mu held.
func (a *App) sessionOfLocked(r *http.Request) (string, *session) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return "", nil
	}
	s := a.st.Sessions[c.Value]
	if s == nil {
		return "", nil
	}
	s.LastSeen = time.Now()
	return c.Value, s
}

// access is who is asking: a key (its robot) or a paired browser (its robots). Browsers must
// send X-Requested-With: dev on API calls, which other sites cannot (no CORS here).
type access struct {
	robots []string
	key    *apiKey
	sid    string
}

func (a *App) accessOf(r *http.Request) (access, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if tok, ok := cutBearer(r.Header.Get("Authorization")); ok {
		k := a.st.Keys[hashKey(tok)]
		if k == nil {
			return access{}, false
		}
		if time.Since(k.LastUsed) > time.Minute {
			k.LastUsed = time.Now()
			a.dirty = true
		}
		return access{robots: []string{k.Robot}, key: k}, true
	}
	if r.Header.Get("X-Requested-With") != "dev" {
		return access{}, false
	}
	sid, s := a.sessionOfLocked(r)
	if s == nil {
		return access{}, false
	}
	return access{robots: slices.Clone(s.Robots), sid: sid}, true
}

func cutBearer(h string) (string, bool) {
	const p = "Bearer "
	if len(h) > len(p) && h[:len(p)] == p {
		return h[len(p):], true
	}
	return "", false
}

/* ---------------------------------- the page --------------------------------- */

type subscriber struct {
	sid string
	ch  chan [2]string // event name, JSON
}

func (a *App) watchingLocked(sid string) bool {
	for s := range a.subs {
		if s.sid == sid {
			return true
		}
	}
	return false
}

// publish sends an event to the pages whose browser is paired with the robot it is about.
func (a *App) publish(name string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	robotID := ""
	switch m := v.(type) {
	case map[string]any:
		if id, ok := m["robot"].(string); ok {
			robotID = id
		} else if id, ok := m["id"].(string); ok {
			robotID = id
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for s := range a.subs {
		if sess := a.st.Sessions[s.sid]; sess != nil && slices.Contains(sess.Robots, robotID) {
			select {
			case s.ch <- [2]string{name, string(b)}:
			default: // a slow page misses one
			}
		}
	}
}
