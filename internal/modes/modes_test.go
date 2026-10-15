package modes

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"lampa/internal/analysis"
	"lampa/internal/color"
	"lampa/internal/light"
)

const frame = 10 * time.Millisecond

func ctxN(n int) Context {
	c := Context{Palette: color.Palettes[0]}
	for i := 0; i < n; i++ {
		c.Lights = append(c.Lights, light.Info{ID: string(rune('a' + i))})
	}
	return c
}

func TestModesZeroLights(t *testing.T) {
	for _, m := range All() {
		f := analysis.Features{Beat: true, Confidence: 1, OnsetLow: analysis.Onset{Hit: true, Strength: 1}}
		for i := 0; i < 5; i++ {
			f.Time = time.Duration(i) * frame
			if s := m.Update(f, ctxN(0)); len(s) != 0 {
				t.Fatalf("%s: scene %v for zero lights", m.Name(), s)
			}
		}
	}
}

func TestSceneLengthAndRanges(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, m := range All() {
		for i := 0; i < 500; i++ {
			f := analysis.Features{Time: time.Duration(i) * frame, Loudness: r.Float64(), Build: r.Float64(),
				BPM: 120, Confidence: r.Float64(), Beat: r.Intn(40) == 0, BarPhase: r.Float64(),
				OnsetLow: analysis.Onset{Hit: r.Intn(30) == 0, Strength: r.Float64()},
				Groups:   analysis.Groups{Bass: r.Float64(), Mid: r.Float64(), High: r.Float64()}}
			s := m.Update(f, ctxN(3))
			if len(s) != 3 {
				t.Fatalf("%s: len %d", m.Name(), len(s))
			}
			for _, tg := range s {
				if tg.Brightness < 0 || tg.Brightness > 1 || math.IsNaN(tg.Brightness) || math.IsNaN(tg.Color.H) {
					t.Fatalf("%s: bad target %+v", m.Name(), tg)
				}
			}
		}
	}
}

func TestIndex(t *testing.T) {
	if Index("chase") != 2 || Index("DROP") != 4 || Index("nope") != 0 {
		t.Fatal("Index lookup broken")
	}
}

func TestPulseFlashesOnBeat(t *testing.T) {
	m := &Pulse{}
	s := m.Update(analysis.Features{Beat: true, Confidence: 1}, ctxN(2))
	if s[0].Brightness != 1 || s[1].Brightness != 1 {
		t.Fatalf("beat %v", s)
	}
	s = m.Update(analysis.Features{Time: frame, Confidence: 1}, ctxN(2))
	if s[0].Brightness > 0.5 {
		t.Fatalf("between beats %v", s)
	}
}

func TestPulseUsesOnsetsWhenTempoUnsure(t *testing.T) {
	m := &Pulse{}
	s := m.Update(analysis.Features{Beat: true, Confidence: 0.1}, ctxN(1))
	if s[0].Brightness == 1 {
		t.Fatal("low-confidence beat must be ignored")
	}
	s = m.Update(analysis.Features{OnsetLow: analysis.Onset{Hit: true, Strength: 1}, Confidence: 0.1}, ctxN(1))
	if s[0].Brightness != 1 {
		t.Fatal("onset should flash when tempo is unsure")
	}
}

func TestChaseOneBrightLampPerBeat(t *testing.T) {
	m := &Chase{}
	seen := map[int]bool{}
	for i := 0; i < 8; i++ {
		s := m.Update(analysis.Features{Time: time.Duration(i) * 500 * time.Millisecond, Beat: true, Confidence: 1}, ctxN(4))
		bright := -1
		for j, tg := range s {
			if tg.Brightness >= 0.9 {
				if bright != -1 {
					t.Fatalf("two bright lamps: %v", s)
				}
				bright = j
			} else if tg.Brightness > 0.1 {
				t.Fatalf("dim lamp too bright: %v", s)
			}
		}
		if bright == -1 {
			t.Fatalf("no bright lamp on beat %d", i)
		}
		seen[bright] = true
	}
	if len(seen) != 4 {
		t.Fatalf("chase visited %v", seen)
	}
}

func TestSpectrumRoles(t *testing.T) {
	m := &Spectrum{}
	c := ctxN(2)
	c.Lights[0].Role = "high"
	c.Lights[1].Role = "bass"
	s := m.Update(analysis.Features{Groups: analysis.Groups{Bass: 0.9, High: 0.1}}, c)
	if s[0].Brightness > 0.2 || s[1].Brightness < 0.8 {
		t.Fatalf("roles ignored: %v", s)
	}
}

func TestAmbientHasNoJumps(t *testing.T) {
	m := &Ambient{}
	r := rand.New(rand.NewSource(2))
	var prev Scene
	for i := 0; i < 3000; i++ {
		f := analysis.Features{Time: time.Duration(i) * frame, Loudness: r.Float64(), BPM: 60 + r.Float64()*120,
			Beat: r.Intn(20) == 0, OnsetLow: analysis.Onset{Hit: r.Intn(10) == 0, Strength: 1}}
		s := m.Update(f, ctxN(3))
		if prev != nil {
			for j := range s {
				dh := math.Abs(math.Mod(s[j].Color.H-prev[j].Color.H+540, 360) - 180)
				if dh > 1 || math.Abs(s[j].Color.C-prev[j].Color.C) > 0.01 || math.Abs(s[j].Brightness-prev[j].Brightness) > 0.02 {
					t.Fatalf("jump at frame %d lamp %d: %+v -> %+v", i, j, prev[j], s[j])
				}
			}
		}
		prev = s
	}
}

