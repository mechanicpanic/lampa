package modes

import (
	"math"
	"time"

	"lampa/internal/analysis"
)

// Meter: loudness fills stacked lamps from the bottom up, like a VU meter;
// a lamp on its own simply follows the loudness.
type Meter struct {
	clk   clock
	level float64
}

const meterFall = 300 * time.Millisecond

func (m *Meter) Name() string                             { return "Meter" }
func (m *Meter) Envelope() (time.Duration, time.Duration) { return 0, 120 * time.Millisecond }

func (m *Meter) Update(f analysis.Features, c Context) Scene {
	dt := m.clk.dt(f.Time)
	if f.Loudness >= m.level {
		m.level = f.Loudness // rise instantly
	} else {
		m.level += (f.Loudness - m.level) * (1 - math.Exp(-dt/meterFall.Seconds()))
	}
	s := make(Scene, len(c.Lights))
	for i, p := range stacks(c) {
		b := m.level
		if p.n > 0 {
			b = m.level*float64(p.n) - float64(p.pos) // fill floor by floor
		}
		s[i] = Target{Color: c.Palette.At(0.6 * p.height()), Brightness: clamp01(b)}
	}
	return s
}
