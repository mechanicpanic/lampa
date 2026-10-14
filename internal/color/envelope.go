package color

import (
	"math"
	"time"
)

// Envelope follows a target with separate rise (Attack) and fall (Release)
// time constants; zero means instant.
type Envelope struct {
	Attack, Release time.Duration
	v               float64
}

func (e *Envelope) Value() float64 { return e.v }

func (e *Envelope) Step(target float64, dt time.Duration) float64 {
	tau := e.Release
	if target > e.v {
		tau = e.Attack
	}
	if tau <= 0 {
		e.v = target
	} else {
		e.v += (target - e.v) * (1 - math.Exp(-float64(dt)/float64(tau)))
	}
	return e.v
}
