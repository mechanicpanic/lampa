package modes

import (
	"time"

	"lampa/internal/analysis"
)

// Pulse: every lamp flashes on the beat; the palette steps forward each bar.
// Stacked lamps flash bottom-up, one floor after another, like a fountain.
type Pulse struct {
	hue     float64
	lastHit time.Duration
	hitSeen bool
}

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
	if h {
		p.lastHit, p.hitSeen = f.Time, true
	}
	s := make(Scene, len(c.Lights))
	st := stacks(c)
	base := 0.12 + 0.25*f.Loudness
	since := f.Time - p.lastHit
	for i := range s {
		b := base
		delay := time.Duration(st[i].pos) * fountainStep
		switch {
		case delay == 0 && h: // bottom floor / single lamps: exactly on the beat
			b = 1
		case delay > 0 && p.hitSeen && since >= delay && since < delay+fountainWindow:
			b = 1 // higher floors follow, one after another
		}
		s[i] = Target{Color: c.Palette.At(p.hue + 0.06*float64(i)), Brightness: clamp01(b)}
	}
	return s
}

// fountainWindow: how long a floor's flash trigger stays open (≥ 2 analysis frames).
const fountainWindow = 25 * time.Millisecond
