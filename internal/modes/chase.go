package modes

import (
	"time"

	"lampa/internal/analysis"
)

// Chase: the light jumps to the next lamp on every beat.
type Chase struct {
	pos, step int
}

func (m *Chase) Name() string                             { return "Chase" }
func (m *Chase) Envelope() (time.Duration, time.Duration) { return 0, 400 * time.Millisecond }

func (m *Chase) Update(f analysis.Features, c Context) Scene {
	n := len(c.Lights)
	if n == 0 {
		return Scene{}
	}
	if hit(f) {
		m.pos = (m.pos + 1) % n
		m.step++
	}
	s := make(Scene, n)
	t := float64(m.step) * 0.1
	for i := range s {
		if i == m.pos {
			s[i] = Target{Color: c.Palette.At(t), Brightness: 1}
		} else {
			s[i] = Target{Color: c.Palette.At(t - 0.1), Brightness: 0.06}
		}
	}
	return s
}
