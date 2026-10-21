package devapp

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// testClock is Dev's clock in a test: it moves only when the test says.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// TestPresenceGate: Jarvis's MCP robot tools run only while the owner is with the robot.
func TestPresenceGate(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 21, 14, 0, 0, 0, time.Local)}
	a := New(Config{RobotToken: "tok", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Version: "test", Now: clock.Now})
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	a.cfg.PublicURL = ts.URL
	robot := connectRobot(t, ts.URL)
	robot.waitFor("pair code", func() bool { return robot.code != "" })
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	browser.Get(ts.URL + "/pair?code=" + robot.code)
	_, out := post(t, browser, ts.URL+"/api/keys", "", map[string]any{"robot": "chan-1", "name": "laptop presence"}, map[string]string{"X-Requested-With": "dev"})
	key, _ := out["key"].(string)
	plain := &http.Client{}

	mcp := func(body string) map[string]any {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		res, err := plain.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var o map[string]any
		json.NewDecoder(res.Body).Decode(&o)
		return o
	}
	tool := func(name, arguments string) map[string]any {
		t.Helper()
		o := mcp(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + arguments + `}}`)
		r, _ := o["result"].(map[string]any)
		if r == nil {
			t.Fatalf("%s: no result: %v", name, o)
		}
		return r
	}
	text := func(r map[string]any) string {
		c, _ := r["content"].([]any)
		if len(c) == 0 {
			return ""
		}
		s, _ := c[0].(map[string]any)["text"].(string)
		return s
	}
	report := func(body map[string]any) int {
		t.Helper()
		code, _ := post(t, plain, ts.URL+"/v1/robots/chan-1/presence", key, body, nil)
		return code
	}
	touch := func() { // a person taps the robot's screen
		t.Helper()
		a.mu.Lock()
		before := a.stateFor("chan-1").seq
		a.mu.Unlock()
		robot.event("screen_tap", map[string]any{"x": 5.0, "y": 5.0})
		for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
			a.mu.Lock()
			seq := a.stateFor("chan-1").seq
			a.mu.Unlock()
			if seq > before {
				return
			}
		}
		t.Fatal("the tap did not arrive")
	}

	steps := []struct {
		name    string
		do      func()
		allowed bool
		refusal string // part of the sentence the refusal says
	}{
		{"no report: the USB is unknown", func() {}, false, "not with the robot"},
		{"usb connected", func() { report(map[string]any{"usb": "connected"}) }, true, ""},
		{"11 minutes with no robot event: alone", func() { clock.advance(11 * time.Minute); report(map[string]any{"usb": "connected"}) }, false, "The robot is alone: nobody touched it for 11 minutes"},
		{"a touch opens it again", func() { touch() }, true, ""},
		{"usb disconnected", func() { report(map[string]any{"usb": "disconnected"}) }, false, "the USB was disconnected at 14:11"},
		{"override 30 minutes with the USB disconnected", func() { report(map[string]any{"override_minutes": 30}) }, true, ""},
		{"override past its end", func() { clock.advance(31 * time.Minute); touch() }, false, "no override runs"},
		{"usb connected again", func() { report(map[string]any{"usb": "connected"}) }, true, ""},
		{"usb connected but the last report is 4 minutes old", func() { clock.advance(4 * time.Minute); touch() }, false, "no report in the last 3 minutes"},
		{"a heartbeat renews the report", func() { report(map[string]any{"usb": "connected"}) }, true, ""},
		{"override off ends it", func() {
			report(map[string]any{"usb": "disconnected"})
			report(map[string]any{"override_minutes": 30})
			report(map[string]any{"override_off": true})
		}, false, "the USB was disconnected"},
	}
	for _, s := range steps {
		s.do()
		r := tool("robot_say", `{"text":"hi"}`)
		refused := r["isError"] == true
		if refused == s.allowed {
			t.Fatalf("%s: robot_say refused=%v, want allowed=%v (%s)", s.name, refused, s.allowed, text(r))
		}
		if !s.allowed && !strings.Contains(text(r), s.refusal) {
			t.Fatalf("%s: refusal %q does not say %q", s.name, text(r), s.refusal)
		}
		st := tool("robot_state", `{}`)
		if st["isError"] == true {
			t.Fatalf("%s: robot_state is never refused: %v", s.name, st)
		}
		p, _ := st["structuredContent"].(map[string]any)["presence"].(map[string]any)
		if p == nil || p["allowed"] != s.allowed {
			t.Fatalf("%s: robot_state presence = %v, want allowed=%v", s.name, p, s.allowed)
		}
		for _, k := range []string{"reason", "usb", "usb_reported_at", "override_until", "idle_seconds"} {
			if _, ok := p[k]; !ok {
				t.Fatalf("%s: presence has no %q: %v", s.name, k, p)
			}
		}
	}

	// presence is not a tool: an agent cannot open its own gate.
	for _, tl := range mcp(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)["result"].(map[string]any)["tools"].([]any) {
		if strings.Contains(tl.(map[string]any)["name"].(string), "presence") {
			t.Fatalf("a presence tool: %v", tl)
		}
	}

	// REST is not gated (the gate is about Jarvis, who comes through MCP), and GET answers the object.
	if code, o := post(t, plain, ts.URL+"/v1/robots/chan-1/say", key, map[string]any{"text": "program"}, nil); code != http.StatusOK {
		t.Fatalf("REST while the gate is closed: %d %v", code, o)
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/robots/chan-1/presence", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	res, err := plain.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("GET presence: %v %v", err, res)
	}
	var got map[string]any
	json.NewDecoder(res.Body).Decode(&got)
	res.Body.Close()
	if got["allowed"] != false || got["usb"] != "disconnected" {
		t.Fatalf("GET presence: %v", got)
	}

	// a key of another robot may not set this robot's presence.
	a.mu.Lock()
	a.st.Keys[hashKey("sdev_other")] = &apiKey{ID: "other", Robot: "chan-2", Name: "other"}
	a.mu.Unlock()
	if code, _ := post(t, plain, ts.URL+"/v1/robots/chan-1/presence", "sdev_other", map[string]any{"usb": "connected"}, nil); code != http.StatusForbidden {
		t.Fatalf("a key of another robot: %d, want 403", code)
	}
}
