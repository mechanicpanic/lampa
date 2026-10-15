package analysis

import (
	"math"
	"testing"
)

func countHits(fs []Features, low bool) int {
	n := 0
	for _, f := range fs {
		if (low && f.OnsetLow.Hit) || (!low && f.OnsetHigh.Hit) {
			n++
		}
	}
	return n
}

func TestHatsAreHighOnsetsOnly(t *testing.T) {
	sig := silence(6, rate)
	n := 0
	for s := 0.5; s < 5.8; s += 0.4 {
		burst(sig, int(s*rate), 8000, 0.5, 0.02, rate)
		n++
	}
	fs := pushAll(New(Config{SampleRate: rate}), sig, 512)
	if h := countHits(fs, false); h < n-1 || h > n+1 {
		t.Fatalf("high hits %d, want ≈%d", h, n)
	}
	if l := countHits(fs, true); l != 0 {
		t.Fatalf("low hits %d, want 0", l)
	}
}

func TestKicksAreLowOnsets(t *testing.T) {
	sig := silence(6, rate)
	n := 0
	for s := 0.5; s < 5.8; s += 0.5 {
		burst(sig, int(s*rate), 60, 0.8, 0.12, rate)
		n++
	}
	fs := pushAll(New(Config{SampleRate: rate}), sig, 512)
	if l := countHits(fs, true); l < n-1 || l > n+1 {
		t.Fatalf("low hits %d, want ≈%d", l, n)
	}
}

func TestNoOnsetsInSteadyTone(t *testing.T) {
	fs := pushAll(New(Config{SampleRate: rate}), sine(440, 0.5, 4, rate), 512)
	for _, f := range fs {
		if f.Time.Seconds() > 0.5 && (f.OnsetLow.Hit || f.OnsetHigh.Hit) {
			t.Fatalf("onset in steady tone at %v", f.Time)
		}
	}
}

func TestNoOnsetsInSilence(t *testing.T) {
	fs := pushAll(New(Config{SampleRate: rate}), silence(3, rate), 512)
	if countHits(fs, true)+countHits(fs, false) != 0 {
		t.Fatal("onsets in silence")
	}
}

// pianoNote: a soft tone with a 10 ms attack, slow decay and a few
// harmonics — no percussive click.
func pianoNote(dst []float32, start int, freq, amp, dur float64, rate int) {
	n := int(dur * float64(rate))
	for i := 0; i < n && start+i < len(dst); i++ {
		t := float64(i) / float64(rate)
		env := math.Min(1, t/0.010) * math.Exp(-t/(dur/3))
		v := 0.0
		for h, w := range []float64{1, 0.5, 0.3, 0.15} {
			v += w * math.Sin(2*math.Pi*freq*float64(h+1)*t)
		}
		dst[start+i] += float32(amp * env * v)
	}
}

func TestSoftPianoNotesAreOnsets(t *testing.T) {
	// Bach-like texture: quiet sixteenths ringing into each other over a
	// sustained chord (pedal/reverb), no drums.
	sig := silence(12, rate)
	for _, f := range []float64{262, 330, 392} {
		pianoNote(sig, 0, f, 0.02, 40, rate)
	}
	notes := []float64{523, 659, 784, 1047, 784, 659, 587, 698, 880, 698, 587, 494}
	n := 0
	for i, s := 0, 0.5; s < 11.5; i, s = i+1, s+0.25 {
		pianoNote(sig, int(s*rate), notes[i%len(notes)], 0.04, 1.5, rate)
		n++
	}
	fs := pushAll(New(Config{SampleRate: rate}), sig, 512)
	if h := countHits(fs, false); h < n*7/10 {
		t.Fatalf("only %d of %d soft notes detected", h, n)
	}
}
