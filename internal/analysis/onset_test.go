package analysis

import "testing"

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
