package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"lampa/internal/analysis"
	"lampa/internal/audio"
	"lampa/internal/engine"
	"lampa/internal/light"
)

type fakeCtl struct {
	calls  []string
	source *audio.App
	view   engine.View
}

func (f *fakeCtl) View() engine.View { return f.view }
func (f *fakeCtl) SetMode(i int)     { f.calls = append(f.calls, "mode"+string(rune('0'+i))) }
func (f *fakeCtl) NextPalette()      { f.calls = append(f.calls, "palette") }
func (f *fakeCtl) AdjustSensitivity(d float64) {
	if d > 0 {
		f.calls = append(f.calls, "sens+")
	} else {
		f.calls = append(f.calls, "sens-")
	}
}
func (f *fakeCtl) AdjustLatency(d time.Duration) {
	if d == 10*time.Millisecond {
		f.calls = append(f.calls, "lat+")
	} else if d == -10*time.Millisecond {
		f.calls = append(f.calls, "lat-")
	}
}
func (f *fakeCtl) TogglePause() { f.calls = append(f.calls, "pause") }
func (f *fakeCtl) Sources(context.Context) ([]audio.App, error) {
	return []audio.App{{PID: 42, Name: "Spotify"}}, nil
}
func (f *fakeCtl) SetSource(a *audio.App) { f.source = a; f.calls = append(f.calls, "source") }
func (f *fakeCtl) SourceName() string     { return "вся система" }
func (f *fakeCtl) SetLampEnabled(id string, on bool) {
	f.calls = append(f.calls, fmt.Sprintf("lamp %s %v", id, on))
}
func (f *fakeCtl) Discover(context.Context) (int, error) {
	f.calls = append(f.calls, "discover")
	return 2, nil
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func press(m tea.Model, msgs ...tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, msg := range msgs {
		m, cmd = m.Update(msg)
	}
	return m, cmd
}

func TestKeys(t *testing.T) {
	c := &fakeCtl{}
	m := tea.Model(New(c))
	m, _ = press(m, runes("3"), runes("p"), tea.KeyMsg{Type: tea.KeyRight}, tea.KeyMsg{Type: tea.KeyLeft},
		tea.KeyMsg{Type: tea.KeyUp}, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeySpace})
	want := "mode2 palette sens+ sens- lat+ lat- pause"
	if got := strings.Join(c.calls, " "); got != want {
		t.Fatalf("calls %q, want %q", got, want)
	}
	_, cmd := press(m, runes("q"))
	if cmd == nil {
		t.Fatal("q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q did not quit")
	}
}

func TestSourcePicker(t *testing.T) {
	c := &fakeCtl{}
	m := tea.Model(New(c))
	m, cmd := press(m, runes("s"))
	m, _ = press(m, cmd()) // deliver the apps list
	if !strings.Contains(m.View(), "Spotify") {
		t.Fatalf("picker not shown:\n%s", m.View())
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyEnter})
	if c.source == nil || c.source.PID != 42 {
		t.Fatalf("source %+v", c.source)
	}
	if strings.Contains(m.View(), "Spotify") && strings.Contains(m.View(), "enter") {
		t.Fatal("picker still open")
	}
}

func TestDiscover(t *testing.T) {
	c := &fakeCtl{}
	m := tea.Model(New(c))
	m, cmd := press(m, runes("d"))
	m, _ = press(m, cmd())
	if !strings.Contains(m.View(), "2") || c.calls[len(c.calls)-1] != "discover" {
		t.Fatalf("discover result not shown:\n%s", m.View())
	}
}

func TestViewRenders(t *testing.T) {
	c := &fakeCtl{view: engine.View{
		Mode: "Pulse", Palette: "sunset", Sensitivity: 0.5, Latency: 60 * time.Millisecond,
		Features: analysis.Features{BPM: 120, Confidence: 0.9},
		Lamps: []engine.LampView{
			{Info: light.Info{Name: "гостиная W3"}, Stats: light.Stats{Status: light.StatusOK, FPS: 20}, Brightness: 1},
			{Info: light.Info{Name: "wiz-1"}, Stats: light.Stats{Status: light.StatusLost, Note: "нет ответа"}},
		},
		History: []engine.Marker{{Beat: true}, {Low: true}, {High: true}},
	}}
	m := tea.Model(New(c))
	m, _ = press(m, tea.WindowSizeMsg{Width: 100, Height: 30}, tickMsg(time.Now()))
	out := m.View()
	for _, s := range []string{"Pulse", "120 BPM", "гостиная W3", "wiz-1", "нет ответа", "нет звука", "sunset"} {
		if !strings.Contains(out, s) {
			t.Fatalf("view lacks %q:\n%s", s, out)
		}
	}
}

func TestSpectrumRows(t *testing.T) {
	rows := spectrumRows([]float64{1, 0, 0.5}, 2)
	if len(rows) != 2 {
		t.Fatalf("rows %d", len(rows))
	}
	top, bottom := []rune(rows[0]), []rune(rows[1])
	if top[0] != '█' || bottom[0] != '█' {
		t.Fatalf("full band: %q %q", rows[0], rows[1])
	}
	if top[1] != ' ' || bottom[1] != ' ' {
		t.Fatalf("empty band: %q %q", rows[0], rows[1])
	}
	if top[2] != ' ' || bottom[2] != '█' {
		t.Fatalf("half band: %q %q", rows[0], rows[1])
	}
}

func TestLampPicker(t *testing.T) {
	c := &fakeCtl{view: engine.View{Lamps: []engine.LampView{
		{Info: light.Info{ID: "a", Name: "hero"}, Enabled: true},
		{Info: light.Info{ID: "b", Name: "потолок"}, Enabled: false},
	}}}
	m := tea.Model(New(c))
	m, _ = press(m, tickMsg(time.Now()))
	if !strings.Contains(m.View(), "выключена") {
		t.Fatalf("disabled lamp tile not marked:\n%s", m.View())
	}
	m, _ = press(m, runes("l"))
	if out := m.View(); !strings.Contains(out, "Лампы") || !strings.Contains(out, "hero") || !strings.Contains(out, "потолок") {
		t.Fatalf("lamp picker not shown:\n%s", out)
	}
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeySpace})
	if cmd == nil {
		t.Fatal("space returned no command")
	}
	cmd()
	if last := c.calls[len(c.calls)-1]; last != "lamp b true" {
		t.Fatalf("calls %v", c.calls)
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if strings.Contains(m.View(), "Лампы") {
		t.Fatal("picker still open after esc")
	}
}
