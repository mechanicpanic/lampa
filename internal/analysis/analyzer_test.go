package analysis

import (
	"math"
	"testing"
)

const rate = 48000

func TestSpectrumSinePeak(t *testing.T) {
	s := NewSpectrum(rate)
	frame := make([]float64, WindowSize)
	for i := range frame {
		frame[i] = math.Sin(2 * math.Pi * 1000 * float64(i) / rate)
	}
	s.Compute(frame)
	best := 0
	for i := range s.Mag {
		if s.Mag[i] > s.Mag[best] {
			best = i
		}
	}
	if f := s.BinFreq(best); math.Abs(f-1000) > rate/WindowSize {
		t.Fatalf("peak at %.1f Hz", f)
	}
	if math.Abs(s.Mag[best]-1) > 0.2 {
		t.Fatalf("full-scale sine peak magnitude %.3f, want ≈1", s.Mag[best])
	}
}

func TestBandLayout(t *testing.T) {
	s := NewSpectrum(rate)
	if s.Centers[0] < MinFreq || s.Centers[NumBands-1] > MaxFreq {
		t.Fatalf("centers %v", s.Centers)
	}
	for i := 1; i < NumBands; i++ {
		if s.Centers[i] <= s.Centers[i-1] {
			t.Fatalf("centers not increasing at %d", i)
		}
	}
}

func TestFrameCountIndependentOfChunking(t *testing.T) {
	sig := sine(440, 0.5, 1, rate)
	n1 := len(pushAll(New(Config{SampleRate: rate}), sig, 4096))
	n2 := len(pushAll(New(Config{SampleRate: rate}), sig, 37))
	want := (len(sig)-WindowSize)/HopSize + 1
	if n1 != want || n2 != want {
		t.Fatalf("frames %d / %d, want %d", n1, n2, want)
	}
}

func TestBassSine(t *testing.T) {
	fs := pushAll(New(Config{SampleRate: rate}), sine(80, 0.5, 3, rate), 1024)
	g := fs[len(fs)-1].Groups
	if g.Bass < 0.5 || g.Mid > 0.1 || g.High > 0.1 {
		t.Fatalf("groups %+v", g)
	}
	if l := fs[len(fs)-1].Loudness; l < 0.8 {
		t.Fatalf("loudness %.2f", l)
	}
}

func TestSilence(t *testing.T) {
	fs := pushAll(New(Config{SampleRate: rate}), silence(2, rate), 1024)
	f := fs[len(fs)-1]
	if f.Loudness != 0 {
		t.Fatalf("loudness %v", f.Loudness)
	}
	for i, b := range f.Bands {
		if b != 0 {
			t.Fatalf("band %d = %v", i, b)
		}
	}
}

func TestTimeAdvances(t *testing.T) {
	fs := pushAll(New(Config{SampleRate: rate}), silence(1, rate), 1000)
	for i := 1; i < len(fs); i++ {
		if d := fs[i].Time - fs[i-1].Time; d <= 0 {
			t.Fatalf("time not increasing at %d: %v", i, d)
		}
	}
}

func checkFinite(t *testing.T, f Features) {
	t.Helper()
	vals := []float64{f.Loudness, f.Groups.Sub, f.Groups.Bass, f.Groups.LowMid, f.Groups.Mid, f.Groups.High,
		f.OnsetLow.Strength, f.OnsetHigh.Strength, f.BPM, f.BeatPhase, f.BarPhase, f.Confidence, f.Build}
	vals = append(vals, f.Bands[:]...)
	for i, v := range vals {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("non-finite value #%d in %+v", i, f)
		}
	}
}

func TestAnalyzerSanitizesSamples(t *testing.T) {
	sig := sine(200, 0.5, 2, rate)
	for i := 0; i < len(sig); i += 997 {
		switch (i / 997) % 3 {
		case 0:
			sig[i] = float32(math.NaN())
		case 1:
			sig[i] = float32(math.Inf(1))
		default:
			sig[i] = 5
		}
	}
	for _, f := range pushAll(New(Config{SampleRate: rate}), sig, 512) {
		checkFinite(t, f)
	}
}
