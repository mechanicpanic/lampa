package modes

import (
	"math"
	"time"

	"lampa/internal/analysis"
)

// Meter: a volume meter made of lamps. It shows how loud the music is
// *compared to the last couple of seconds*, not absolutely — auto-gained
// loudness sits near the top on any steady track, which kept the meter full.
// The track's average sits about half-way, louder moments and kicks throw it
// up, quieter phrases let it sink. Stacked lamps fill floor by floor from the
// bottom; a lamp on its own just brightens and dims.
type Meter struct {
	clk     clock
	avg     float64 // loudness averaged over ~meterAverage
	kick    float64 // extra push from a kick, fading out
	level   float64 // what is shown, 0..1
	started bool
}

const (
	meterAverage = 1500 * time.Millisecond
	meterKick    = 0.5 // how far a full-strength kick pushes the meter up
	meterKickTau = 250 * time.Millisecond
	meterFall    = 300 * time.Millisecond
)

func (m *Meter) Name() string                             { return "Meter" }
func (m *Meter) Envelope() (time.Duration, time.Duration) { return 0, 120 * time.Millisecond }

func (m *Meter) Update(f analysis.Features, c Context) Scene {
	dt := m.clk.dt(f.Time)
	if !m.started {
		m.avg, m.started = f.Loudness, true
	}
	m.avg += (f.Loudness - m.avg) * (1 - math.Exp(-dt/meterAverage.Seconds()))
	m.kick *= math.Exp(-dt / meterKickTau.Seconds())
	if f.OnsetLow.Hit {
		m.kick = math.Max(m.kick, meterKick*f.OnsetLow.Strength)
	}
	target := clamp01(0.5 + (f.Loudness-m.avg)*meterGain(c.Sensitivity) + m.kick)
	if target >= m.level {
		m.level = target // rise instantly
	} else {
		m.level += (target - m.level) * (1 - math.Exp(-dt/meterFall.Seconds()))
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

// meterGain: how many meters a unit of loudness swing is worth. Loudness
// spans 30 dB, so at the default sensitivity (0.5 → 10) a ±1.5 dB breath of
// the music already swings the meter from bottom to top; ←→ tunes it.
func meterGain(sensitivity float64) float64 {
	if sensitivity <= 0 {
		sensitivity = 0.5
	}
	return 4 + 12*sensitivity
}
