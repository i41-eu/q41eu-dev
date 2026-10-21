package devapp

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net/http"
	"slices"
	"time"

	"github.com/i41-eu/q41eu-dev/robot"
	"github.com/i41-eu/q41eu-dev/sound"
	"github.com/i41-eu/q41eu-raw/wire"
)

// Speak: a WAV (Piper's voice on the person's computer) played on the robot's speaker. It is a
// route, not an action, for the same reason as listen: audio never passes through an agent.
// Dev flushes the robot's speaker, then sends the sound in short pieces, never more than about
// 3 s ahead of what the robot has played (it buffers about 3 s).

const (
	maxSpeakBody   = 4 << 20 // bytes of WAV
	maxSpeakSecs   = 60
	speakFrameSecs = 0.5                    // seconds of audio in one speaker message
	speakAheadSecs = 2.5                    // before a frame, queued audio plus the frame is at most this
	speakWatch     = 100 * time.Millisecond // the longest the gates go unchecked while the speaker is in use
	wavSentence    = "speak takes a 16-bit mono PCM WAV, 8000 to 48000 Hz, up to 60 seconds."
)

var errWAV = errors.New(wavSentence)

// parseWAV returns the samples of a 16-bit mono PCM WAV, resampled to sound.Rate.
func parseWAV(b []byte) ([]int16, error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, errWAV
	}
	le16, le32 := binary.LittleEndian.Uint16, binary.LittleEndian.Uint32
	rate, haveFmt := 0, false
	for p := 12; p+8 <= len(b); {
		id, size := string(b[p:p+4]), int(le32(b[p+4:]))
		body := b[p+8:]
		switch id {
		case "fmt ":
			if size < 16 || len(body) < 16 || le16(body) != 1 || le16(body[2:]) != 1 || le16(body[14:]) != 16 {
				return nil, errWAV
			}
			rate = int(le32(body[4:]))
			if rate < 8000 || rate > 48000 {
				return nil, errWAV
			}
			haveFmt = true
		case "data":
			if !haveFmt {
				return nil, errWAV
			}
			size = min(size, len(body)) // a streamed WAV may claim more than it has
			n := size / 2
			if n == 0 {
				return nil, errors.New("the WAV holds no sound.")
			}
			if n > maxSpeakSecs*rate {
				return nil, errors.New("the WAV is longer than 60 seconds.")
			}
			pcm := make([]int16, n)
			for i := range pcm {
				pcm[i] = int16(le16(body[2*i:]))
			}
			if rate != sound.Rate {
				pcm = resample(pcm, rate, sound.Rate)
			}
			return pcm, nil
		}
		p += 8 + size + size%2
	}
	return nil, errWAV
}

// handleSpeak: POST a WAV as the body (audio/wav, at most 4 MB); the answer comes when the robot
// has played the sound (by the pacing clock). Until then presence, the caller and the robot's
// connection are watched; presence closing or the caller leaving flushes the speaker once and
// stops (gl:readme.md#the-api).
func (a *App) handleSpeak(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.authed(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !slices.Contains(acc.robots, id) {
		writeErr(w, errNoRobot)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, maxSpeakBody+1))
	if len(body) > maxSpeakBody {
		writeErr(w, errors.New("the WAV is larger than 4 MB."))
		return
	}
	pcm, err := parseWAV(body)
	if err != nil {
		writeErr(w, err)
		return
	}
	rb := a.hub.Robot(id)
	if rb == nil {
		writeErr(w, robot.ErrGone)
		return
	}
	cmds := rb.Capabilities().Commands
	if !slices.Contains(cmds, "speaker") || !slices.Contains(cmds, "speaker_flush") {
		writeErr(w, robot.NotSupported("a speaker that can be flushed"))
		return
	}

	// Taking the number and flushing, and checking the number and sending a piece, are each done
	// under a.mu, so an older speak sends nothing after a newer one's flush.
	a.mu.Lock()
	if ok, reason := a.presenceOf(id).check(a.now()); !ok {
		a.mu.Unlock()
		refuse(w, http.StatusForbidden, reason)
		return
	}
	rs := a.stateFor(id)
	rs.speakSeq++
	mine := rs.speakSeq
	sent := rb.Command("speaker_flush", nil)
	a.mu.Unlock()
	if !sent {
		writeErr(w, robot.ErrGone)
		return
	}

	// watch checks every gate once, under a.mu: false means the call is over and the answer is
	// written. Only a call that is still the current one flushes: a replaced one stays silent,
	// its successor has flushed already.
	offline := func() {
		refuse(w, http.StatusServiceUnavailable, "The robot went offline while speaking.")
	}
	watch := func() bool {
		select {
		case <-rb.Done():
			offline()
			return false
		default:
		}
		a.mu.Lock()
		if rs.speakSeq != mine {
			a.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "replaced": true})
			return false
		}
		if ok, reason := a.presenceOf(id).check(a.now()); !ok {
			rb.Command("speaker_flush", nil)
			a.mu.Unlock()
			refuse(w, http.StatusForbidden, reason)
			return false
		}
		select {
		case <-r.Context().Done():
			rb.Command("speaker_flush", nil)
			a.mu.Unlock()
			return false
		default:
		}
		a.mu.Unlock()
		return true
	}
	// sleep waits d, running watch at least every speakWatch; false means the call is over.
	sleep := func(d time.Duration) bool {
		end := time.Now().Add(d)
		for {
			left := time.Until(end)
			if left <= 0 {
				return true
			}
			t := time.NewTimer(min(left, speakWatch))
			select {
			case <-t.C:
			case <-r.Context().Done():
				t.Stop()
			case <-rb.Done():
				t.Stop()
			}
			if !watch() {
				return false
			}
		}
	}

	per := int(speakFrameSecs * sound.Rate)
	var first time.Time
	queuedSamples := 0 // sent so far
	for i := 0; i < len(pcm); i += per {
		piece := pcm[i:min(i+per, len(pcm))]
		if !first.IsZero() {
			queued := float64(queuedSamples)/sound.Rate - time.Since(first).Seconds()
			if wait := max(queued, 0) + float64(len(piece))/sound.Rate - speakAheadSecs; wait > 0 {
				if !sleep(time.Duration(wait * float64(time.Second))) {
					return
				}
			}
		}
		if !watch() {
			return
		}
		a.mu.Lock()
		if rs.speakSeq != mine { // replaced between the check and here
			a.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "replaced": true})
			return
		}
		sentPiece := rb.Binary(wire.BinSpeakerPCM, sound.Message(piece))
		a.mu.Unlock()
		if !sentPiece {
			offline()
			return
		}
		if first.IsZero() {
			first = time.Now()
		}
		queuedSamples += len(piece)
	}
	// The tail: the robot plays on until the pacing clock says the last sample is out.
	if !sleep(time.Until(first.Add(time.Duration(float64(len(pcm)) / sound.Rate * float64(time.Second))))) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "seconds": math.Round(float64(len(pcm))*100/sound.Rate) / 100})
}
