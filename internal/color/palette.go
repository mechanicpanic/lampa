package color

import "math"

// Palette is a cyclic gradient: At(1) == At(0).
type Palette struct {
	Name  string
	Stops []OKLCH
}

func (p Palette) At(t float64) OKLCH {
	n := len(p.Stops)
	if n == 0 {
		return OKLCH{L: 1}
	}
	t = t - math.Floor(t)
	x := t * float64(n)
	i := int(x) % n
	return Mix(p.Stops[i], p.Stops[(i+1)%n], x-math.Floor(x))
}

var Palettes = []Palette{
	{"sunset", []OKLCH{{0.70, 0.20, 30}, {0.75, 0.17, 60}, {0.62, 0.22, 350}, {0.55, 0.20, 310}}},
	{"ocean", []OKLCH{{0.65, 0.14, 230}, {0.75, 0.12, 195}, {0.55, 0.16, 260}, {0.80, 0.10, 170}}},
	{"neon", []OKLCH{{0.70, 0.30, 330}, {0.85, 0.20, 140}, {0.65, 0.28, 280}, {0.85, 0.17, 195}}},
	{"forest", []OKLCH{{0.65, 0.17, 140}, {0.80, 0.16, 110}, {0.55, 0.12, 160}, {0.75, 0.14, 80}}},
	{"warm", []OKLCH{{0.80, 0.10, 70}, {0.70, 0.14, 50}, {0.85, 0.07, 85}, {0.65, 0.15, 40}}},
}
