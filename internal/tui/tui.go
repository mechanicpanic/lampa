// Package tui is the interactive terminal UI.
package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"lampa/internal/audio"
	"lampa/internal/engine"
)

type Controller interface {
	View() engine.View
	SetMode(int)
	NextPalette()
	AdjustSensitivity(float64)
	AdjustLatency(time.Duration)
	TogglePause()
	Sources(ctx context.Context) ([]audio.App, error)
	SetSource(app *audio.App)
	SourceName() string
	Discover(ctx context.Context) (added int, err error)
	SetLampEnabled(id string, on bool)
}

type tickMsg time.Time

type appsMsg struct {
	apps []audio.App
	err  error
}

type discoveredMsg struct {
	added int
	err   error
}

type Model struct {
	c          Controller
	v          engine.View
	width      int
	picking    bool
	apps       []audio.App
	cursor     int
	lamps      bool // lamp picker open
	lampCursor int
	status     string
}

func New(c Controller) Model {
	return Model{c: c, v: c.View(), width: 80}
}

func tick() tea.Cmd {
	return tea.Tick(33*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Init() tea.Cmd { return tick() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.v = m.c.View()
		return m, tick()
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case appsMsg:
		if msg.err != nil {
			m.status = "источники: " + msg.err.Error()
			return m, nil
		}
		m.picking, m.apps, m.cursor = true, msg.apps, 0
	case discoveredMsg:
		if msg.err != nil {
			m.status = "поиск: " + msg.err.Error()
		} else {
			m.status = fmt.Sprintf("найдено новых ламп: %d", msg.added)
		}
	case tea.KeyMsg:
		if m.picking {
			return m.updatePicker(msg)
		}
		if m.lamps {
			return m.updateLampPicker(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

func (m Model) updatePicker(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = min(len(m.apps), m.cursor+1)
	case "enter":
		if m.cursor == 0 {
			m.c.SetSource(nil)
		} else {
			app := m.apps[m.cursor-1]
			m.c.SetSource(&app)
		}
		m.picking = false
	case "esc", "q":
		m.picking = false
	}
	return m, nil
}

func (m Model) updateKeys(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch s := k.String(); s {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "1", "2", "3", "4", "5":
		m.c.SetMode(int(s[0] - '1'))
	case "p":
		m.c.NextPalette()
	case "right":
		m.c.AdjustSensitivity(0.05)
	case "left":
		m.c.AdjustSensitivity(-0.05)
	case "up":
		m.c.AdjustLatency(10 * time.Millisecond)
	case "down":
		m.c.AdjustLatency(-10 * time.Millisecond)
	case " ":
		m.c.TogglePause()
	case "l":
		m.lamps, m.lampCursor = true, 0
	case "s":
		c := m.c
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			apps, err := c.Sources(ctx)
			return appsMsg{apps, err}
		}
	case "d":
		m.status = "ищу лампы…"
		c := m.c
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			n, err := c.Discover(ctx)
			return discoveredMsg{n, err}
		}
	}
	return m, nil
}

func (m Model) updateLampPicker(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.v.Lamps)
	switch k.String() {
	case "up", "k":
		m.lampCursor = max(0, m.lampCursor-1)
	case "down", "j":
		m.lampCursor = min(max(0, n-1), m.lampCursor+1)
	case " ", "enter":
		if m.lampCursor < n {
			l := m.v.Lamps[m.lampCursor]
			c := m.c
			// switching talks to the lamp (restore / snapshot), so off the UI thread
			return m, func() tea.Msg { c.SetLampEnabled(l.Info.ID, !l.Enabled); return nil }
		}
	case "esc", "q", "l":
		m.lamps = false
	}
	return m, nil
}
