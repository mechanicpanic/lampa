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
	// estimates (every 0.5 s) a new tempo must win in a row to take over
	tempoSwitchVotes = 4
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
	// low-band (kick) novelty, same ring layout as env; anchors the phase
	low       []float64
	lastLowOn time.Duration
	haveLowOn bool
	// a challenger tempo and how many estimates in a row it has won
	cand  time.Duration
	candN int
}

func newTempoTracker(fps float64) *tempoTracker {
	n := int(tempoWindow.Seconds() * fps)
	return &tempoTracker{fps: fps, env: make([]float64, n), low: make([]float64, n)}
}

func (t *tempoTracker) ordered() []float64 { return t.chrono(t.env) }

// chrono returns a ring buffer (sharing pos/filled) oldest first.
func (t *tempoTracker) chrono(ring []float64) []float64 {
	if !t.filled {
		return ring[:t.pos]
	}
	return append(append([]float64{}, ring[t.pos:]...), ring[:t.pos]...)
}

// estimate updates period and confidence from the onset envelope.
func (t *tempoTracker) estimate(now time.Duration) {
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
	lag := float64(best) + refineLag(r[best-1], r[best], r[best+1])
	p := time.Duration(lag / t.fps * float64(time.Second))
	switch {
	case t.period == 0:
		t.adopt(p)
	case relDiff(p, t.period) <= 0.05:
		t.period = (t.period*3 + p) / 4
		t.cand, t.candN = 0, 0
	default:
		// A different tempo must win several estimates in a row: music
		// without drums offers beat, half- and double-time almost equally,
		// and switching on every estimate made the tempo jump around.
		if t.cand != 0 && relDiff(p, t.cand) <= 0.05 {
			t.candN++
		} else {
			t.cand, t.candN = p, 1
		}
		votes := tempoSwitchVotes
		if metricallyRelated(p, t.period) {
			votes *= 2 // half/double/dotted time of the same music: be stubborn
		}
		if t.candN >= votes {
			t.adopt(p)
		}
	}
	// Confidence is about the tempo we actually follow.
	cur := int(math.Round(t.period.Seconds() * t.fps))
	if cur >= lagMin && cur <= lagMax {
		t.conf = clamp01((r[cur] - avg) * 2)
	} else {
		t.conf = 0
	}
	t.alignPhase(now)
}

func (t *tempoTracker) adopt(p time.Duration) {
	t.period, t.next = p, t.anchor()+p
	t.cand, t.candN = 0, 0
}

func relDiff(a, b time.Duration) float64 {
	return math.Abs(float64(a-b)) / float64(b)
}

// anchor is the latest onset to hang the beat grid on: kicks when the
// track has them, any onset otherwise.
func (t *tempoTracker) anchor() time.Duration {
	if t.haveLowOn {
		return t.lastLowOn
	}
	return t.lastOn
}

// alignPhase picks the grid offset whose beats carry the most low-band
// energy (comb over the kick envelope). The PLL only corrects small errors,
// so without this a grid started on an off-beat hat would stay there.
func (t *tempoTracker) alignPhase(now time.Duration) {
	x := t.chrono(t.low)
	n := len(x)
	L := t.period.Seconds() * t.fps
	if L < 2 || float64(n) < 2*L {
		return
	}
	score := func(phi float64) float64 {
		s := 0.0
		for k := 0.0; ; k++ {
			i := n - 1 - int(math.Round(phi+k*L))
			if i < 1 {
				return s
			}
			v := max(x[i], x[i-1])
			if i+1 < n {
				v = max(v, x[i+1])
			}
			s += v
		}
	}
	best, bestS := 0.0, -1.0
	for phi := 0.0; phi < L; phi++ {
		if s := score(phi); s > bestS {
			best, bestS = phi, s
		}
	}
	if anti := score(math.Mod(best+L/2, L)); bestS <= 1e-9 || bestS < 1.2*anti {
		return // no kicks, or no clear winner between beat and off-beat
	}
	nn := now - time.Duration(best/t.fps*float64(time.Second)) + t.period
	for nn <= now {
		nn += t.period
	}
	d := time.Duration(math.Mod(float64(nn-t.next), float64(t.period)))
	if d >= t.period/2 {
		d -= t.period
	} else if d < -t.period/2 {
		d += t.period
	}
	if absDur(d) > t.period/4 {
		t.next += d
	}
}

func (t *tempoTracker) step(now time.Duration, flux, lowFlux float64, onset, lowOnset bool, latency time.Duration) (bpm, phase, bar, conf float64, beat bool) {
	t.env[t.pos] = flux
	t.low[t.pos] = lowFlux
	t.pos++
	if t.pos == len(t.env) {
		t.pos, t.filled = 0, true
	}
	t.frames++
	if t.frames%int(t.fps/2) == 0 {
		t.estimate(now)
	}
	if onset {
		t.lastOn = now - onsetBias
	}
	if lowOnset {
		t.lastLowOn, t.haveLowOn = now-onsetBias, true
	}
	// With kicks present, only kicks steer the grid; off-beat hats don't.
	if t.haveLowOn && now-t.lastLowOn < 2*time.Second {
		onset = lowOnset
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

// refineLag: parabolic sub-bin offset of a peak at b with neighbours a, c.
// Only meaningful when b is a true local maximum; otherwise stay on the bin.
func refineLag(a, b, c float64) float64 {
	if b < a || b < c || a-2*b+c >= 0 {
		return 0
	}
	return max(-0.5, min(0.5, 0.5*(a-c)/(a-2*b+c)))
}

// metricallyRelated: a is b at another metrical level (×2, ×3/2, ×4/3 or inverse).
func metricallyRelated(a, b time.Duration) bool {
	r := float64(a) / float64(b)
	for _, k := range []float64{2, 1.5, 4.0 / 3} {
		if math.Abs(r/k-1) < 0.04 || math.Abs(r*k-1) < 0.04 {
			return true
		}
	}
	return false
}
