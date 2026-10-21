package devapp

import (
	"encoding/binary"
	"encoding/json"
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

const (
	tapSentence = "Nobody tapped the robot: listen starts only within 10 seconds of a tap on its screen or a pat on its head."
)

// audioFrame is a BinAudioMulti payload: rate, channels, then interleaved s16le stereo samples.
func audioFrame(rate, n int, ch0, ch1 int16) []byte {
	b := []byte{byte(rate), byte(rate >> 8), 2}
	for i := 0; i < n; i++ {
		b = binary.LittleEndian.AppendUint16(b, uint16(ch0))
		b = binary.LittleEndian.AppendUint16(b, uint16(ch1))
	}
	return b
}

// TestListen: the mic opens only after a tap on the robot and while the owner is with it, and the
// answer is a WAV, never an MCP tool.
func TestListen(t *testing.T) {
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
	_, out := post(t, browser, ts.URL+"/api/keys", "", map[string]any{"robot": "chan-1", "name": "laptop listen"}, map[string]string{"X-Requested-With": "dev"})
	key, _ := out["key"].(string)
	plain := &http.Client{}

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
	usb := func(state string) {
		t.Helper()
		if code, _ := post(t, plain, ts.URL+"/v1/robots/chan-1/presence", key, map[string]any{"usb": state}, nil); code != http.StatusOK {
			t.Fatalf("presence usb %s: %d", state, code)
		}
	}

	type answer struct {
		code   int
		header http.Header
		body   []byte
		err    error
	}
	listen := func(body string) <-chan answer {
		ch := make(chan answer, 1)
		go func() {
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/robots/chan-1/listen", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("Content-Type", "application/json")
			res, err := plain.Do(req)
			if err != nil {
				ch <- answer{err: err}
				return
			}
			defer res.Body.Close()
			b, _ := io.ReadAll(res.Body)
			ch <- answer{code: res.StatusCode, header: res.Header, body: b}
		}()
		return ch
	}
	wait := func(ch <-chan answer) answer {
		t.Helper()
		select {
		case r := <-ch:
			if r.err != nil {
				t.Fatal(r.err)
			}
			return r
		case <-time.After(5 * time.Second):
			t.Fatal("listen: no answer in 5 s")
		}
		return answer{}
	}
	errOf := func(r answer) string {
		var o map[string]any
		json.Unmarshal(r.body, &o)
		s, _ := o["error"].(string)
		return s
	}
	refused := func(name string, r answer, code int, sentence string) {
		t.Helper()
		if r.code != code || !strings.Contains(errOf(r), sentence) {
			t.Errorf("%s: %d %q, want %d %q", name, r.code, r.body, code, sentence)
		}
	}
	// checkWAV: the 44-byte header and the samples; every sample within 1000 +- 2.
	checkWAV := func(name string, r answer, want int) {
		t.Helper()
		if r.code != http.StatusOK || r.header.Get("Content-Type") != "audio/wav" || len(r.body) < 44 {
			t.Errorf("%s: %d %q %.80q, want a 200 audio/wav", name, r.code, r.header.Get("Content-Type"), r.body)
			return
		}
		b := r.body
		le16, le32 := binary.LittleEndian.Uint16, binary.LittleEndian.Uint32
		n := (len(b) - 44) / 2
		if string(b[0:4]) != "RIFF" || string(b[8:16]) != "WAVEfmt " || string(b[36:40]) != "data" ||
			le16(b[20:]) != 1 || le16(b[22:]) != 1 || le32(b[24:]) != 16000 || le32(b[28:]) != 32000 || le16(b[32:]) != 2 || le16(b[34:]) != 16 ||
			int(le32(b[40:])) != n*2 || int(le32(b[4:])) != 36+n*2 {
			t.Errorf("%s: the WAV header is wrong: %x", name, b[:44])
		}
		if n < want-1 || n > want+1 {
			t.Errorf("%s: %d samples, want %d +- 1", name, n, want)
		}
		for i := 0; i < n; i++ {
			if v := int(int16(le16(b[44+2*i:]))); v < 998 || v > 1002 {
				t.Errorf("%s: sample %d is %d, want 1000 +- 2 (channel 0 only)", name, i, v)
				break
			}
		}
	}

	// 1. presence closed (no USB report), robot tapped: refused with presence's sentence, no mic command.
	touch()
	r := wait(listen(`{}`))
	refused("presence closed", r, http.StatusForbidden, "The owner is not with the robot")
	if n := robot.mics(true) + robot.mics(false); n != 0 {
		t.Errorf("presence closed: %d mic commands reached the robot", n)
	}

	// 2. presence open, but the tap is 11 s old.
	usb("connected")
	touch()
	clock.advance(11 * time.Second)
	refused("tap too old", wait(listen(`{}`)), http.StatusForbidden, tapSentence)

	// a wrong value is 400, and a robot without a tap is not asked.
	touch()
	for _, body := range []string{`{"seconds":0}`, `{"seconds":31}`, `{"seconds":"x"}`} {
		if r := wait(listen(body)); r.code != http.StatusBadRequest {
			t.Errorf("body %s: %d, want 400", body, r.code)
		}
	}
	if n := robot.mics(true); n != 0 {
		t.Errorf("a refused listen turned the mic on %d times", n)
	}

	// 3. a tap, then listen: 3 frames of 8000 stereo samples at 24 kHz, then a second tap ends it.
	ch := listen(`{}`)
	robot.waitFor("mic on", func() bool { return robot.micsLocked(true) == 1 })
	for i := 0; i < 3; i++ {
		robot.bin(wire.BinAudioMulti, audioFrame(24000, 8000, 1000, -1000))
	}
	robot.bin(0x02, []byte{0x80, 0x3e, 1, 2, 3, 4}) // another kind: ignored
	touch()
	checkWAV("tap ends it", wait(ch), 16000)
	robot.waitFor("mic off", func() bool { return robot.micsLocked(false) == 1 })

	// 4. the same tap twice: the second listen is refused.
	refused("same tap twice", wait(listen(`{}`)), http.StatusForbidden, tapSentence)

	// 5. seconds 1 with 2 s of frames: exactly 1 s, ended by the samples, no tap.
	touch()
	ch = listen(`{"seconds":1}`)
	robot.waitFor("mic on", func() bool { return robot.micsLocked(true) == 2 })
	for i := 0; i < 6; i++ {
		robot.bin(wire.BinAudioMulti, audioFrame(24000, 8000, 1000, -1000))
	}
	checkWAV("seconds 1", wait(ch), 16000)
	robot.waitFor("mic off", func() bool { return robot.micsLocked(false) == 2 })

	// 6. never an MCP tool.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := plain.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Contains(string(raw), "listen") || strings.Contains(string(raw), "robot_mic") {
		t.Errorf("tools/list names listen or the mic: %s", raw)
	}
}
