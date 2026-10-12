// Package sound makes short sounds for the robot's speaker as 16 kHz mono PCM: tones, bells,
// glides, melodies written as notes ("C5:120 E5:120 G5:300"), and named chimes from the apps
// (Focus's bells, the pet's chirps). The speaker message (wire.BinSpeakerPCM) is the sample rate,
// then s16le samples; the robot buffers about 3 s, so Chunks splits longer sounds for a paced
// send.
package sound

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
)

// Rate is the sample rate of every sound here.
const Rate = 16000

// MaxChunkMs: a speaker message holds at most this much (the robot buffers ~3 s).
const MaxChunkMs = 2500

const peak = 0.33 * math.MaxInt16 // headroom: the robot's small speaker distorts when loud

func samples(ms float64) int { return int(ms * Rate / 1000) }

// Silence is a pause.
func Silence(ms float64) []int16 { return make([]int16, samples(ms)) }

// envelope softens both ends so a tone does not click.
func envelope(i, n int) float64 {
	a := samples(8)
	switch {
	case i < a:
		return float64(i) / float64(a)
	case i > n-4*a:
		return max(0, float64(n-i)/float64(4*a))
	}
	return 1
}

// Tone is a soft square-ish tone (sine plus a little third harmonic); hz 0 is a pause. gain 0..1.
func Tone(hz, ms, gain float64) []int16 {
	n := samples(ms)
	out := make([]int16, n)
	if hz <= 0 {
		return out
	}
	for i := range n {
		ph := 2 * math.Pi * hz * float64(i) / Rate
		out[i] = int16((math.Sin(ph) + 0.25*math.Sin(3*ph)) / 1.25 * envelope(i, n) * gain * peak)
	}
	return out
}

// Bell is a struck tone: soft attack, exponential decay, a quiet octave for shine.
func Bell(hz, ms, gain float64) []int16 {
	n := samples(ms)
	out := make([]int16, n)
	if hz <= 0 {
		return out
	}
	for i := range n {
		t := float64(i) / Rate
		attack := math.Min(1, float64(i)/(0.005*Rate))
		release := math.Min(1, float64(n-i)/(0.01*Rate))
		v := math.Sin(2*math.Pi*hz*t) + 0.25*math.Sin(4*math.Pi*hz*t)
		out[i] = int16(peak / 1.25 * v * attack * math.Exp(-t*4) * release * gain)
	}
	return out
}

// Glide sweeps from one frequency to another.
func Glide(fromHz, toHz, ms, gain float64) []int16 {
	n := samples(ms)
	out := make([]int16, n)
	ph := 0.0
	for i := range n {
		f := fromHz + (toHz-fromHz)*float64(i)/float64(n)
		ph += 2 * math.Pi * f / Rate
		out[i] = int16(math.Sin(ph) * envelope(i, n) * gain * peak)
	}
	return out
}

// Noise is a short crunchy burst (a bite), the same every time.
func Noise(ms, gain float64) []int16 {
	n := samples(ms)
	out := make([]int16, n)
	r := rand.New(rand.NewPCG(1, 2))
	prev := 0.0
	for i := range n {
		prev = 0.6*prev + 0.4*(r.Float64()*2-1) // low-passed: less hiss
		out[i] = int16(prev * envelope(i, n) * gain * peak * 2)
	}
	return out
}

// Hz is a note's frequency: "A4" = 440, "C5", "F#4", "Bb3"; "-" or "R" is a rest (0).
func Hz(note string) (float64, error) {
	n := strings.TrimSpace(note)
	if n == "-" || strings.EqualFold(n, "r") {
		return 0, nil
	}
	if len(n) < 2 {
		return 0, fmt.Errorf("note %q: want a letter and an octave, e.g. C5", note)
	}
	semis := map[byte]int{'C': -9, 'D': -7, 'E': -5, 'F': -4, 'G': -2, 'A': 0, 'B': 2}
	s, ok := semis[strings.ToUpper(n[:1])[0]]
	if !ok {
		return 0, fmt.Errorf("note %q: unknown letter", note)
	}
	rest := n[1:]
	switch {
	case strings.HasPrefix(rest, "#"):
		s, rest = s+1, rest[1:]
	case strings.HasPrefix(rest, "b"):
		s, rest = s-1, rest[1:]
	}
	oct, err := strconv.Atoi(rest)
	if err != nil || oct < 0 || oct > 8 {
		return 0, fmt.Errorf("note %q: octave 0-8", note)
	}
	return 440 * math.Pow(2, float64(s+12*(oct-4))/12), nil
}

