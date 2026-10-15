package tui

import (
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"lampa/internal/analysis"
	"lampa/internal/color"
	"lampa/internal/engine"
	"lampa/internal/i18n"
	"lampa/internal/light"
)

// TestShot renders the README screenshot as ANSI:
//
//	LAMPA_SHOT=docs/hero-dark.ansi LAMPA_SHOT_THEME=dark go test ./internal/tui -run TestShot
//
// then turn it into a picture with freeze (see docs/README-images.md).
func TestShot(t *testing.T) {
	path := os.Getenv("LAMPA_SHOT")
	if path == "" {
		t.Skip("set LAMPA_SHOT to render the README screenshot")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer i18n.Set(i18n.Current())
	i18n.Set(i18n.EN)

	// The terminal theme decides the chrome colors; pin them for the picture.
	theme := map[string][6]string{ // accent, soft, dim, ok, warn, border
		"dark":  {"#f5a3c7", "#c9a7f5", "#7f7a99", "#9fd8a6", "#f28b9b", "#4a4560"},
		"light": {"#c2457c", "#8a5cc7", "#9a94ad", "#3f8f53", "#c94a5b", "#d8d2e3"},
	}[os.Getenv("LAMPA_SHOT_THEME")]
	if theme[0] == "" {
		theme = [6]string{"#f5a3c7", "#c9a7f5", "#7f7a99", "#9fd8a6", "#f28b9b", "#4a4560"}
	}
	c := func(i int) lipgloss.Color { return lipgloss.Color(theme[i]) }
	accent = lipgloss.NewStyle().Foreground(c(0)).Bold(true)
	soft = lipgloss.NewStyle().Foreground(c(1))
	dim = lipgloss.NewStyle().Foreground(c(2))
	ok = lipgloss.NewStyle().Foreground(c(3))
	warn = lipgloss.NewStyle().Foreground(c(4))
	border = c(5)

	bands := []float64{.55, .82, .95, .88, .7, .52, .44, .5, .62, .7, .66, .58, .6, .68, .72, .64, .55, .5, .46, .4, .34, .28, .2, .12}
	v := engine.View{Mode: "Pulse", Palette: "sunset", Sensitivity: 0.55, Latency: 60 * time.Millisecond,
		AudioAlive: true, Features: analysis.Features{BPM: 124, Confidence: 0.9, BeatPhase: 0.02}}
	copy(v.Features.Bands[:], bands)
	lamp := func(id, name string, rgb color.RGB, b float64) engine.LampView {
		return engine.LampView{Info: light.Info{ID: id, Name: name, MaxFPS: 20}, Enabled: true, Color: rgb,
			Brightness: b, Stats: light.Stats{Status: light.StatusOK, FPS: 20}}
	}
	v.Lamps = []engine.LampView{
		lamp("a", "hero", color.RGB{R: 1, G: 0.42, B: 0.33}, 0.9),
		lamp("b", "ceiling 1", color.RGB{R: 0.98, G: 0.58, B: 0.12}, 0.6),
		lamp("c", "ceiling 2", color.RGB{R: 0.86, G: 0.24, B: 0.6}, 0.75),
	}
	ctl := &fakeCtl{view: v, track: "Yasuha — Fly-day Chinatown"}
	m := tea.Model(New(ctl))
	m, _ = press(m, tea.WindowSizeMsg{Width: 96, Height: 30})
	t0 := time.Now()
	for i := 0; i < 100; i++ { // fill the beat ribbon: ♥ every 15 columns, kicks and hats between
		for k := 0; k < 3; k++ {
			ctl.view.History = append(ctl.view.History, engine.Marker{
				Beat: i%15 == 3 && k == 0, Low: i%15 == 10 && k == 0, High: i%4 == 1 && k == 1})
		}
		ctl.view.Frames += 3
		m, _ = press(m, tickMsg(t0.Add(time.Duration(i)*33*time.Millisecond)))
	}
	if err := os.WriteFile(path, []byte(m.View()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
