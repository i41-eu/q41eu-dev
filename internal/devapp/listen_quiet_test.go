package devapp

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/i41-eu/q41eu-raw/wire"
)

// TestListenQuiet: a listen ends by itself after 1.5 s of quiet that follows speech, unless the
// body says quiet_end false.
func TestListenQuiet(t *testing.T) {
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
	_, out := post(t, browser, ts.URL+"/api/keys", "", map[string]any{"robot": "chan-1", "name": "laptop quiet"}, map[string]string{"X-Requested-With": "dev"})
	key, _ := out["key"].(string)
	plain := &http.Client{}
	if code, _ := post(t, plain, ts.URL+"/v1/robots/chan-1/presence", key, map[string]any{"usb": "connected"}, nil); code != http.StatusOK {
		t.Fatalf("presence usb: %d", code)
	}

	touch := func() {
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
	// send: n stereo samples at 24000 Hz, channel 0 at v, in frames of 4800 samples.
	send := func(n int, v int16) {
		for n > 0 {
			m := min(n, 4800)
			robot.bin(wire.BinAudioMulti, audioFrame(24000, m, v, -v))
			n -= m
		}
	}
	type answer struct {
		code int
		body []byte
	}
	start := func(body string) <-chan answer {
		ch := make(chan answer, 1)
		go func() {
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/robots/chan-1/listen", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("Content-Type", "application/json")
			res, err := plain.Do(req)
			if err != nil {
				ch <- answer{}
				return
			}
			defer res.Body.Close()
			b, _ := io.ReadAll(res.Body)
			ch <- answer{res.StatusCode, b}
		}()
		return ch
	}
	mic := 0 // mic-on commands expected so far
	// run: a fresh tap, a listen with body, the sound, a second tap when tap is true; the answer is
	// read with a deadline of 5 s. A listen still running at the deadline is ended by a tap, so the
	// next case starts clean.
	run := func(name, body string, sound func(), tap bool, wantSamples int) {
		t.Helper()
		touch()
		ch := start(body)
		mic++
		robot.waitFor("mic on", func() bool { return robot.micsLocked(true) == mic })
		sound()
		if tap {
			touch()
		}
		var r answer
		select {
		case r = <-ch:
		case <-time.After(5 * time.Second):
			t.Errorf("%s: no answer in 5 s", name)
			touch()
			r = <-ch
		}
		if r.code != http.StatusOK || len(r.body) < 44 {
			t.Errorf("%s: %d %.80q, want a WAV", name, r.code, r.body)
			return
		}
		n := (len(r.body) - 44) / 2
		if d := n - wantSamples; d < -320 || d > 320 {
			t.Errorf("%s: %d samples, want %d +- 320", name, n, wantSamples)
		}
	}

	// 1 s of speech, then silence and no second tap: it ends at 2.5 s, mic off.
	run("quiet ends it", `{}`, func() { send(24000, 8000); send(72000, 0) }, false, 40000)
	robot.waitFor("mic off", func() bool { return robot.micsLocked(false) >= 1 })

	// nothing loud yet: quiet never ends it; the tap does, at 2 s.
	run("no speech yet", `{}`, func() { send(48000, 0) }, true, 32000)

	// quiet_end false: the tap ends it at 3 s.
	run("quiet_end false", `{"quiet_end":false}`, func() { send(24000, 8000); send(48000, 0) }, true, 48000)

	// a pause of 1 s is not 1.5 s.
	run("short pause", `{}`, func() { send(24000, 8000); send(24000, 0); send(24000, 8000) }, true, 48000)

	// a wrong type is 400.
	touch()
	r := <-start(`{"quiet_end":"yes"}`)
	if r.code != http.StatusBadRequest {
		t.Errorf("quiet_end yes: %d, want 400", r.code)
	}
}
