package analysis

import (
	"math"
	"slices"
	"time"
)

const (
	onsetGamma      = 20.0                    // log compression: log1p(gamma*mag)
	onsetMinFlux    = 1.0                     // absolute floor of the threshold
	onsetHistory    = 1500 * time.Millisecond // median window
	onsetRefractory = 80 * time.Millisecond
	lowCutHz        = 250.0
	highCutHz       = 1000.0
)

type fluxBand struct {
	lo, hi  int // bin range [lo,hi)
	hist    []float64
	pos     int
	filled  bool
	lastHit time.Duration
	scratch []float64
}

func (b *fluxBand) median() float64 {
	n := len(b.hist)
	if !b.filled {
		n = b.pos
	}
	if n == 0 {
		return 0
	}
	b.scratch = append(b.scratch[:0], b.hist[:n]...)
	slices.Sort(b.scratch)
	return b.scratch[n/2]
}

func (b *fluxBand) push(v float64) {
	b.hist[b.pos] = v
	b.pos++
	if b.pos == len(b.hist) {
		b.pos, b.filled = 0, true
	}
}

type onsetDetector struct {
	prev      []float64
	havePrev  bool
	low, high fluxBand
}

func newOnsetDetector(spec *Spectrum, rate int) *onsetDetector {
	hist := int(onsetHistory.Seconds() * float64(rate) / HopSize)
	bins := len(spec.Mag)
	lowHi := int(math.Ceil(lowCutHz / spec.binWidth()))
	highLo := int(math.Ceil(highCutHz / spec.binWidth()))
	return &onsetDetector{
		prev: make([]float64, bins),
		low:  fluxBand{lo: 1, hi: lowHi, hist: make([]float64, hist), lastHit: -time.Hour},
		high: fluxBand{lo: highLo, hi: bins, hist: make([]float64, hist), lastHit: -time.Hour},
	}
}

func (d *onsetDetector) step(mag []float64, now time.Duration, k float64) (low, high Onset, flux float64) {
	fluxOf := func(b *fluxBand) float64 {
		s := 0.0
		for i := b.lo; i < b.hi; i++ {
			if diff := math.Log1p(onsetGamma*mag[i]) - d.prev[i]; diff > 0 {
				s += diff
			}
		}
		return s
	}
	var fl, fh float64
	if d.havePrev {
		fl, fh = fluxOf(&d.low), fluxOf(&d.high)
	}
	for i, m := range mag {
		d.prev[i] = math.Log1p(onsetGamma * m)
	}
	d.havePrev = true
	return d.decide(&d.low, fl, now, k), d.decide(&d.high, fh, now, k), fl + fh
}

func (d *onsetDetector) decide(b *fluxBand, flux float64, now time.Duration, k float64) Onset {
	thr := b.median()*k + onsetMinFlux
	b.push(flux)
	if flux <= thr || now-b.lastHit < onsetRefractory {
		return Onset{}
	}
	b.lastHit = now
	return Onset{Hit: true, Strength: clamp01((flux - thr) / thr)}
}
