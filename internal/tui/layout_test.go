package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lampa/internal/analysis"
	"lampa/internal/color"
	"lampa/internal/engine"
	"lampa/internal/light"
)

func richView(lamps int) engine.View {
	v := engine.View{Mode: "Pulse", Palette: "sunset", Sensitivity: 0.5, AudioAlive: true,
		Features: analysis.Features{BPM: 124, Confidence: 1}}
	for i := range v.Features.Bands {
		v.Features.Bands[i] = 1
	}
	for i := 0; i < engine.HistoryLen; i++ {
		v.History = append(v.History, engine.Marker{Beat: i%47 == 0, Low: i%23 == 0, High: i%7 == 0})
	}
	for i := 0; i < lamps; i++ {
		v.Lamps = append(v.Lamps, engine.LampView{Info: light.Info{ID: fmt.Sprint(i), Name: fmt.Sprintf("lamp-%d", i)},
			Enabled: i != 1, Color: color.RGB{R: 1, G: 0.5}, Brightness: 0.7,
			Stats: light.Stats{Status: light.StatusOK, FPS: 20}})
	}
	return v
}

func sized(v engine.View, w, h int) string {
	m := tea.Model(New(&fakeCtl{view: v}))
	m, _ = press(m, tea.WindowSizeMsg{Width: w, Height: h}, tickMsg(time.Now()))
	return m.View()
}

func TestFitsWindow(t *testing.T) {
	for _, sz := range [][2]int{{40, 12}, {41, 13}, {60, 20}, {80, 24}, {100, 30}, {137, 41}, {200, 50}} {
		w, h := sz[0], sz[1]
		for _, n := range []int{0, 3, 7} {
			lines := strings.Split(sized(richView(n), w, h), "\n")
			if len(lines) > h {
				t.Fatalf("%dx%d, %d lamps: %d lines", w, h, n, len(lines))
			}
			for i, l := range lines {
				if lw := lipgloss.Width(l); lw > w {
					t.Fatalf("%dx%d, %d lamps: line %d is %d wide: %q", w, h, n, i, lw, l)
				}
			}
			if lw := lipgloss.Width(lines[0]); lw != w {
				t.Fatalf("%dx%d: frame is %d wide, should fill the window", w, h, lw)
			}
		}
	}
}

func TestSpectrumFillsWidth(t *testing.T) {
	for _, w := range []int{40, 77, 100, 163} {
		out := sized(richView(3), w, 30)
		best := 0
		for _, l := range strings.Split(out, "\n") {
			best = max(best, strings.Count(l, "█"))
		}
		if best != w-4 {
			t.Fatalf("width %d: fullest spectrum row has %d blocks, want %d", w, best, w-4)
		}
	}
}

func TestTooSmall(t *testing.T) {
	if out := sized(richView(3), 30, 10); !strings.Contains(out, "too small") {
		t.Fatalf("tiny window:\n%s", out)
	}
}

func TestAllLampsVisibleWhenWrapped(t *testing.T) {
	out := sized(richView(7), 60, 40)
	for i := 0; i < 7; i++ {
		if !strings.Contains(out, fmt.Sprintf("lamp-%d", i)) {
			t.Fatalf("lamp %d missing:\n%s", i, out)
		}
	}
}

func TestShortWindowKeepsStatusAndAllLamps(t *testing.T) {
	out := sized(richView(3), 46, 16)
	if !strings.Contains(out, "palette") {
		t.Fatalf("status line pushed out:\n%s", out)
	}
	for i := 0; i < 3; i++ {
		if !strings.Contains(out, fmt.Sprintf("lamp-%d", i)) {
			t.Fatalf("lamp %d pushed out:\n%s", i, out)
		}
	}
}

func TestHelpWrapsBeforeDroppingLabels(t *testing.T) {
	if out := sized(richView(3), 80, 30); !strings.Contains(out, "palette") || !strings.Contains(out, "quit") {
		t.Fatalf("labels dropped although two lines fit:\n%s", out)
	}
}

func fullestRow(out string) int {
	best := 0
	for _, l := range strings.Split(out, "\n") {
		best = max(best, strings.Count(l, "█"))
	}
	return best
}

func TestSpectrumFallsSmoothly(t *testing.T) {
	c := &fakeCtl{view: richView(0)}
	m := tea.Model(New(c))
	t0 := time.Now()
	m, _ = press(m, tea.WindowSizeMsg{Width: 80, Height: 30}, tickMsg(t0))
	for i := range c.view.Features.Bands {
		c.view.Features.Bands[i] = 0 // music suddenly drops out
	}
	m, _ = press(m, tickMsg(t0.Add(33*time.Millisecond)))
	if fullestRow(m.View()) == 0 {
		t.Fatal("spectrum vanished in one frame — should fall smoothly")
	}
	for i := 2; i < 40; i++ {
		m, _ = press(m, tickMsg(t0.Add(time.Duration(i)*33*time.Millisecond)))
	}
	if n := fullestRow(m.View()); n != 0 {
		t.Fatalf("spectrum still up after 1.3 s of silence (%d blocks)", n)
	}
	for i := range c.view.Features.Bands {
		c.view.Features.Bands[i] = 1
	}
	m, _ = press(m, tickMsg(t0.Add(41*33*time.Millisecond)))
	if fullestRow(m.View()) != 76 {
		t.Fatal("rise must be instant")
	}
}

func TestFPSDoesNotFlicker(t *testing.T) {
	c := &fakeCtl{view: richView(1)}
	c.view.Lamps[0].Info.MaxFPS = 20
	m := tea.Model(New(c))
	t0 := time.Now()
	m, _ = press(m, tea.WindowSizeMsg{Width: 80, Height: 30})
	for i := 0; i < 30; i++ {
		c.view.Lamps[0].Stats.FPS = float64(19 + i%2) // the 1 s window lands on 19 or 20
		m, _ = press(m, tickMsg(t0.Add(time.Duration(i)*33*time.Millisecond)))
		if out := m.View(); !strings.Contains(out, "20 fps") {
			t.Fatalf("tick %d: fps display flickered:\n%s", i, out)
		}
	}
}

func TestUnsureTempoIsNotShown(t *testing.T) {
	v := richView(1)
	v.Features.BPM, v.Features.Confidence = 153, 0.2
	if out := sized(v, 80, 24); strings.Contains(out, "153") || !strings.Contains(out, "— bpm") {
		t.Fatalf("unsure tempo shown:\n%s", out)
	}
	v.Features.Confidence = 0.8
	if out := sized(v, 80, 24); !strings.Contains(out, "153 BPM") {
		t.Fatalf("confident tempo hidden:\n%s", out)
	}
}

func TestHeartFadesOverTheBeat(t *testing.T) {
	prev := heartLevel(0)
	if prev != heartLevels-1 {
		t.Fatalf("heart not at full on the beat: %d", prev)
	}
	seen := map[int]bool{prev: true}
	for p := 0.0; p < 1; p += 0.01 {
		l := heartLevel(p)
		if l > prev {
			t.Fatalf("heart brightened mid-beat at phase %.2f", p)
		}
		if prev-l > 1 {
			t.Fatalf("heart skipped a step at phase %.2f (%d → %d)", p, prev, l)
		}
		prev = l
		seen[l] = true
	}
	if len(seen) != heartLevels || heartLevel(0.99) != 0 {
		t.Fatalf("fade should pass through all %d steps, saw %v", heartLevels, seen)
	}
}
