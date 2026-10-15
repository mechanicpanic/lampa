package color

import (
	"math"
	"testing"
	"time"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestRoundTrip(t *testing.T) {
	for _, c := range []RGB{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {1, 1, 1}, {0.2, 0.5, 0.7}} {
		got := FromRGB(c).ToRGB()
		if !near(got.R, c.R, 1e-3) || !near(got.G, c.G, 1e-3) || !near(got.B, c.B, 1e-3) {
			t.Fatalf("round trip %v -> %v", c, got)
		}
	}
}

func TestKnownValues(t *testing.T) {
	white := FromRGB(RGB{1, 1, 1})
	if !near(white.L, 1, 1e-3) || white.C > 1e-3 {
		t.Fatalf("white = %+v", white)
	}
	red := FromRGB(RGB{1, 0, 0})
	if !near(red.L, 0.628, 2e-3) || !near(red.C, 0.2577, 2e-3) || !near(red.H, 29.23, 0.5) {
		t.Fatalf("red = %+v", red)
	}
}

func TestOutOfGamutStaysInRange(t *testing.T) {
	c := OKLCH{L: 0.7, C: 0.5, H: 200}.ToRGB()
	for _, v := range []float64{c.R, c.G, c.B} {
		if v < 0 || v > 1 || math.IsNaN(v) {
			t.Fatalf("out of range: %+v", c)
		}
	}
}

func TestMixShortestHue(t *testing.T) {
	m := Mix(OKLCH{0.5, 0.1, 350}, OKLCH{0.5, 0.1, 10}, 0.5)
	if !(near(m.H, 0, 1e-9) || near(m.H, 360, 1e-9)) {
		t.Fatalf("hue = %v, want 0", m.H)
	}
}

func TestPaletteWraps(t *testing.T) {
	p := Palettes[0]
	a, b := p.At(0.25), p.At(1.25)
	if a != b {
		t.Fatalf("At(0.25)=%v At(1.25)=%v", a, b)
	}
	if p.At(0) != p.Stops[0] {
		t.Fatalf("At(0) should be first stop")
	}
}

func TestEnvelope(t *testing.T) {
	e := Envelope{Attack: 0, Release: 100 * time.Millisecond}
	if v := e.Step(1, 10*time.Millisecond); v != 1 {
		t.Fatalf("instant attack: %v", v)
	}
	v := e.Step(0, 100*time.Millisecond)
	if !near(v, math.Exp(-1), 1e-9) {
		t.Fatalf("release after tau: %v", v)
	}
}

func TestHex(t *testing.T) {
	if h := (RGB{1, 0.5, 0}).Hex(); h != "#ff8000" {
		t.Fatalf("hex %s", h)
	}
}

func TestPalettes(t *testing.T) {
	if len(Palettes) < 11 {
		t.Fatalf("only %d palettes", len(Palettes))
	}
	seen := map[string]bool{}
	for _, p := range Palettes {
		if seen[p.Name] {
			t.Fatalf("duplicate palette %q", p.Name)
		}
		seen[p.Name] = true
		if len(p.Stops) < 3 {
			t.Fatalf("%s: %d stops", p.Name, len(p.Stops))
		}
		for _, s := range p.Stops {
			if s.L < 0.45 || s.L > 0.92 || s.C < 0.06 {
				t.Fatalf("%s: stop %+v too dark, too pale or too grey for lamps", p.Name, s)
			}
		}
	}
}
