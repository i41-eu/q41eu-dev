package devapp

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/i41-eu/q41eu-dev/robot"
	"github.com/i41-eu/q41eu-dev/robothub"
	"github.com/i41-eu/q41eu-raw/wire"
)

// Listen: the robot's microphone, for one person's tap. It is a route, not an action: every action
// is an MCP tool, and sound must never pass through an agent's context. The answer goes to the
// HTTP caller (a program on the person's computer) as a WAV; nothing is stored.

const (
	tapWindow     = 10 * time.Second // a tap opens a listen for this long
	maxListen     = 30               // seconds
	listenSlack   = 5 * time.Second  // wall-clock time beyond the seconds asked for
	wavRate       = 16000
	noSoundReason = "The robot sent no sound: its mic may be blocked at night (22:00 to 07:00)."

	// loudRMS is the RMS (of 32767) from which a 20 ms window counts as loud. It is a first guess
	// for the robot's mic, to be tuned on the real robot.
	loudRMS      = 500
	speechTenths = 3  // tenths of a second of loud windows before quiet can end a listen
	quietTenths  = 15 // tenths of a second of quiet windows in a row that end it
)

// listener collects the channel-0 samples of one listen (a.mu).
type listener struct {
	rb      *robothub.Robot // the connection the listen started on; only its frames count
	want    int             // seconds asked for
	rate    int             // the sample rate of the first frame; 0 = none yet
	samples []int16         // channel 0, at most want*rate
	wake    chan struct{}   // after samples came (buffer 1)
	stop    chan struct{}   // a tap or pat after the mic went on (buffer 1)

	quietEnd bool  // the quiet end is on
	quiet    bool  // the quiet end came: samples after it are dropped
	winN     int   // samples in the current window
	winSq    int64 // their squares, summed
	loud     int   // samples in loud windows so far
	still    int   // samples in the quiet windows in a row
}

// hear measures one sample (a.mu): in windows of rate/50 samples (20 ms) a window is loud when its
// RMS is at least loudRMS. Speech first: only after speechTenths of loud windows in total does
// quietTenths of quiet windows in a row end the listen (quiet = true).
func (l *listener) hear(s int16) {
	l.winN++
	l.winSq += int64(s) * int64(s)
	if l.winN < l.rate/50 {
		return
	}
	if l.winSq >= int64(loudRMS)*int64(loudRMS)*int64(l.winN) {
		l.loud += l.winN
		l.still = 0
	} else {
		l.still += l.winN
	}
	l.winN, l.winSq = 0, 0
	if l.quietEnd && l.loud*10 >= l.rate*speechTenths && l.still*10 >= l.rate*quietTenths {
		l.quiet = true
	}
}

func ping(c chan struct{}) {
	select {
	case c <- struct{}{}:
	default:
	}
}

// refuse answers with a status writeErr does not know.
func refuse(w http.ResponseWriter, status int, sentence string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": sentence})
}

// binary is what the robot sends as binary frames: the microphone's go to the active listener.
func (a *App) binary(r *robothub.Robot, kind byte, payload []byte) {
	if kind != wire.BinAudioMulti || len(payload) < 3 {
		return
	}
	rate, ch := int(binary.LittleEndian.Uint16(payload)), int(payload[2])
	if rate == 0 || ch == 0 {
		return
	}
	data := payload[3:]
	n := len(data) / (2 * ch)
	a.mu.Lock()
	defer a.mu.Unlock()
	rs := a.robots[r.ID()]
	if rs == nil || rs.lis == nil {
		return
	}
	l := rs.lis
	if l.rb != r { // frames of another connection than the one the listen started on
		return
	}
	if l.rate == 0 {
		l.rate = rate
	}
	if rate != l.rate {
		return
	}
	for i := 0; i < n && len(l.samples) < l.want*l.rate && !l.quiet; i++ {
		s := int16(binary.LittleEndian.Uint16(data[i*2*ch:]))
		l.samples = append(l.samples, s)
		l.hear(s)
	}
	ping(l.wake)
}

