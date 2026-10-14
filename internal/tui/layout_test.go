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
		v.Lamps = append(v.Lamps, engine.LampView{Info: light.Info{ID: fmt.Sprint(i), Name: fmt.Sprintf("лампа-%d", i)},
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
	if out := sized(richView(3), 30, 10); !strings.Contains(out, "маленькое") {
		t.Fatalf("tiny window:\n%s", out)
	}
}

func TestAllLampsVisibleWhenWrapped(t *testing.T) {
	out := sized(richView(7), 60, 40)
	for i := 0; i < 7; i++ {
		if !strings.Contains(out, fmt.Sprintf("лампа-%d", i)) {
			t.Fatalf("lamp %d missing:\n%s", i, out)
		}
	}
}

func TestShortWindowKeepsStatusAndAllLamps(t *testing.T) {
	out := sized(richView(3), 46, 16)
	if !strings.Contains(out, "палитра") {
		t.Fatalf("status line pushed out:\n%s", out)
	}
	for i := 0; i < 3; i++ {
		if !strings.Contains(out, fmt.Sprintf("лампа-%d", i)) {
			t.Fatalf("lamp %d pushed out:\n%s", i, out)
		}
	}
}

func TestHelpWrapsBeforeDroppingLabels(t *testing.T) {
	if out := sized(richView(3), 80, 30); !strings.Contains(out, "палитра") || !strings.Contains(out, "выход") {
		t.Fatalf("labels dropped although two lines fit:\n%s", out)
	}
}
