package analysis

import (
	"math"
	"math/rand"
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

func TestRefineLagStaysNearItsBin(t *testing.T) {
	// The prior-weighted winner need not be the raw peak: a neighbour can be
	// higher, which made the parabola fly off (265 BPM on Bach).
	for _, c := range [][3]float64{{1.05, 1, 0.94}, {0.94, 1, 1.05}, {1, 1, 1}, {0.2, 1, 0.3}, {1, 0.5, 1}} {
		if d := refineLag(c[0], c[1], c[2]); d < -0.5 || d > 0.5 || math.IsNaN(d) {
			t.Fatalf("refineLag%v = %v", c, d)
		}
	}
	if d := refineLag(0.5, 1, 0.5); d != 0 {
		t.Fatalf("symmetric peak shifted by %v", d)
	}
}

// bachLike: soft sixteenths at `bpm`, the first of every four a bit louder,
// ±3% rubato, a sustained chord underneath; no percussion.
func bachLike(bpm, seconds float64, rate int) []float32 {
	sig := silence(seconds, rate)
	for _, f := range []float64{131, 196, 262} {
		pianoNote(sig, 0, f, 0.015, seconds*3, rate)
	}
	r := rand.New(rand.NewSource(7))
	notes := []float64{523, 659, 784, 1047, 880, 698, 587, 494, 440, 587, 698, 880}
	step := 60 / bpm / 4
	for i, s := 0, 0.3; s < seconds-0.5; i++ {
		amp := 0.03
		if i%4 == 0 {
			amp = 0.045
		}
		pianoNote(sig, int(s*float64(rate)), notes[i%len(notes)], amp, 1.2, rate)
		s += step * (1 + 0.03*(2*r.Float64()-1))
	}
	return sig
}

func TestTempoStaysPutWithoutDrums(t *testing.T) {
	fs := pushAll(New(Config{SampleRate: rate}), bachLike(80, 40, rate), 1024)
	jumps, shown, last := 0, 0, 0.0
	for _, f := range fs {
		if f.Time.Seconds() < 10 || f.Confidence < 0.4 || f.BPM == 0 {
			continue // the UI hides an unsure tempo
		}
		shown++
		if last != 0 && math.Abs(f.BPM-last)/last > 0.08 {
			jumps++
		}
		last = f.BPM
		if f.BPM < tempoMinBPM-1 || f.BPM > tempoMaxBPM+1 {
			t.Fatalf("BPM %.1f out of range", f.BPM)
		}
	}
	if jumps > 1 {
		t.Fatalf("shown tempo jumped %d times (%d frames shown)", jumps, shown)
	}
}

func TestRealTempoChangeIsFollowed(t *testing.T) {
	a, _ := metronome(100, 15, rate)
	b, _ := metronome(130, 15, rate)
	fs := pushAll(New(Config{SampleRate: rate}), append(a, b...), 1024)
	for _, f := range fs {
		if f.Time.Seconds() > 23 && math.Abs(f.BPM-130) > 3 {
			t.Fatalf("at %.1f s still %.1f BPM, want 130 (changed at 15 s)", f.Time.Seconds(), f.BPM)
		}
	}
}
