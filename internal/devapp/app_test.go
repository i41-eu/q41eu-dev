package devapp

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/i41-eu/q41eu-raw/robotauth"
	"github.com/i41-eu/q41eu-raw/sso"
	"github.com/i41-eu/q41eu-raw/wire"
)

// robotSim is a fake robot: it records what it gets and can send events.
type robotSim struct {
	t    *testing.T
	ws   *websocket.Conn
	mu   sync.Mutex
	cmds []wire.RobotCommandBody
	bins []byte
	code string
}

func connectRobot(t *testing.T, url string) *robotSim {
	h := http.Header{}
	h.Set("Authorization", "Bearer tok")
	h.Set(wire.DeviceIDHeader, "chan-1")
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(url, "http")+wire.ConnectPath, h)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	s := &robotSim{t: t, ws: ws}
	reg, _ := wire.Marshal(wire.KindRegister, wire.Meta{}, wire.RegisterBody{Class: wire.ClassRobot,
		Capabilities: wire.RobotCapabilities{Model: "sim", Commands: []string{"image", "say", "nod", "look", "leds", "face", "speaker", "sticker", "server_switch"}}})
	ws.WriteMessage(websocket.TextMessage, reg)
	go func() {
		for {
			mt, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			s.mu.Lock()
			if mt == websocket.BinaryMessage {
				s.bins = append(s.bins, data[0])
			} else if frames, err := wire.Parse(data); err == nil {
				for _, f := range frames {
					switch f.Kind {
					case wire.KindRobotCommand:
						var c wire.RobotCommandBody
						f.Decode(&c)
						s.cmds = append(s.cmds, c)
					case wire.KindPairCode:
						var pc wire.PairCodeBody
						f.Decode(&pc)
						s.code = pc.Code
					}
				}
			}
			s.mu.Unlock()
		}
	}()
	return s
}

func (s *robotSim) event(name string, data map[string]any) {
	b, _ := wire.Marshal(wire.KindRobotEvent, wire.Meta{}, wire.RobotEventBody{Name: name, Data: data})
	s.ws.WriteMessage(websocket.TextMessage, b)
}

func (s *robotSim) waitFor(what string, ok func() bool) {
	s.t.Helper()
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		s.mu.Lock()
		done := ok()
		s.mu.Unlock()
		if done {
			return
		}
	}
	s.t.Fatalf("robot: no %s", what)
}

func post(t *testing.T, c *http.Client, url, key string, body any, hdr map[string]string) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	raw, _ := io.ReadAll(res.Body)
	json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

