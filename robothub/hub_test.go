package robothub

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/i41-eu/q41eu-raw/robotauth"
	"github.com/i41-eu/q41eu-raw/wire"
)

type fakeRobot struct {
	t  *testing.T
	ws *websocket.Conn
}

func dial(t *testing.T, url, id, token string) (*fakeRobot, *http.Response, error) {
	t.Helper()
	h := http.Header{}
	h.Set("Authorization", "Bearer "+token)
	h.Set(wire.DeviceIDHeader, id)
	ws, res, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(url, "http")+wire.ConnectPath, h)
	if err != nil {
		return nil, res, err
	}
	t.Cleanup(func() { ws.Close() })
	return &fakeRobot{t: t, ws: ws}, res, nil
}

func (f *fakeRobot) send(kind string, body any) {
	f.t.Helper()
	b, _ := wire.Marshal(kind, wire.Meta{}, body)
	if err := f.ws.WriteMessage(websocket.TextMessage, b); err != nil {
		f.t.Fatal(err)
	}
}

// next reads frames until one of kind, decoding its body into v.
func (f *fakeRobot) next(kind string, v any) {
	f.t.Helper()
	f.ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		mt, data, err := f.ws.ReadMessage()
		if err != nil {
			f.t.Fatalf("waiting for %s: %v", kind, err)
		}
		if mt != websocket.TextMessage {
			continue
		}
		frames, _ := wire.Parse(data)
		for _, fr := range frames {
			if fr.Kind == kind {
				if v != nil {
					fr.Decode(v)
				}
				return
			}
		}
	}
}

func (f *fakeRobot) register(commands ...string) {
	f.send(wire.KindRegister, wire.RegisterBody{Class: wire.ClassRobot, Capabilities: wire.RobotCapabilities{Model: "test", Commands: commands}})
}

func TestHub(t *testing.T) {
	var mu sync.Mutex
	var events []string
	connected := make(chan *Robot, 4)
	hub := New(Config{
		Token: "secret", View: "readings", PublicURL: "http://dev.test",
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Connected: func(r *Robot) { connected <- r },
		Event: func(r *Robot, ev wire.RobotEventBody) {
			mu.Lock()
			events = append(events, r.ID()+":"+ev.Name)
			mu.Unlock()
		},
		Paired: func(id string) wire.PairedBody { return wire.PairedBody{Viewers: 2, Watching: 1} },
	})
	ts := httptest.NewServer(hub)
	defer ts.Close()

	if _, res, err := dial(t, ts.URL, "chan-1", "wrong"); err == nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a wrong token must be refused, got %v %v", res.StatusCode, err)
	}

	robot, _, err := dial(t, ts.URL, "chan-1", "secret")
	if err != nil {
		t.Fatal(err)
	}
	robot.register("nod", "sprite")
	var acc struct {
		View string `json:"view"`
	}
	robot.next(wire.KindAccepted, &acc)
	if acc.View != "readings" {
		t.Fatalf("Accepted view %q", acc.View)
	}
	var pc wire.PairCodeBody
	robot.next(wire.KindPairCode, &pc)
	if pc.URL != "http://dev.test/pair?code="+pc.Code || len(pc.Code) != 8 {
		t.Fatalf("pair code %+v", pc)
	}
	var paired wire.PairedBody
	robot.next(wire.KindPaired, &paired)
	if paired.Viewers != 2 || !paired.Reconnect {
		t.Fatalf("paired %+v", paired)
	}
	var cmd wire.RobotCommandBody
	robot.next(wire.KindRobotCommand, &cmd)
	if cmd.Command != "sprite_clear" {
		t.Fatalf("first command %q, want sprite_clear", cmd.Command)
	}
	r := <-connected
	if !r.Has("nod") || r.Has("look") {
		t.Fatalf("capabilities %+v", r.Capabilities())
	}

	// A browser redeems the code once; the robot gets a fresh one.
	if id, ok := hub.Redeem(strings.ToLower(pc.Code)); !ok || id != "chan-1" {
		t.Fatalf("redeem: %q %v", id, ok)
	}
	if _, ok := hub.Redeem(pc.Code); ok {
		t.Fatal("a code works once")
	}
	var fresh wire.PairCodeBody
	robot.next(wire.KindPairCode, &fresh)
	if fresh.Code == pc.Code {
		t.Fatal("no fresh code after a pairing")
	}

	// Commands go out; events and telemetry come in.
	if !r.Command("nod", map[string]any{"times": 2}) {
		t.Fatal("command not queued")
	}
	robot.next(wire.KindRobotCommand, &cmd)
	if cmd.Command != "nod" || cmd.Args["times"] != float64(2) {
		t.Fatalf("command %+v", cmd)
	}
	robot.send(wire.KindRobotTelemetry, wire.RobotTelemetryBody{Measurements: map[string]float64{"battery_pct": 80}})
	robot.send(wire.KindRobotEvent, wire.RobotEventBody{Name: "head_press"})
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(events)
		mu.Unlock()
		if n == 1 && r.Telemetry()["battery_pct"] == 80 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("events %v, telemetry %v", events, r.Telemetry())
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The same robot again: the new connection replaces the old one.
	again, _, err := dial(t, ts.URL, "chan-1", "secret")
	if err != nil {
		t.Fatal(err)
	}
	again.register()
	again.next(wire.KindAccepted, nil)
	r2 := <-connected
	select {
	case <-r.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the old connection stays open")
	}
	if hub.Robot("chan-1") != r2 {
		t.Fatal("the hub does not have the new connection")
	}
	b, _ := json.Marshal(hub.Robots()[0].Capabilities())
	if !strings.Contains(string(b), `"model":"test"`) {
		t.Fatalf("robots: %s", b)
	}
}

