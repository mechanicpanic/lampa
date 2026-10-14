package analysis

import (
	"math"
	"time"
)

// A band may not be boosted more than this far below the loudest band's peak.
const bandHeadroomDB = 24.0

type Config struct {
	SampleRate  int
	Sensitivity float64       // 0..1, default 0.5
	Latency     time.Duration // beats are predicted this far ahead
}

type Analyzer struct {
	cfg     Config
	spec    *Spectrum
	buf     []float64
	pending int
	total   int64 // samples consumed
	dt      time.Duration
	frame   []float64

	power    [NumBands]float64
	bandNorm [NumBands]Normalizer
	loudNorm Normalizer
	onset    *onsetDetector
	flux     float64       // combined novelty of the last frame
	groupOf  [NumBands]int // 0 sub, 1 bass, 2 lowmid, 3 mid, 4 high
	groupN   [5]int
}

func New(cfg Config) *Analyzer {
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 48000
	}
	if cfg.Sensitivity == 0 {
		cfg.Sensitivity = 0.5
	}
	a := &Analyzer{
		cfg:   cfg,
		spec:  NewSpectrum(cfg.SampleRate),
		dt:    time.Duration(float64(HopSize) / float64(cfg.SampleRate) * float64(time.Second)),
		frame: make([]float64, WindowSize),
	}
	a.onset = newOnsetDetector(a.spec, cfg.SampleRate)
	for b, c := range a.spec.Centers {
		g := 4
		switch {
		case c < 60:
			g = 0
		case c < 250:
			g = 1
		case c < 1000:
			g = 2
		case c < 4000:
			g = 3
		}
		a.groupOf[b] = g
		a.groupN[g]++
	}
	return a
}

func (a *Analyzer) Rate() int                  { return a.cfg.SampleRate }
func (a *Analyzer) SetSensitivity(s float64)   { a.cfg.Sensitivity = clamp01(s) }
func (a *Analyzer) SetLatency(d time.Duration) { a.cfg.Latency = d }

func sanitize(x float32) float64 {
	v := float64(x)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Max(-1, math.Min(1, v))
}

// Push consumes samples and returns one Features per completed hop.
func (a *Analyzer) Push(samples []float32) []Features {
	var out []Features
	for _, x := range samples {
		a.buf = append(a.buf, sanitize(x))
		a.total++
		a.pending++
		if len(a.buf) >= WindowSize && a.pending >= HopSize {
			copy(a.frame, a.buf[len(a.buf)-WindowSize:])
			out = append(out, a.process())
			a.pending = 0
			if len(a.buf) > 4*WindowSize {
				a.buf = append(a.buf[:0], a.buf[len(a.buf)-WindowSize:]...)
			}
		}
	}
	return out
}

func (a *Analyzer) now() time.Duration {
	center := float64(a.total) - WindowSize/2
	return time.Duration(center / float64(a.cfg.SampleRate) * float64(time.Second))
}

func (a *Analyzer) process() Features {
	f := Features{Time: a.now()}
	a.spec.Compute(a.frame)
	a.computeBands(&f)
	k := 1 + (1 - a.cfg.Sensitivity)
	f.OnsetLow, f.OnsetHigh, a.flux = a.onset.step(a.spec.Mag, f.Time, k)
	return f
}

func (a *Analyzer) computeBands(f *Features) {
	a.spec.Bands(&a.power)
	loudest := floorDB
	total := 0.0
	for b := range a.bandNorm {
		loudest = math.Max(loudest, a.bandNorm[b].PeakDB())
		total += a.power[b]
	}
	var sums [5]float64
	for b := range a.bandNorm {
		v := a.bandNorm[b].Step(a.power[b], a.dt, loudest-bandHeadroomDB)
		f.Bands[b] = v
		sums[a.groupOf[b]] += v
	}
	avg := func(g int) float64 {
		if a.groupN[g] == 0 {
			return 0
		}
		return sums[g] / float64(a.groupN[g])
	}
	f.Groups = Groups{Sub: avg(0), Bass: avg(1), LowMid: avg(2), Mid: avg(3), High: avg(4)}
	f.Loudness = a.loudNorm.Step(total, a.dt, floorDB+rangeDB)
}
