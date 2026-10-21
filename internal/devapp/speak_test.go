package devapp

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testWAV is a WAV file of a 440 Hz sine (amplitude 8000), with a LIST chunk before the data.
func testWAV(rate, channels, bits int, seconds float64) []byte {
	le32, le16 := binary.LittleEndian.AppendUint32, binary.LittleEndian.AppendUint16
	n := int(seconds * float64(rate))
	var data []byte
	for i := 0; i < n; i++ {
		v := int16(8000 * math.Sin(2*math.Pi*440*float64(i)/float64(rate)))
		for c := 0; c < channels; c++ {
			if bits == 8 {
				data = append(data, byte(int(v>>8)+128))
			} else {
				data = le16(data, uint16(v))
			}
		}
	}
	b := []byte("RIFF")
	b = le32(b, uint32(4+8+16+8+10+8+len(data)))
	b = append(b, "WAVEfmt "...)
	b = le32(b, 16)
	b = le16(b, 1)
	b = le16(b, uint16(channels))
	b = le32(b, uint32(rate))
	b = le32(b, uint32(rate*channels*bits/8))
	b = le16(b, uint16(channels*bits/8))
	b = le16(b, uint16(bits))
	b = append(b, "LIST"...)
	b = le32(b, 10)
	b = append(b, "INFOabcdef"...)
	b = append(b, "data"...)
	b = le32(b, uint32(len(data)))
	return append(b, data...)
}

