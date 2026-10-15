// Package modes turns musical features into per-lamp colors.
package modes

import (
	"math"
	"strings"
	"time"

	"lampa/internal/analysis"
	"lampa/internal/color"
	"lampa/internal/light"
)

type Target struct {
	Color      color.OKLCH
	Brightness float64
}

type Scene []Target

type Context struct {
	Palette color.Palette
	Lights  []light.Info
}

type Mode interface {
	Name() string
	Update(f analysis.Features, c Context) Scene
	Envelope() (attack, release time.Duration)
}

func All() []Mode {
	return []Mode{&Pulse{}, &Spectrum{}, &Chase{}, &Ambient{}, &Drop{}, &Meter{}}
}

func Index(name string) int {
	for i, m := range All() {
		if strings.EqualFold(m.Name(), name) {
			return i
		}
	}
	return 0
}

const minConfidence = 0.4

// hit is the rhythmic trigger: beats when the tempo is trusted, kicks otherwise.
func hit(f analysis.Features) bool {
	if f.Confidence >= minConfidence {
		return f.Beat
	}
	return f.OnsetLow.Hit
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

// clock turns feature timestamps into a bounded frame delta.
type clock struct {
	last    time.Duration
	started bool
}

func (c *clock) dt(now time.Duration) float64 {
	if !c.started {
		c.started, c.last = true, now
		return 0
	}
	d := (now - c.last).Seconds()
	c.last = now
	return math.Max(0, math.Min(0.1, d))
}
