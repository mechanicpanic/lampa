package analysis

import (
	"math"
	"math/cmplx"

	"gonum.org/v1/gonum/dsp/fourier"
)

// Spectrum computes a Hann-windowed magnitude spectrum and log-spaced band powers.
type Spectrum struct {
	rate     int
	fft      *fourier.FFT
	window   []float64
	buf      []float64
	coeffs   []complex128
	Mag      []float64 // WindowSize/2+1 bins; full-scale sine peak ≈ 1
	bandBins [NumBands][2]int
	Centers  [NumBands]float64
}

func NewSpectrum(rate int) *Spectrum {
	s := &Spectrum{
		rate:   rate,
		fft:    fourier.NewFFT(WindowSize),
		window: make([]float64, WindowSize),
		buf:    make([]float64, WindowSize),
		Mag:    make([]float64, WindowSize/2+1),
	}
	for i := range s.window {
		s.window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(WindowSize))
	}
	ratio := MaxFreq / MinFreq
	for b := 0; b < NumBands; b++ {
		lo := MinFreq * math.Pow(ratio, float64(b)/NumBands)
		hi := MinFreq * math.Pow(ratio, float64(b+1)/NumBands)
		s.Centers[b] = math.Sqrt(lo * hi)
		kLo := int(math.Ceil(lo / s.binWidth()))
		kHi := int(math.Ceil(hi / s.binWidth()))
		if kHi > len(s.Mag) {
			kHi = len(s.Mag)
		}
		if kHi <= kLo { // band narrower than a bin: use the nearest bin
			k := int(math.Round(s.Centers[b] / s.binWidth()))
			kLo, kHi = k, k+1
		}
		s.bandBins[b] = [2]int{kLo, kHi}
	}
	return s
}

func (s *Spectrum) binWidth() float64     { return float64(s.rate) / WindowSize }
func (s *Spectrum) BinFreq(k int) float64 { return float64(k) * s.binWidth() }

// Compute analyses exactly WindowSize samples.
func (s *Spectrum) Compute(frame []float64) {
	for i, x := range frame {
		s.buf[i] = x * s.window[i]
	}
	s.coeffs = s.fft.Coefficients(s.coeffs, s.buf)
	for k, c := range s.coeffs {
		s.Mag[k] = cmplx.Abs(c) / (WindowSize / 4)
	}
}

// Bands writes the power (sum of squared magnitudes) of each band.
func (s *Spectrum) Bands(out *[NumBands]float64) {
	for b, r := range s.bandBins {
		p := 0.0
		for k := r[0]; k < r[1]; k++ {
			p += s.Mag[k] * s.Mag[k]
		}
		out[b] = p
	}
}