// TestSpeak: a WAV is streamed to the robot's speaker, flush first, paced so the robot's buffer
// never holds more than about 3 s; a new speak replaces an old one; never an MCP tool.
func TestSpeak(t *testing.T) {
	a := New(Config{RobotToken: "tok", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Version: "test"})
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	a.cfg.PublicURL = ts.URL
	robot := connectRobot(t, ts.URL)
	robot.waitFor("pair code", func() bool { return robot.code != "" })
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	browser.Get(ts.URL + "/pair?code=" + robot.code)
	_, out := post(t, browser, ts.URL+"/api/keys", "", map[string]any{"robot": "chan-1", "name": "laptop speak"}, map[string]string{"X-Requested-With": "dev"})
	key, _ := out["key"].(string)
	plain := &http.Client{}

	type answer struct {
		code int
		body []byte
		err  error
	}
	speak := func(body []byte) <-chan answer {
		ch := make(chan answer, 1)
		go func() {
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/robots/chan-1/speak", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("Content-Type", "audio/wav")
			res, err := plain.Do(req)
			if err != nil {
				ch <- answer{err: err}
				return
			}
			defer res.Body.Close()
			b, _ := io.ReadAll(res.Body)
			ch <- answer{code: res.StatusCode, body: b}
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
		case <-time.After(15 * time.Second):
			t.Fatal("speak: no answer in 15 s")
		}
		return answer{}
	}
	obj := func(r answer) map[string]any {
		var o map[string]any
		json.Unmarshal(r.body, &o)
		return o
	}
	snapshot := func() []simEntry {
		robot.mu.Lock()
		defer robot.mu.Unlock()
		return append([]simEntry(nil), robot.log...)
	}
	// from the log: the speaker_flush and frame entries since index from.
	speakerLog := func(from int) []simEntry {
		var l []simEntry
		for _, e := range snapshot()[from:] {
			if e.cmd == "speaker_flush" || e.cmd == "" {
				l = append(l, e)
			}
		}
		return l
	}
	mark := func() int { return len(snapshot()) }

	// 1. presence closed (no USB report): refused, nothing reached the speaker.
	start := mark()
	r := wait(speak(testWAV(16000, 1, 16, 1)))
	if e, _ := obj(r)["error"].(string); r.code != http.StatusForbidden || !strings.Contains(e, "The owner is not with the robot") {
		t.Errorf("presence closed: %d %s, want 403 with presence's sentence", r.code, r.body)
	}
	time.Sleep(100 * time.Millisecond)
	if l := speakerLog(start); len(l) != 0 {
		t.Errorf("presence closed: %d speaker entries reached the robot", len(l))
	}

	// presence open: the USB is connected and a person taps.
	if code, _ := post(t, plain, ts.URL+"/v1/robots/chan-1/presence", key, map[string]any{"usb": "connected"}, nil); code != http.StatusOK {
		t.Fatalf("presence usb: %d", code)
	}
	robot.event("screen_tap", map[string]any{"x": 5.0, "y": 5.0})
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		a.mu.Lock()
		seq := a.stateFor("chan-1").seq
		a.mu.Unlock()
		if seq > 0 {
			break
		}
	}

	// 2. a 5 s WAV at 22050 Hz: flush first, 80000 samples at 16 kHz, never more than 3 s ahead.
	start = mark()
	r = wait(speak(testWAV(22050, 1, 16, 5)))
	if o := obj(r); r.code != http.StatusOK || o["ok"] != true || o["seconds"] != 5.0 {
		t.Errorf("5 s speak: %d %s, want 200 ok seconds 5", r.code, r.body)
	}
	time.Sleep(100 * time.Millisecond)
	l := speakerLog(start)
	if len(l) < 2 || l[0].cmd != "speaker_flush" {
		t.Fatalf("5 s speak: the log must start with speaker_flush then frames: %v", l)
	}
	total, first := 0, time.Time{}
	for _, e := range l[1:] {
		if e.cmd != "" {
			t.Errorf("5 s speak: another flush among the frames")
			continue
		}
		if first.IsZero() {
			first = e.at
		}
		total += e.samples
		if ahead := float64(total)/16000 - e.at.Sub(first).Seconds(); ahead > 3.1 {
			t.Errorf("5 s speak: %.2f s of audio ahead of the clock at %d samples, want at most 3.0", ahead, total)
		}
	}
	if total < 80000-2 || total > 80000+2 {
		t.Errorf("5 s speak: %d samples, want 80000 +- 2", total)
	}

	// 3. replace: a 6 s speak, then after its first frame a 1 s one.
	start = mark()
	ch1 := speak(testWAV(16000, 1, 16, 6))
	robot.waitFor("first frame", func() bool {
		for _, e := range robot.log[start:] {
			if e.cmd == "" {
				return true
			}
		}
		return false
	})
	r2 := wait(speak(testWAV(16000, 1, 16, 1)))
	r1 := wait(ch1)
	if o := obj(r1); r1.code != http.StatusOK || o["replaced"] != true {
		t.Errorf("replaced speak: %d %s, want 200 replaced", r1.code, r1.body)
	}
	if o := obj(r2); r2.code != http.StatusOK || o["seconds"] != 1.0 {
		t.Errorf("second speak: %d %s, want 200 seconds 1", r2.code, r2.body)
	}
	time.Sleep(100 * time.Millisecond)
	l = speakerLog(start)
	flushes := 0
	second := -1
	for i, e := range l {
		if e.cmd == "speaker_flush" {
			if flushes++; flushes == 2 {
				second = i
			}
		}
	}
	if second < 0 {
		t.Fatalf("replace: want two speaker_flush, got %d: %v", flushes, l)
	}
	after := 0
	for _, e := range l[second+1:] {
		after += e.samples
	}
	if after != 16000 {
		t.Errorf("replace: %d samples after the second flush, want exactly the second line's 16000", after)
	}

	// 4. wrong files: stereo, 8-bit, not a WAV, 61 s.
	for name, body := range map[string][]byte{
		"stereo":      testWAV(16000, 2, 16, 1),
		"8-bit":       testWAV(16000, 1, 8, 1),
		"not a WAV":   []byte("this is not a WAV file at all, just words"),
		"61 seconds":  testWAV(8000, 1, 16, 61),
		"empty data":  testWAV(16000, 1, 16, 0),
		"rate 4000":   testWAV(4000, 1, 16, 1),
		"empty body":  nil,
		"cut at data": testWAV(16000, 1, 16, 1)[:60],
	} {
		before := mark()
		if r := wait(speak(body)); r.code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, r.code, r.body)
		}
		if l := speakerLog(before); len(l) != 0 {
			t.Errorf("%s: %d speaker entries reached the robot", name, len(l))
		}
	}

	// 5. never an MCP tool.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := plain.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Contains(string(raw), "robot_speak") {
		t.Errorf("tools/list names robot_speak: %s", raw)
	}
}
