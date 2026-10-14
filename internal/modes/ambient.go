package modes

import (
	"math"
	"time"

	"lampa/internal/analysis"
)

// Ambient: slow drift; tempo sets the speed, loudness the saturation.
type Ambient struct {
	clk  clock
	t    float64
	loud float64
}

func (m *Ambient) Name() string { return "Ambient" }
func (m *Ambient) Envelope() (time.Duration, time.Duration) {
	return 1500 * time.Millisecond, 1500 * time.Millisecond
}

func (m *Ambient) Update(f analysis.Features, c Context) Scene {
	dt := m.clk.dt(f.Time)
	speed := 0.02
	if f.BPM > 0 {
		speed *= f.BPM / 120
	}
	m.t += dt * speed
	m.loud += (f.Loudness - m.loud) * (1 - math.Exp(-dt/1.0))
	n := len(c.Lights)
	s := make(Scene, n)
	for i := range s {
		col := c.Palette.At(m.t + 0.3*float64(i)/float64(n))
		col.C *= 0.5 + 0.5*m.loud
		s[i] = Target{Color: col, Brightness: clamp01(0.35 + 0.4*m.loud)}
	}
	return s
}
