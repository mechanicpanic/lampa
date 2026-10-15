// Package light defines lamps and drives each one from its own goroutine.
package light

import (
	"context"
	"math"
)

type Kind string

const (
	KindWiZ      Kind = "wiz"
	KindYeelight Kind = "yeelight"
	KindVirtual  Kind = "virtual"
)

type Frame struct {
	R, G, B    uint8
	Brightness float64
}

type Info struct {
	ID, Name, Model, Role string
	Kind                  Kind
	MaxFPS                float64
}

type State struct{ Data map[string]any }

type Light interface {
	Info() Info
	Apply(ctx context.Context, f Frame) error
	Snapshot(ctx context.Context) (State, error)
	Restore(ctx context.Context, s State) error
	Close() error
}

type Status string

const (
	StatusOK         Status = "ok"
	StatusLost       Status = "lost"
	StatusConnecting Status = "connecting"
)

// Statuser is implemented by drivers that know their connection state.
type Statuser interface {
	Status() (Status, string)
}

const gamma = 2.0

// Split maps a perceptual frame onto a device with a minimum brightness level.
func Split(f Frame, floor float64) (r, g, b uint8, level float64) {
	p := math.Pow(math.Max(0, math.Min(1, f.Brightness)), gamma)
	r, g, b = vivid(f.R, f.G, f.B)
	level = p
	if p < floor {
		level = floor
		s := p / floor
		r = uint8(math.Round(float64(r) * s))
		g = uint8(math.Round(float64(g) * s))
		b = uint8(math.Round(float64(b) * s))
	}
	if r == 0 && g == 0 && b == 0 {
		switch {
		case f.R >= f.G && f.R >= f.B:
			r = 1
		case f.G >= f.B:
			g = 1
		default:
			b = 1
		}
	}
	return r, g, b, level
}

// whiteRemoval: share of the common r,g,b part dropped for lamps. Bulbs render
// that part as white light, so a screen green (78,167,57) looked white.
const whiteRemoval = 0.8

// vivid strips most of the white from a colored screen color, keeping its
// strongest channel. Near-whites and greys are left alone (smoothly).
func vivid(r, g, b uint8) (uint8, uint8, uint8) {
	hi := float64(max(r, g, b))
	lo := float64(min(r, g, b))
	if hi == 0 || hi == lo {
		return r, g, b
	}
	sat := (hi - lo) / hi
	t := math.Max(0, math.Min(1, (sat-0.05)/0.25))
	k := whiteRemoval * t * t * (3 - 2*t) // smoothstep: pastels barely change
	cut := k * lo
	scale := hi / (hi - cut)
	ch := func(c uint8) uint8 { return uint8(math.Round((float64(c) - cut) * scale)) }
	return ch(r), ch(g), ch(b)
}