func TestDevApp(t *testing.T) {
	a := New(Config{RobotToken: "tok", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Version: "test"})
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	a.cfg.PublicURL = ts.URL
	robot := connectRobot(t, ts.URL)
	robot.waitFor("pair code and welcome card", func() bool { return robot.code != "" && len(robot.bins) == 1 })

	// A browser pairs by the code and makes a key.
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	res, err := browser.Get(ts.URL + "/pair?code=" + robot.code)
	if err != nil || res.StatusCode != http.StatusOK || res.Request.URL.Path != "/robots" {
		t.Fatalf("pair: %v %v", err, res.Status)
	}
	dev := map[string]string{"X-Requested-With": "dev"}
	if code, _ := post(t, browser, ts.URL+"/api/keys", "", map[string]any{"robot": "chan-1", "name": "laptop"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("a key without the page's header: %d", code)
	}
	code, out := post(t, browser, ts.URL+"/api/keys", "", map[string]any{"robot": "chan-1", "name": "laptop"}, dev)
	key, _ := out["key"].(string)
	if code != http.StatusOK || !strings.HasPrefix(key, "sdev_") {
		t.Fatalf("new key: %d %v", code, out)
	}

	// Programs: no key, no robot; with it, actions.
	plain := &http.Client{}
	if code, _ := post(t, plain, ts.URL+"/v1/robots/chan-1/show", "", map[string]any{"text": "x"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("without a key: %d", code)
	}
	if code, out := post(t, plain, ts.URL+"/v1/robots/chan-1/show", key, map[string]any{"text": "Hello", "color": "green", "progress": 0.5}, nil); code != http.StatusOK {
		t.Fatalf("show: %d %v", code, out)
	}
	robot.waitFor("the card", func() bool { return len(robot.bins) == 2 && robot.bins[1] == wire.BinShowJPEG })
	if code, out := post(t, plain, ts.URL+"/v1/robots/chan-1/move", key, map[string]any{"gesture": "wave"}, nil); code != http.StatusBadRequest {
		t.Fatalf("a bad argument: %d %v", code, out)
	}
	if code, _ := post(t, plain, ts.URL+"/v1/robots/chan-1/command", key, map[string]any{"command": "server_switch", "args": map[string]any{"server": "x"}}, nil); code != http.StatusBadRequest {
		t.Fatalf("switching the robot's app is not a program's: %d", code)
	}
	if code, out := post(t, plain, ts.URL+"/v1/robots/chan-2/say", key, map[string]any{"text": "hi"}, nil); code != http.StatusNotFound {
		t.Fatalf("another robot: %d %v", code, out)
	}
	post(t, plain, ts.URL+"/v1/robots/chan-1/leds", key, map[string]any{"bar": 0.5, "color": "#ff0000"}, nil)
	robot.waitFor("leds", func() bool {
		for _, c := range robot.cmds {
			if c.Command == "leds" && len(c.Args["pixels"].([]any)) == 12 {
				return true
			}
		}
		return false
	})

	// ask: the question waits for a tap on an answer.
	type result struct {
		code int
		out  map[string]any
	}
	done := make(chan result, 1)
	go func() {
		c, o := post(t, plain, ts.URL+"/v1/robots/chan-1/ask", key, map[string]any{"question": "Coffee or tea?", "answers": []any{"Coffee", "Tea"}, "timeout_s": 5.0}, nil)
		done <- result{c, o}
	}()
	robot.waitFor("the question", func() bool { return len(robot.bins) >= 3 })
	time.Sleep(50 * time.Millisecond)
	robot.event("screen_tap", map[string]any{"x": 5.0, "y": 5.0})     // not on a button
	robot.event("screen_tap", map[string]any{"x": 240.0, "y": 200.0}) // "Tea", the right one of two
	select {
	case r := <-done:
		if r.code != http.StatusOK || r.out["answer"] != "Tea" {
			t.Fatalf("ask: %d %v", r.code, r.out)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("ask did not answer")
	}

	// events: what people did, after a sequence number.
	robot.event("head_release", map[string]any{"ms": 2000.0})
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/robots/chan-1/events?after=0&names=pet&seconds=2", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	res, err = plain.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var evs struct {
		Events []seqEvent `json:"events"`
	}
	json.NewDecoder(res.Body).Decode(&evs)
	res.Body.Close()
	if len(evs.Events) != 1 || evs.Events[0].Name != "pet" || evs.Events[0].Data["seconds"] != 2.0 {
		t.Fatalf("events: %+v", evs)
	}

	// MCP: the same actions as tools.
	mcp := func(body string) map[string]any {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		res, err := plain.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode == http.StatusAccepted {
			return nil
		}
		var out map[string]any
		json.NewDecoder(res.Body).Decode(&out)
		return out
	}
	init := mcp(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if init["result"].(map[string]any)["protocolVersion"] != "2025-06-18" {
		t.Fatalf("initialize: %v", init)
	}
	if mcp(`{"jsonrpc":"2.0","method":"notifications/initialized"}`) != nil {
		t.Fatal("a notification gets no answer")
	}
	tools := mcp(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)["result"].(map[string]any)["tools"].([]any)
	if len(tools) != len(Actions) || tools[1].(map[string]any)["name"] != "robot_show" {
		t.Fatalf("tools: %v", tools)
	}
	call := mcp(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"robot_say","arguments":{"text":"Hi from an agent"}}}`)
	if r := call["result"].(map[string]any); r["isError"] == true {
		t.Fatalf("say: %v", r)
	}
	robot.waitFor("say", func() bool { return robot.cmds[len(robot.cmds)-1].Command == "say" })
	bad := mcp(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"robot_say","arguments":{}}}`)
	if bad["result"].(map[string]any)["isError"] != true {
		t.Fatalf("a missing argument is a tool error: %v", bad)
	}
}

// On the internet: a robot set up by a manager, its key shown to the owner there (and removed
// there), and actions limited per key.
func TestManagerAndLimits(t *testing.T) {
	var mu sync.Mutex
	var reported []string
	unpair := ""
	mgr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Robot, Token string
			Seen         *robotauth.Seen
		}
		json.NewDecoder(r.Body).Decode(&in)
		out := robotauth.Auth{OK: in.Token == "tok", Owner: "acct-1", CacheS: 60}
		mu.Lock()
		if in.Seen != nil {
			reported = nil
			for _, p := range in.Seen.Pairings {
				reported = append(reported, p.ID+" "+p.Device)
			}
			if unpair != "" {
				out.Unpair = []string{unpair}
			}
		}
		mu.Unlock()
		json.NewEncoder(w).Encode(out)
	}))
	defer mgr.Close()
	a := New(Config{Manager: robotauth.New(mgr.URL, "secret"), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	a.cfg.PublicURL = ts.URL
	robot := connectRobot(t, ts.URL)
	robot.waitFor("pair code", func() bool { return robot.code != "" })

	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	browser.Get(ts.URL + "/pair?code=" + robot.code)
	_, out := post(t, browser, ts.URL+"/api/keys", "", map[string]any{"robot": "chan-1", "name": "laptop"}, map[string]string{"X-Requested-With": "dev"})
	key, id := out["key"].(string), out["id"].(string)

	a.report(t.Context(), "chan-1")
	mu.Lock()
	got := strings.Join(reported, "; ")
	mu.Unlock()
	if !strings.Contains(got, "key-"+id+" API key: laptop") || strings.Count(got, ";") != 1 {
		t.Fatalf("the manager sees the browser and the key: %q", got)
	}

	// Too many actions: 429.
	plain := &http.Client{}
	limited := false
	for range actionsBurst + 5 {
		if code, _ := post(t, plain, ts.URL+"/v1/robots/chan-1/move", key, map[string]any{"gesture": "nod"}, nil); code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("no limit on actions")
	}

	// The owner removes the key on the manager: it stops working.
	mu.Lock()
	unpair = "key-" + id
	mu.Unlock()
	a.report(t.Context(), "chan-1")
	time.Sleep(1100 * time.Millisecond) // a token back in the bucket
	if code, _ := post(t, plain, ts.URL+"/v1/robots/chan-1/state", key, nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("a key removed on the manager still works: %d", code)
	}

	// Wrong pairing codes: a limit per address.
	tooMany := false
	for range pairFailsPer10m + 2 {
		res, err := plain.Get(ts.URL + "/pair?code=WRONGONE")
		if err == nil && res.StatusCode == http.StatusTooManyRequests {
			tooMany = true
		}
		if err == nil {
			res.Body.Close()
		}
	}
	if !tooMany {
		t.Fatal("no limit on wrong pairing codes")
	}
}

// The docs: the OpenAPI document (from the actions) and the examples' files, without a key.
func TestDocs(t *testing.T) {
	a := New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), PublicURL: "https://dev.example", Version: "test"})
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	res, err := http.Get(ts.URL + "/v1/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		OpenAPI string                    `json:"openapi"`
		Servers []struct{ URL string }    `json:"servers"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	json.NewDecoder(res.Body).Decode(&doc)
	res.Body.Close()
	if doc.OpenAPI != "3.1.0" || doc.Servers[0].URL != "https://dev.example" {
		t.Fatalf("openapi %q servers %v", doc.OpenAPI, doc.Servers)
	}
	for _, act := range Actions {
		p := "/v1/robots/{robot}/" + act.Name
		if act.Name == "state" {
			p = "/v1/robots/{robot}"
		}
		if doc.Paths[p] == nil {
			t.Errorf("no path for %s", act.Name)
		}
	}
	for _, f := range []string{"curl/hello.sh", "python/ask.py", "go/events/main.go", "README.md"} {
		res, err := http.Get(ts.URL + "/examples/" + f)
		if err != nil || res.StatusCode != http.StatusOK {
			t.Fatalf("%s: %v %v", f, err, res.Status)
		}
		res.Body.Close()
	}
	if res, _ := http.Get(ts.URL + "/examples/../go.mod"); res.StatusCode == http.StatusOK {
		t.Fatal("only the examples")
	}
}

// Sign-in through the manager: the robot's owner, signed in, gets it without the QR code (and can
// make keys); someone else signed in gets nothing; signing out ends it here.
func TestSignIn(t *testing.T) {
	var mu sync.Mutex
	var next *sso.Account
	codes, handles := map[string]sso.Account{}, map[string]sso.Account{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sso", func(w http.ResponseWriter, r *http.Request) {
		ret := r.URL.Query().Get("return")
		mu.Lock()
		defer mu.Unlock()
		if next == nil {
			http.Redirect(w, r, ret+"&error=login_required", http.StatusFound)
			return
		}
		codes["c"+next.Key] = *next
		http.Redirect(w, r, ret+"&code=c"+next.Key, http.StatusFound)
		next = nil
	})
	mux.HandleFunc("POST /api/", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Robot, Token, Code, Handle string }
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/api/robot-auth":
			json.NewEncoder(w).Encode(robotauth.Auth{OK: req.Token == "tok", Owner: "ema", CacheS: 60})
		case "/api/sso/token":
			a, ok := codes[req.Code]
			delete(codes, req.Code)
			handles["h"+a.Key] = a
			json.NewEncoder(w).Encode(sso.Answer{OK: ok, Handle: "h" + a.Key, Account: &a, CacheS: 60})
		case "/api/sso/check":
			a, ok := handles[req.Handle]
			json.NewEncoder(w).Encode(sso.Answer{OK: ok, Account: &a})
		case "/api/sso/logout":
			delete(handles, req.Handle)
		}
	})
	mgr := httptest.NewServer(mux)
	defer mgr.Close()

	var h http.Handler
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }))
	defer ts.Close()
	a := New(Config{Manager: robotauth.New(mgr.URL, "secret"), SignIn: sso.New(mgr.URL, "secret"), ManagerURL: mgr.URL,
		PublicURL: ts.URL, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	h = a.Handler()
	robot := connectRobot(t, ts.URL)
	robot.waitFor("pair code", func() bool { return robot.code != "" })

	signIn := func(key, name string) (*http.Client, map[string]any) {
		mu.Lock()
		next = &sso.Account{Key: key, Name: name}
		mu.Unlock()
		jar, _ := cookiejar.New(nil)
		c := &http.Client{Jar: jar}
		res, err := c.Get(ts.URL + "/auth/login?next=/")
		if err != nil || res.StatusCode != http.StatusOK {
			t.Fatalf("sign-in: %v %v", err, res.Status)
		}
		res.Body.Close()
		res, _ = c.Get(ts.URL + "/api/state")
		var st map[string]any
		json.NewDecoder(res.Body).Decode(&st)
		res.Body.Close()
		return c, st
	}
	owner, st := signIn("ema", "Ema")
	me := st["me"].(map[string]any)
	if robots := st["robots"].([]any); len(robots) != 1 || me["signed_in"] != true || me["manager_url"] != mgr.URL {
		t.Fatalf("the owner signed in: robots %v, me %v", robots, me)
	}
	if code, out := post(t, owner, ts.URL+"/api/keys", "", map[string]any{"robot": "chan-1", "name": "laptop"}, map[string]string{"X-Requested-With": "dev"}); code != http.StatusOK {
		t.Fatalf("the owner makes a key: %d %v", code, out)
	}
	if _, st := signIn("someone", "Someone"); len(st["robots"].([]any)) != 0 {
		t.Fatalf("someone else gets the robot: %v", st["robots"])
	}
	post(t, owner, ts.URL+"/auth/logout", "", nil, map[string]string{"X-Requested-With": "dev"})
	res, _ := owner.Get(ts.URL + "/api/state")
	json.NewDecoder(res.Body).Decode(&st)
	res.Body.Close()
	if st["me"].(map[string]any)["signed_in"] == true {
		t.Fatal("still signed in after signing out")
	}
}

// The docs pages and Home answer an anonymous request at once, with the page (a search engine, a
// link preview, curl); Your robots, and Run with silent=1, try the silent sign-in at the manager,
// once in a while; Home only for a browser the manager's hint cookie names.
func TestSignInPages(t *testing.T) {
	const mgr = "https://manager.example"
	a := New(Config{SignIn: sso.New(mgr, "secret"), SignInAlways: true, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	a.cfg.PublicURL = ts.URL
	client := func() *http.Client { // a browser that does not follow redirects, so they can be checked
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	get := func(c *http.Client, path string, hint bool) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		if hint {
			req.AddCookie(&http.Cookie{Name: sso.HintCookie, Value: "h1"})
		}
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return res, string(b)
	}
	silent := func(res *http.Response) bool {
		loc := res.Header.Get("Location")
		return res.StatusCode == http.StatusFound && strings.HasPrefix(loc, mgr+"/sso?") && strings.Contains(loc, "silent=1")
	}

	for _, p := range sitePages {
		res, body := get(client(), p.Path, false)
		if p.Path == "/robots" {
			if !silent(res) {
				t.Errorf("%s: %d %q, want the silent sign-in", p.Path, res.StatusCode, res.Header.Get("Location"))
			}
			continue
		}
		if res.StatusCode != http.StatusOK || res.Header.Get("Location") != "" || res.Header.Get("Set-Cookie") != "" || !strings.Contains(body, "<h1>") {
			t.Errorf("%s: %d, Location %q, Set-Cookie %q: want the page itself", p.Path, res.StatusCode, res.Header.Get("Location"), res.Header.Get("Set-Cookie"))
		}
	}
	if res, _ := get(client(), "/", true); !silent(res) {
		t.Errorf("home with the manager's hint: %d, want the silent sign-in", res.StatusCode)
	}
	if res, _ := get(client(), "/docs/api", true); res.StatusCode != http.StatusOK {
		t.Errorf("a docs page with the manager's hint: %d", res.StatusCode)
	}

	// Your robots tries once; then the page, without going there again.
	c := client()
	if res, _ := get(c, "/robots", false); !silent(res) {
		t.Fatalf("/robots: %d", res.StatusCode)
	}
	if res, _ := get(c, "/robots", false); res.StatusCode != http.StatusOK {
		t.Fatalf("/robots tried again: %d %q", res.StatusCode, res.Header.Get("Location"))
	}

	// Run on a page without a robot: silent=1 goes to the manager, back to the example; tried
	// lately, straight back with ?signin=no.
	c = client()
	run := "/auth/login?silent=1&next=" + url.QueryEscape("/docs/examples#ex-hello")
	if res, _ := get(c, run, false); !silent(res) || !strings.Contains(res.Header.Get("Location"), url.QueryEscape(a.ssoReturn("/docs/examples#ex-hello"))) {
		t.Fatalf("Run's sign-in: %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	if res, _ := get(c, run, false); res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/docs/examples?signin=no#ex-hello" {
		t.Fatalf("Run's sign-in again: %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	if res, _ := get(client(), "/auth/login?silent=1&next=//elsewhere.example", false); !strings.Contains(res.Header.Get("Location"), url.QueryEscape(a.ssoReturn("/"))) {
		t.Fatalf("next elsewhere: %q", res.Header.Get("Location"))
	}

	// Dev on the manager's domain (no SignInAlways): only a browser with the hint goes there.
	a.cfg.SignInAlways = false
	if res, _ := get(client(), "/robots", false); res.StatusCode != http.StatusOK {
		t.Errorf("/robots without the hint: %d", res.StatusCode)
	}
	if res, _ := get(client(), "/robots", true); !silent(res) {
		t.Errorf("/robots with the hint: %d", res.StatusCode)
	}
	if res, _ := get(client(), run, false); res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/docs/examples?signin=no#ex-hello" {
		t.Errorf("Run's sign-in without the hint: %d %q", res.StatusCode, res.Header.Get("Location"))
	}
}