func TestDropFlashesAfterBuild(t *testing.T) {
	kick := analysis.Onset{Hit: true, Strength: 0.9}
	m := &Drop{}
	now := time.Duration(0)
	for ; now < 6*time.Second; now += frame {
		m.Update(analysis.Features{Time: now, Build: 0.8, Loudness: 0.5}, ctxN(2))
	}
	s := m.Update(analysis.Features{Time: now, OnsetLow: kick, Loudness: 1}, ctxN(2))
	for _, tg := range s {
		if tg.Brightness != 1 || tg.Color.C != 0 || tg.Color.L != 1 {
			t.Fatalf("no white flash on drop: %v", s)
		}
	}
	calm := &Drop{}
	s = calm.Update(analysis.Features{Time: 0, OnsetLow: kick, Loudness: 1}, ctxN(2))
	if s[0].Color.C == 0 && s[0].Brightness == 1 {
		t.Fatal("flash without a build")
	}
}

// ikea: three stacked lamps listed in a scrambled order (top, bottom, middle).
func ikea() Context {
	c := Context{Palette: color.Palettes[0]}
	for _, lv := range []int{3, 1, 2} {
		c.Lights = append(c.Lights, light.Info{ID: fmt.Sprint(lv), Group: "ikea", Level: lv})
	}
	return c
}

const top, bottom, middle = 0, 1, 2 // indexes into ikea().Lights

func TestSpectrumStackedBassAtTheBottom(t *testing.T) {
	m := &Spectrum{}
	s := m.Update(analysis.Features{Groups: analysis.Groups{Bass: 0.9, Mid: 0.1, High: 0.5}}, ikea())
	if s[bottom].Brightness != 0.9 || s[middle].Brightness != 0.1 || s[top].Brightness != 0.5 {
		t.Fatalf("bottom/middle/top = %.1f/%.1f/%.1f, want bass/mid/high 0.9/0.1/0.5",
			s[bottom].Brightness, s[middle].Brightness, s[top].Brightness)
	}
}

func TestPulseRisesLikeAFountain(t *testing.T) {
	m := &Pulse{}
	beat := analysis.Features{Beat: true, Confidence: 1}
	s := m.Update(beat, ikea())
	if s[bottom].Brightness != 1 || s[middle].Brightness == 1 || s[top].Brightness == 1 {
		t.Fatalf("on the beat only the bottom should flash: %v", s)
	}
	lit := map[int]time.Duration{}
	for ms := 10; ms <= 300; ms += 10 {
		f := analysis.Features{Time: time.Duration(ms) * time.Millisecond, Confidence: 1}
		for i, tg := range m.Update(f, ikea()) {
			if _, seen := lit[i]; !seen && tg.Brightness == 1 {
				lit[i] = f.Time
			}
		}
	}
	if !(lit[middle] > 0 && lit[top] > lit[middle]) {
		t.Fatalf("flash should climb bottom → middle → top, lit at %v", lit)
	}
	single := m.Update(beat, ctxN(2))
	if single[0].Brightness != 1 || single[1].Brightness != 1 {
		t.Fatal("ungrouped lamps still flash together on the beat")
	}
}

func TestMeterBreathesAroundTheAverage(t *testing.T) {
	m := &Meter{}
	var s Scene
	now := time.Duration(0)
	feed := func(loud float64, kick bool, d time.Duration) {
		for end := now + d; now < end; now += 10 * time.Millisecond {
			f := analysis.Features{Time: now, Loudness: loud}
			if kick {
				f.OnsetLow = analysis.Onset{Hit: true, Strength: 1}
				kick = false
			}
			s = m.Update(f, ikea())
		}
	}
	feed(0.95, false, 4*time.Second) // loud but steady: should sit mid-way, not fill up
	if s[bottom].Brightness < 0.9 || s[top].Brightness > 0.2 {
		t.Fatalf("steady music: bottom/middle/top = %.2f/%.2f/%.2f, want about half full",
			s[bottom].Brightness, s[middle].Brightness, s[top].Brightness)
	}
	feed(1.0, true, 20*time.Millisecond) // a kick on a louder moment
	if s[top].Brightness < 0.8 {
		t.Fatalf("a kick should throw the meter to the top: %.2f", s[top].Brightness)
	}
	feed(0.95, false, 2*time.Second)
	feed(0.75, false, 600*time.Millisecond) // a quieter phrase
	if s[middle].Brightness > 0.1 || s[top].Brightness > 0 {
		t.Fatalf("quiet phrase should drop below the middle: %.2f/%.2f/%.2f",
			s[bottom].Brightness, s[middle].Brightness, s[top].Brightness)
	}
	if Index("meter") != 5 || len(All()) != 6 {
		t.Fatal("Meter must be mode 6")
	}
}