// A robot set up by a manager: its own token, checked with the manager; the owner comes from
// there, and Report sends the paired browsers and brings back the ones to unpair.
func TestHubManager(t *testing.T) {
	var mu sync.Mutex
	var seen []robotauth.Pairing
	mgr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/robot-auth" || r.Header.Get("Authorization") != "Bearer app-secret" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		var in struct {
			Robot, Token string
			Seen         *robotauth.Seen
		}
		json.NewDecoder(r.Body).Decode(&in)
		if in.Robot != "chan-9" || in.Token != "robot-token-from-manager" {
			json.NewEncoder(w).Encode(robotauth.Auth{OK: false})
			return
		}
		out := robotauth.Auth{OK: true, Owner: "acct-1", CacheS: 60}
		if in.Seen != nil {
			mu.Lock()
			seen = in.Seen.Pairings
			mu.Unlock()
			out.Unpair = []string{"old-browser"}
		}
		json.NewEncoder(w).Encode(out)
	}))
	defer mgr.Close()

	owners := make(chan string, 4)
	connected := make(chan *Robot, 2)
	hub := New(Config{Manager: robotauth.New(mgr.URL, "app-secret"), Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Owner:     func(id, owner string) { owners <- id + "=" + owner },
		Connected: func(r *Robot) { connected <- r }})
	ts := httptest.NewServer(hub)
	defer ts.Close()

	if _, res, err := dial(t, ts.URL, "chan-9", "a-wrong-token"); err == nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatal("a token the manager does not know must be refused")
	}
	robot, _, err := dial(t, ts.URL, "chan-9", "robot-token-from-manager")
	if err != nil {
		t.Fatal(err)
	}
	robot.register()
	robot.next(wire.KindAccepted, nil)
	r := <-connected
	if !r.Managed() || <-owners != "chan-9=acct-1" {
		t.Fatal("the manager's robot and its owner")
	}
	unpair, err := hub.Report(t.Context(), "chan-9", []robotauth.Pairing{{ID: "phone-1", Device: "Chrome on Android"}})
	if err != nil || len(unpair) != 1 || unpair[0] != "old-browser" {
		t.Fatalf("report: %v %v", unpair, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0].ID != "phone-1" {
		t.Fatalf("the manager saw %+v", seen)
	}
}
