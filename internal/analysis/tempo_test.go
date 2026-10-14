package analysis

import (
	"math"
	"slices"
	"testing"
	"time"
)

func beatTimes(fs []Features) []float64 {
	var ts []float64
	for _, f := range fs {
		if f.Beat {
			ts = append(ts, f.Time.Seconds())
		}
	}
	return ts
}

// beatErrors returns, for every true beat in [from,to], the signed error of
// the closest predicted beat (predicted - true - shift).
func beatErrors(pred, truth []float64, from, to, shift float64) []float64 {
	var errs []float64
	for _, tb := range truth {
		if tb < from || tb > to {
			continue
		}
		best := math.Inf(1)
		for _, p := range pred {
			if e := p - tb + shift; math.Abs(e) < math.Abs(best) {
				best = e
			}
		}
		errs = append(errs, best)
	}
	return errs
}

func checkMetronome(t *testing.T, r int, latency time.Duration) {
	t.Helper()
	sig, truth := metronome(120, 20, r)
	a := New(Config{SampleRate: r, Latency: latency})
	fs := pushAll(a, sig, 1024)
	last := fs[len(fs)-1]
	if math.Abs(last.BPM-120) > 2 {
		t.Fatalf("BPM %.2f, want 120±2", last.BPM)
	}
	if last.Confidence < 0.6 {
		t.Fatalf("confidence %.2f", last.Confidence)
	}
	errs := beatErrors(beatTimes(fs), truth, 8, 19, latency.Seconds())
	abs := make([]float64, len(errs))
	for i, e := range errs {
		abs[i] = math.Abs(e)
	}
	slices.Sort(abs)
	if med := abs[len(abs)/2]; med > 0.020 {
		t.Fatalf("median beat error %.1f ms (errors %v)", med*1000, errs)
	}
	if abs[len(abs)-1] > 0.035 {
		t.Fatalf("max beat error %.1f ms", abs[len(abs)-1]*1000)
	}
}

func TestMetronome48k(t *testing.T) { checkMetronome(t, 48000, 0) }
func TestMetronome44k(t *testing.T) { checkMetronome(t, 44100, 0) }
func TestLatencyShiftsBeatsEarlier(t *testing.T) {
	checkMetronome(t, 48000, 100*time.Millisecond)
}

func TestSilenceHasNoTempo(t *testing.T) {
	fs := pushAll(New(Config{SampleRate: rate}), silence(10, rate), 1024)
	last := fs[len(fs)-1]
	if last.BPM != 0 || last.Confidence != 0 || len(beatTimes(fs)) != 0 {
		t.Fatalf("silence: %+v, beats %d", last, len(beatTimes(fs)))
	}
}

func TestBuild(t *testing.T) {
	steady := pushAll(New(Config{SampleRate: rate}), sine(300, 0.3, 10, rate), 1024)
	if b := steady[len(steady)-1].Build; b > 0.1 {
		t.Fatalf("steady build %.2f", b)
	}
	// 10 s crescendo from -40 dB to 0 dB
	sig := sine(300, 1, 10, rate)
	for i := range sig {
		db := -40 + 40*float64(i)/float64(len(sig))
		sig[i] *= float32(math.Pow(10, db/20))
	}
	fs := pushAll(New(Config{SampleRate: rate}), sig, 1024)
	if b := fs[len(fs)-1].Build; b < 0.3 {
		t.Fatalf("crescendo build %.2f", b)
	}
}

// fourOnFloor: kick on every beat, open hat on every off-beat.
func fourOnFloor(bpm, start, seconds float64, rate int) ([]float32, []float64) {
	out := silence(seconds, rate)
	var beats []float64
	period := 60 / bpm
	for t := start; t < seconds; t += period {
		burst(out, int(t*float64(rate)), 55, 0.8, 0.15, rate)
		if h := t + period/2; h < seconds {
			burst(out, int(h*float64(rate)), 7000, 0.5, 0.09, rate)
		}
		beats = append(beats, t)
	}
	return out, beats
}

func TestBeatGridFollowsKicksNotOffbeatHats(t *testing.T) {
	for _, bpm := range []float64{122, 124, 128} {
		for _, start := range []float64{0.1, 0.23, 0.37, 0.41, 0.5} {
			sig, truth := fourOnFloor(bpm, start, 30, rate)
			fs := pushAll(New(Config{SampleRate: rate}), sig, 1024)
			errs := beatErrors(beatTimes(fs), truth, 15, 29, 0)
			abs := make([]float64, len(errs))
			for i, e := range errs {
				abs[i] = math.Abs(e)
			}
			slices.Sort(abs)
			if med := abs[len(abs)/2]; med > 0.030 {
				t.Errorf("bpm %.0f start %.2f: median error %.0f ms (grid on the off-beat?)", bpm, start, med*1000)
			}
		}
	}
}
