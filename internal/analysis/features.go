// Package analysis turns a mono audio stream into per-hop musical features.
package analysis

import "time"

const (
	WindowSize = 2048
	HopSize    = 512
	NumBands   = 24
	MinFreq    = 30.0
	MaxFreq    = 16000.0
)

type Groups struct{ Sub, Bass, LowMid, Mid, High float64 }

type Onset struct {
	Hit      bool
	Strength float64 // 0..1, how far above threshold
}

// Features describes the music at Time (centre of the analysis window).
// All floats are 0..1 unless noted.
type Features struct {
	Time       time.Duration
	Bands      [NumBands]float64
	Groups     Groups
	Loudness   float64
	OnsetLow   Onset   // kick / bass (< 250 Hz)
	OnsetHigh  Onset   // snare, hats, vocals (> 1 kHz)
	BPM        float64 // 0 when unknown
	BeatPhase  float64 // 0 at the (latency-compensated) beat
	Beat       bool    // a beat is due now (already shifted by latency)
	BarPhase   float64 // position in a 4-beat bar
	Confidence float64 // tempo confidence
	Build      float64 // energy rising over the last ~8 s
}
