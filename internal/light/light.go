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
	r, g, b = f.R, f.G, f.B
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
