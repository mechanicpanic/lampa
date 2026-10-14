package modes

import (
	"time"

	"lampa/internal/analysis"
	"lampa/internal/color"
)

const (
	flashLen = 150 * time.Millisecond
	glowLen  = 4 * time.Second
)

// Drop: tension accumulates during a build; a strong kick after it fires
// a white flash and a burst of color.
type Drop struct {
	clk        clock
	charge     float64
	hue        float64
	flashUntil time.Duration
	glowUntil  time.Duration
}

func (m *Drop) Name() string                             { return "Drop" }
func (m *Drop) Envelope() (time.Duration, time.Duration) { return 0, 300 * time.Millisecond }

func (m *Drop) Update(f analysis.Features, c Context) Scene {
	dt := m.clk.dt(f.Time)
	if f.Build > 0.2 {
		m.charge += f.Build * dt * 0.4
	} else {
		m.charge -= dt * 0.2
	}
	m.charge = clamp01(m.charge)
	if m.charge > 0.5 && f.OnsetLow.Hit && f.OnsetLow.Strength > 0.3 {
		m.flashUntil, m.glowUntil = f.Time+flashLen, f.Time+glowLen
		m.hue += 0.37
		m.charge = 0
	}
	s := make(Scene, len(c.Lights))
	for i := range s {
		switch {
		case f.Time < m.flashUntil:
			s[i] = Target{Color: color.OKLCH{L: 1}, Brightness: 1}
		case f.Time < m.glowUntil:
			b := 0.5
			if hit(f) {
				b = 1
			}
			s[i] = Target{Color: c.Palette.At(m.hue + 0.1*float64(i)), Brightness: b}
		default:
			col := c.Palette.At(m.hue + 0.05*float64(i))
			col.C *= 0.3 + 0.7*m.charge
			b := 0.15 + 0.6*m.charge
			if m.charge > 0.3 && f.OnsetHigh.Hit {
				b += 0.2
			}
			s[i] = Target{Color: col, Brightness: clamp01(b)}
		}
	}
	return s
}
