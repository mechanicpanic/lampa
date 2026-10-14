package analysis

import "math"

func sine(freq, amp, seconds float64, rate int) []float32 {
	n := int(seconds * float64(rate))
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(amp * math.Sin(2*math.Pi*freq*float64(i)/float64(rate)))
	}
	return out
}

func silence(seconds float64, rate int) []float32 {
	return make([]float32, int(seconds*float64(rate)))
}

// burst adds a decaying sine burst of length dur starting at sample start.
func burst(dst []float32, start int, freq, amp, dur float64, rate int) {
	n := int(dur * float64(rate))
	for i := 0; i < n && start+i < len(dst); i++ {
		t := float64(i) / float64(rate)
		env := math.Exp(-t / (dur / 4))
		dst[start+i] += float32(amp * env * math.Sin(2*math.Pi*freq*t))
	}
}

// metronome: kick (60 Hz, 120 ms) on every beat for the given duration.
// Returns the samples and the true beat times in seconds.
func metronome(bpm, seconds float64, rate int) ([]float32, []float64) {
	out := silence(seconds, rate)
	var beats []float64
	period := 60 / bpm
	for t := 0.5; t < seconds; t += period {
		burst(out, int(t*float64(rate)), 60, 0.8, 0.12, rate)
		burst(out, int(t*float64(rate)), 3000, 0.2, 0.02, rate)
		beats = append(beats, t)
	}
	return out, beats
}

func pushAll(a *Analyzer, s []float32, chunk int) []Features {
	var fs []Features
	for i := 0; i < len(s); i += chunk {
		j := min(i+chunk, len(s))
		fs = append(fs, a.Push(s[i:j])...)
	}
	return fs
}