// handleListen: POST {"seconds": 1 to 30, default 30}; the answer is a 16 kHz mono WAV.
func (a *App) handleListen(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.authed(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !slices.Contains(acc.robots, id) {
		writeErr(w, errNoRobot)
		return
	}
	var in struct {
		Seconds  *int  `json:"seconds"`
		QuietEnd *bool `json:"quiet_end"`
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			writeErr(w, fmt.Errorf("the body: a JSON object with seconds, a whole number, and quiet_end, true or false (%v)", err))
			return
		}
	}
	seconds := maxListen
	if in.Seconds != nil {
		seconds = *in.Seconds
	}
	if seconds < 1 || seconds > maxListen {
		writeErr(w, fmt.Errorf("seconds: 1 to %d", maxListen))
		return
	}
	rb := a.hub.Robot(id)
	if rb == nil {
		writeErr(w, robot.ErrGone)
		return
	}
	if !slices.Contains(rb.Capabilities().Commands, "mic") {
		writeErr(w, robot.NotSupported("a microphone"))
		return
	}

	a.mu.Lock()
	now := a.now()
	if ok, reason := a.presenceOf(id).check(now); !ok {
		a.mu.Unlock()
		refuse(w, http.StatusForbidden, reason)
		return
	}
	rs := a.stateFor(id)
	if rs.tapAt.IsZero() || now.Sub(rs.tapAt) > tapWindow {
		a.mu.Unlock()
		refuse(w, http.StatusForbidden, "Nobody tapped the robot: listen starts only within 10 seconds of a tap on its screen or a pat on its head.")
		return
	}
	if rs.lis != nil {
		a.mu.Unlock()
		refuse(w, http.StatusConflict, "The robot is already listening.")
		return
	}
	l := &listener{rb: rb, want: seconds, quietEnd: in.QuietEnd == nil || *in.QuietEnd, wake: make(chan struct{}, 1), stop: make(chan struct{}, 1)}
	rs.tapAt = time.Time{} // one tap opens one listen
	rs.lis = l
	a.mu.Unlock()

	ended := false
	end := func() { // the mic goes off and the listener goes, once
		if ended {
			return
		}
		ended = true
		a.mu.Lock()
		if rs.lis == l {
			rs.lis = nil
		}
		a.mu.Unlock()
		rb.Command("mic", map[string]any{"on": false})
		if cur := a.hub.Robot(id); cur != nil && cur != rb { // the robot came back on a new connection
			cur.Command("mic", map[string]any{"on": false})
		}
	}
	defer end()
	if !rb.Command("mic", map[string]any{"on": true}) {
		writeErr(w, robot.ErrGone)
		return
	}

	limit := time.NewTimer(time.Duration(seconds)*time.Second + listenSlack)
	defer limit.Stop()
	// closed says whether the owner is no longer with the robot; the reason is presence's sentence.
	closed := func() (bool, string) {
		a.mu.Lock()
		defer a.mu.Unlock()
		ok, reason := a.presenceOf(id).check(a.now())
		return !ok, reason
	}
wait:
	for {
		select {
		case <-l.wake:
			a.mu.Lock()
			full := l.quiet || l.rate > 0 && len(l.samples) >= l.want*l.rate
			a.mu.Unlock()
			if no, reason := closed(); no {
				end()
				refuse(w, http.StatusForbidden, reason)
				return
			}
			if full {
				break wait
			}
		case <-l.stop:
			break wait
		case <-limit.C:
			break wait
		case <-rb.Done():
			end()
			refuse(w, http.StatusServiceUnavailable, "The robot went offline while listening.")
			return
		case <-r.Context().Done():
			return
		}
	}
	end()
	if no, reason := closed(); no {
		refuse(w, http.StatusForbidden, reason)
		return
	}
	a.mu.Lock()
	rate, samples := l.rate, slices.Clone(l.samples)
	a.mu.Unlock()
	if len(samples) == 0 {
		refuse(w, http.StatusGatewayTimeout, noSoundReason)
		return
	}
	pcm := resample(samples, rate, wavRate)
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(wavBytes(pcm, wavRate))
}

// resample converts mono samples from one rate to another by linear interpolation; the output
// holds len(in)*to/from samples.
func resample(in []int16, from, to int) []int16 {
	n := len(in) * to / from
	out := make([]int16, n)
	for i := range out {
		pos := i * from // in units of 1/to samples
		j, frac := pos/to, pos%to
		v := float64(in[j])
		if j+1 < len(in) && frac > 0 {
			v += (float64(in[j+1]) - v) * float64(frac) / float64(to)
		}
		out[i] = int16(v + 0.5*sign(v))
	}
	return out
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// wavBytes is a 44-byte RIFF header, then the 16-bit mono PCM.
func wavBytes(pcm []int16, rate int) []byte {
	b := make([]byte, 0, 44+2*len(pcm))
	le32 := binary.LittleEndian.AppendUint32
	le16 := binary.LittleEndian.AppendUint16
	b = append(b, "RIFF"...)
	b = le32(b, uint32(36+2*len(pcm)))
	b = append(b, "WAVEfmt "...)
	b = le32(b, 16)
	b = le16(b, 1) // PCM
	b = le16(b, 1) // mono
	b = le32(b, uint32(rate))
	b = le32(b, uint32(rate*2))
	b = le16(b, 2)
	b = le16(b, 16)
	b = append(b, "data"...)
	b = le32(b, uint32(2*len(pcm)))
	for _, s := range pcm {
		b = le16(b, uint16(s))
	}
	return b
}
