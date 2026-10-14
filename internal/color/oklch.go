// Package color does perceptual color math (OKLCH) and smoothing for lamps.
package color

import (
	"fmt"
	"math"
)

// RGB is gamma-encoded sRGB, components 0..1.
type RGB struct{ R, G, B float64 }

// OKLCH: L 0..1, C >= 0 (≈0.37 max in sRGB), H in degrees [0,360).
type OKLCH struct{ L, C, H float64 }

func clamp01(x float64) float64 {
	if math.IsNaN(x) || x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func (c RGB) Bytes() (r, g, b uint8) {
	q := func(x float64) uint8 { return uint8(math.Round(clamp01(x) * 255)) }
	return q(c.R), q(c.G), q(c.B)
}

func (c RGB) Hex() string {
	r, g, b := c.Bytes()
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func toLinear(x float64) float64 {
	if x <= 0.04045 {
		return x / 12.92
	}
	return math.Pow((x+0.055)/1.055, 2.4)
}

func toGamma(x float64) float64 {
	if x <= 0.0031308 {
		return 12.92 * x
	}
	return 1.055*math.Pow(x, 1/2.4) - 0.055
}

func FromRGB(c RGB) OKLCH {
	r, g, b := toLinear(c.R), toLinear(c.G), toLinear(c.B)
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	L := 0.2104542553*l + 0.7936177850*m - 0.0040720468*s
	A := 1.9779984951*l - 2.4285922050*m + 0.4505937099*s
	B := 0.0259040371*l + 0.7827717662*m - 0.8086757660*s
	h := math.Atan2(B, A) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return OKLCH{L: L, C: math.Hypot(A, B), H: h}
}

// linear returns linear-light sRGB, possibly out of [0,1].
func (c OKLCH) linear() (r, g, b float64) {
	h := c.H * math.Pi / 180
	A, B := c.C*math.Cos(h), c.C*math.Sin(h)
	l := c.L + 0.3963377774*A + 0.2158037573*B
	m := c.L - 0.1055613458*A - 0.0638541728*B
	s := c.L - 0.0894841775*A - 1.2914855480*B
	l, m, s = l*l*l, m*m*m, s*s*s
	r = 4.0767416621*l - 3.3077115913*m + 0.2309699292*s
	g = -1.2684380046*l + 2.6097574011*m - 0.3413193965*s
	b = -0.0041960863*l - 0.7034186147*m + 1.7076147010*s
	return
}

func inGamut(r, g, b float64) bool {
	const eps = 1e-6
	return r >= -eps && r <= 1+eps && g >= -eps && g <= 1+eps && b >= -eps && b <= 1+eps
}

// ToRGB converts to sRGB, lowering chroma (keeping L and H) until in gamut.
func (c OKLCH) ToRGB() RGB {
	c.L = clamp01(c.L)
	if c.C < 0 || math.IsNaN(c.C) {
		c.C = 0
	}
	r, g, b := c.linear()
	if !inGamut(r, g, b) {
		lo, hi := 0.0, c.C
		for i := 0; i < 20; i++ {
			mid := (lo + hi) / 2
			if rr, gg, bb := (OKLCH{c.L, mid, c.H}).linear(); inGamut(rr, gg, bb) {
				lo = mid
			} else {
				hi = mid
			}
		}
		r, g, b = (OKLCH{c.L, lo, c.H}).linear()
	}
	return RGB{toGamma(clamp01(r)), toGamma(clamp01(g)), toGamma(clamp01(b))}
}

// Mix interpolates L, C linearly and H along the shortest arc.
func Mix(a, b OKLCH, t float64) OKLCH {
	dh := math.Mod(b.H-a.H+540, 360) - 180
	h := math.Mod(a.H+dh*t+360, 360)
	return OKLCH{L: a.L + (b.L-a.L)*t, C: a.C + (b.C-a.C)*t, H: h}
}
