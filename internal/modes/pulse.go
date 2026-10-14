package modes

import (
	"time"

	"lampa/internal/analysis"
)

// Pulse: every lamp flashes on the beat; the palette steps forward each bar.
type Pulse struct{ hue float64 }

func (p *Pulse) Name() string                             { return "Pulse" }
func (p *Pulse) Envelope() (time.Duration, time.Duration) { return 0, 250 * time.Millisecond }

func (p *Pulse) Update(f analysis.Features, c Context) Scene {
	h := hit(f)
	if h {
		if f.Confidence < minConfidence {
			p.hue += 0.03
		} else if f.BarPhase < 0.125 {
			p.hue += 0.125
		}
	}
	s := make(Scene, len(c.Lights))
	base := 0.12 + 0.25*f.Loudness
	for i := range s {
		b := base
		if h {
			b = 1
		}
		s[i] = Target{Color: c.Palette.At(p.hue + 0.06*float64(i)), Brightness: clamp01(b)}
	}
	return s
}
