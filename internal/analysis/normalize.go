package analysis

import (
	"math"
	"time"
)

const (
	floorDB  = -80.0
	rangeDB  = 30.0
	decayTau = 6 * time.Second
)

// Normalizer maps a power to 0..1 relative to a slowly decaying peak,
// so quiet and loud tracks both use the full range.
type Normalizer struct {
	peak float64 // linear power
}

func toDB(p float64) float64 { return 10 * math.Log10(p+1e-20) }

// PeakDB is the current ceiling in dB (never below floorDB+rangeDB).
func (n *Normalizer) PeakDB() float64 { return math.Max(toDB(n.peak), floorDB+rangeDB) }

// Step updates the peak and returns the normalized value. minPeakDB lets the
// caller raise the ceiling (e.g. relative to the loudest band) so leakage in
// an otherwise empty band does not get amplified to full scale.
func (n *Normalizer) Step(power float64, dt time.Duration, minPeakDB float64) float64 {
	n.peak = math.Max(power, n.peak*math.Exp(-float64(dt)/float64(decayTau)))
	x := toDB(power)
	if x < floorDB {
		return 0
	}
	peak := math.Max(n.PeakDB(), minPeakDB)
	return clamp01(1 - (peak-x)/rangeDB)
}

func clamp01(x float64) float64 {
	if math.IsNaN(x) || x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
