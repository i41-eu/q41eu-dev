package sound

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestHz(t *testing.T) {
	for note, want := range map[string]float64{"A4": 440, "A5": 880, "C5": 523.25, "F#4": 369.99, "Bb3": 233.08, "-": 0} {
		got, err := Hz(note)
		if err != nil || math.Abs(got-want) > 0.01 {
			t.Errorf("%s: %v %v, want %v", note, got, err, want)
		}
	}
	for _, bad := range []string{"H4", "C", "C9", "Cx"} {
		if _, err := Hz(bad); err == nil {
			t.Errorf("%s: want an error", bad)
		}
	}
}

func TestMelody(t *testing.T) {
	pcm, err := Melody("C5:100 -:50 E5", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := Ms(pcm); got != 350 {
		t.Fatalf("length %v ms, want 350", got)
	}
	if _, err := Melody("C5:100 X", 1, false); err == nil {
		t.Fatal("a bad note is an error")
	}
	if _, err := Melody("", 1, false); err == nil {
		t.Fatal("no notes is an error")
	}
}

func TestChimes(t *testing.T) {
	for name := range Chimes {
		pcm := Chime(name)
		if len(pcm) == 0 || Ms(pcm) > 3000 { // one message the robot buffers whole (~3 s)
			t.Errorf("%s: %v ms", name, Ms(pcm))
		}
		loud := 0
		for _, s := range pcm {
			loud = max(loud, int(math.Abs(float64(s))))
		}
		if loud == 0 || loud > math.MaxInt16/2 {
			t.Errorf("%s: peak %d", name, loud)
		}
	}
	if Chime("nope") != nil {
		t.Fatal("unknown chime")
	}
}

func TestChunks(t *testing.T) {
	pcm := Silence(6000)
	chunks := Chunks(pcm)
	if len(chunks) != 3 {
		t.Fatalf("%d chunks, want 3", len(chunks))
	}
	if binary.LittleEndian.Uint16(chunks[0]) != Rate || len(chunks[0]) != 2+2*Rate*MaxChunkMs/1000 {
		t.Fatalf("chunk 0: %d bytes", len(chunks[0]))
	}
}

func TestWAV(t *testing.T) {
	w := WAV(Silence(100))
	if string(w[:4]) != "RIFF" || string(w[8:12]) != "WAVE" || len(w) != 44+2*Rate/10 || binary.LittleEndian.Uint32(w[24:]) != Rate {
		t.Fatalf("not a 16 kHz WAV: %d bytes", len(w))
	}
}
