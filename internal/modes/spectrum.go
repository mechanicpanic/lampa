package modes

import (
	"time"

	"lampa/internal/analysis"
)

var groupNames = []string{"sub", "bass", "lowmid", "mid", "high"}

// default assignment for lamps without a role: bass, mid, high, bass, …
var defaultGroups = []int{1, 3, 4}

func groupValue(g analysis.Groups, i int) float64 {
	return [...]float64{g.Sub, g.Bass, g.LowMid, g.Mid, g.High}[i]
}

// Spectrum: each lamp follows one frequency group.
type Spectrum struct {
	clk clock
	hue float64
}

func (m *Spectrum) Name() string                             { return "Spectrum" }
func (m *Spectrum) Envelope() (time.Duration, time.Duration) { return 0, 150 * time.Millisecond }

func (m *Spectrum) Update(f analysis.Features, c Context) Scene {
	m.hue += m.clk.dt(f.Time) * 0.01
	s := make(Scene, len(c.Lights))
	st := stacks(c)
	for i, l := range c.Lights {
		g := defaultGroups[i%len(defaultGroups)]
		if st[i].n > 0 {
			g = st[i].verticalBand() // stacked: bass at the bottom, highs on top
		}
		for j, n := range groupNames {
			if l.Role == n {
				g = j
			}
		}
		s[i] = Target{
			Color:      c.Palette.At(m.hue + float64(g)/float64(len(groupNames))),
			Brightness: clamp01(groupValue(f.Groups, g)),
		}
	}
	return s
}
