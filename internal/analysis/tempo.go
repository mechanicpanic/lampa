package analysis

import (
	"math"
	"time"
)

const (
	tempoWindow   = 6 * time.Second
	tempoMinBPM   = 70.0
	tempoMaxBPM   = 180.0
	tempoPriorBPM = 120.0
	tempoPriorOct = 0.9 // width of the log2 prior, octaves
	pllGain       = 0.25
	// onsetBias corrects the systematic offset between when the detector
	// fires and the true onset (frame time is the window centre).
	onsetBias = 0 * time.Millisecond
)

type tempoTracker struct {
	fps    float64
	env    []float64
	pos    int
	filled bool
	frames int
	period time.Duration
	conf   float64
	next   time.Duration
	beats  int
	lastOn time.Duration
}

func newTempoTracker(fps float64) *tempoTracker {
	return &tempoTracker{fps: fps, env: make([]float64, int(tempoWindow.Seconds()*fps))}
}

func (t *tempoTracker) ordered() []float64 {
	if !t.filled {
		return t.env[:t.pos]
	}
	return append(append([]float64{}, t.env[t.pos:]...), t.env[:t.pos]...)
}

// estimate updates period and confidence from the onset envelope.
func (t *tempoTracker) estimate() {
	x := t.ordered()
	if len(x) < int(4*t.fps) {
		return
	}
	mean := 0.0
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))
	y := make([]float64, len(x))
	acf0 := 0.0
	for i, v := range x {
		y[i] = v - mean
		acf0 += y[i] * y[i]
	}
	if acf0 < 1e-9 {
		t.period, t.conf = 0, 0
		return
	}
	lagMin := int(math.Floor(60 * t.fps / tempoMaxBPM))
	lagMax := int(math.Ceil(60 * t.fps / tempoMinBPM))
	r := make([]float64, lagMax+2)
	for lag := lagMin - 1; lag <= lagMax+1; lag++ {
		s := 0.0
		for i := 0; i+lag < len(y); i++ {
			s += y[i] * y[i+lag]
		}
		r[lag] = s / acf0 * float64(len(y)) / float64(len(y)-lag)
	}
	best, bestScore, sum := 0, math.Inf(-1), 0.0
	for lag := lagMin; lag <= lagMax; lag++ {
		bpm := 60 * t.fps / float64(lag)
		w := math.Exp(-0.5 * math.Pow(math.Log2(bpm/tempoPriorBPM)/tempoPriorOct, 2))
		if s := r[lag] * w; s > bestScore {
			best, bestScore = lag, s
		}
		sum += r[lag]
	}
	avg := sum / float64(lagMax-lagMin+1)
	// parabolic refinement
	lag := float64(best)
	if a, b, c := r[best-1], r[best], r[best+1]; a-2*b+c < 0 {
		lag += 0.5 * (a - c) / (a - 2*b + c)
	}
	t.conf = clamp01((r[best] - avg) * 2)
	p := time.Duration(lag / t.fps * float64(time.Second))
	if t.period == 0 || math.Abs(float64(p-t.period))/float64(t.period) > 0.05 {
		t.period = p
		t.next = t.lastOn + p
	} else {
		t.period = (t.period*3 + p) / 4
	}
}

func (t *tempoTracker) step(now time.Duration, flux float64, onset bool, latency time.Duration) (bpm, phase, bar, conf float64, beat bool) {
	t.env[t.pos] = flux
	t.pos++
	if t.pos == len(t.env) {
		t.pos, t.filled = 0, true
	}
	t.frames++
	if t.frames%int(t.fps/2) == 0 {
		t.estimate()
	}
	if onset {
		t.lastOn = now - onsetBias
	}
	if t.period == 0 || t.conf == 0 {
		return 0, 0, 0, t.conf, false
	}
	if onset {
		on := now - onsetBias
		e1, e2 := on-(t.next-t.period), on-t.next
		e := e1
		if absDur(e2) < absDur(e1) {
			e = e2
		}
		if absDur(e) < t.period/4 {
			t.next += time.Duration(pllGain * float64(e))
		}
	}
	ahead := now + latency
	for ahead >= t.next {
		beat = true
		t.beats++
		t.next += t.period
	}
	phase = clamp01(1 - float64(t.next-ahead)/float64(t.period))
	bar = (float64(t.beats%4) + phase) / 4
	return 60 / t.period.Seconds(), phase, bar, t.conf, beat
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// buildMeter: how much louder the last ~1 s is than the last ~8 s (6 dB → 1).
type buildMeter struct {
	fast, slow float64
	init       bool
}

func (b *buildMeter) step(powerDB float64, dt time.Duration) float64 {
	powerDB = math.Max(powerDB, floorDB)
	if !b.init {
		b.fast, b.slow, b.init = powerDB, powerDB, true
	}
	b.fast += (powerDB - b.fast) * (1 - math.Exp(-dt.Seconds()/1.0))
	b.slow += (powerDB - b.slow) * (1 - math.Exp(-dt.Seconds()/8.0))
	return clamp01((b.fast - b.slow) / 6)
}