// Melody plays notes written as "C5:120 E5:120 G5:300 -:100 C6" (note:ms; 200 ms without one).
// bell: struck notes instead of soft tones. At most 30 s.
func Melody(notes string, gain float64, bell bool) ([]int16, error) {
	var out []int16
	total := 0.0
	for _, f := range strings.Fields(notes) {
		name, msText, hasMs := strings.Cut(f, ":")
		ms := 200.0
		if hasMs {
			v, err := strconv.ParseFloat(msText, 64)
			if err != nil || v <= 0 || v > 5000 {
				return nil, fmt.Errorf("%q: length 1-5000 ms", f)
			}
			ms = v
		}
		hz, err := Hz(name)
		if err != nil {
			return nil, err
		}
		if total += ms; total > 30000 {
			return nil, fmt.Errorf("longer than 30 s")
		}
		if bell {
			out = append(out, Bell(hz, ms, gain)...)
		} else {
			out = append(out, Tone(hz, ms, gain)...)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no notes")
	}
	return out, nil
}

// Chimes are ready-made sounds, by name.
var Chimes = map[string]func() []int16{
	// Focus's timer
	"start":      func() []int16 { return Bell(880, 90, 1) },
	"done":       func() []int16 { return bells([]float64{523.25, 659.25, 783.99, 1046.5}, []float64{180, 180, 180, 600}) },
	"break_over": func() []int16 { return bells([]float64{783.99, 0, 523.25}, []float64{260, 80, 600}) },
	// the pet's
	"chirp":   func() []int16 { return must("C6:70 E6:70 G6:110", 0.9) },
	"tada":    func() []int16 { return must("C5:110 E5:110 G5:110 C6:350", 1) },
	"no":      func() []int16 { return must("A4:180 -:40 G4:300", 0.9) },
	"hello":   func() []int16 { return must("G5:110 C6:180", 0.8) },
	"morning": func() []int16 { return must("G5:150 C6:150 E6:150 -:60 E6:120 G6:400", 0.8) },
	"lullaby": func() []int16 { return must("G5:350 E5:350 G5:350 E5:350 F5:300 D5:300 C5:700", 0.45) }, // slow and soft
	"yawn":    func() []int16 { return Glide(700, 300, 900, 0.6) },
	"whee":    func() []int16 { return Glide(400, 1400, 450, 0.7) },
	"munch": func() []int16 {
		var out []int16
		for range 3 {
			out = append(out, Noise(90, 0.5)...)
			out = append(out, Silence(110)...)
		}
		return out
	},
	// questions and answers
	"ask":   func() []int16 { return must("E5:90 A5:160", 0.7) },
	"yes":   func() []int16 { return must("C5:90 G5:180", 0.8) },
	"error": func() []int16 { return must("E4:160 C4:300", 0.8) },
}

func bells(hz, ms []float64) []int16 {
	var out []int16
	for i := range hz {
		out = append(out, Bell(hz[i], ms[i], 1)...)
	}
	return out
}

func must(notes string, gain float64) []int16 {
	pcm, err := Melody(notes, gain, false)
	if err != nil {
		panic(err)
	}
	return pcm
}

// Chime is a named chime, or nil.
func Chime(name string) []int16 {
	if f, ok := Chimes[name]; ok {
		return f()
	}
	return nil
}

// Message is the speaker message body (without the type byte): uint16 LE rate, then s16le PCM.
func Message(pcm []int16) []byte {
	b := make([]byte, 2+2*len(pcm))
	binary.LittleEndian.PutUint16(b, Rate)
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(b[2+2*i:], uint16(s))
	}
	return b
}

// Chunks splits a sound into speaker messages of at most MaxChunkMs each: send one, wait about
// its length, send the next (the robot buffers ~3 s).
func Chunks(pcm []int16) [][]byte {
	per := samples(MaxChunkMs)
	var out [][]byte
	for i := 0; i < len(pcm); i += per {
		out = append(out, Message(pcm[i:min(i+per, len(pcm))]))
	}
	return out
}

// Ms is a sound's length.
func Ms(pcm []int16) float64 { return float64(len(pcm)) * 1000 / Rate }

// WAV wraps PCM as a WAV file (16-bit mono at Rate), for the robot's file store: the robot plays
// a stored sound ("play") of any length, from its own storage.
func WAV(pcm []int16) []byte {
	data := 2 * len(pcm)
	b := make([]byte, 44+data)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+data))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)     // fmt chunk size
	binary.LittleEndian.PutUint16(b[20:], 1)      // PCM
	binary.LittleEndian.PutUint16(b[22:], 1)      // mono
	binary.LittleEndian.PutUint32(b[24:], Rate)   // sample rate
	binary.LittleEndian.PutUint32(b[28:], Rate*2) // byte rate
	binary.LittleEndian.PutUint16(b[32:], 2)      // block align
	binary.LittleEndian.PutUint16(b[34:], 16)     // bits
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(data))
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(b[44+2*i:], uint16(s))
	}
	return b
}
