// Package tui is the interactive terminal UI.
package tui

import (
	"context"
	"fmt"
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"lampa/internal/analysis"
	"lampa/internal/audio"
	"lampa/internal/engine"
	"lampa/internal/i18n"
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
	SetLang(lang string) // remember the UI language
	NowPlaying() string
	HideSleeping() bool      // hide switched-off lamps from the main screen
	SetHideSleeping(on bool) // "Artist — Title" or ""
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
	c            Controller
	v            engine.View
	width        int
	height       int
	picking      bool
	apps         []audio.App
	cursor       int
	lamps        bool // lamp picker open
	lampCursor   int
	hideSleeping bool
	help         bool // help screen open
	helpScroll   int
	// display smoothing (the engine's values are raw, per analysis frame)
	bands    [analysis.NumBands]float64
	fps      map[string]float64
	lastTick time.Time
	// ribbon: one column per UI frame, so markers glide left at a steady pace
	ribbon     []engine.Marker
	seenFrames uint64
	status     string
}

func New(c Controller) Model {
	if zone.DefaultManager == nil {
		zone.NewGlobal() // clickable cards
	}
	return Model{c: c, v: c.View(), width: 80, height: 24, fps: map[string]float64{}, hideSleeping: c.HideSleeping()}
}

func tick() tea.Cmd {
	return tea.Tick(33*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Init() tea.Cmd { return tick() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.v = m.c.View()
		m.smooth(time.Time(msg))
		m.advanceRibbon()
		return m, tick()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case appsMsg:
		if msg.err != nil {
			m.status = i18n.T("sources: ") + msg.err.Error()
			return m, nil
		}
		m.picking, m.apps, m.cursor = true, msg.apps, 0
	case discoveredMsg:
		if msg.err != nil {
			m.status = i18n.T("discovery: ") + msg.err.Error()
		} else {
			m.status = fmt.Sprintf(i18n.T("new lamps found: %d"), msg.added)
		}
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && !m.picking && !m.lamps && !m.help {
			return m, m.clickCmd(msg)
		}
		return m, nil
	case tea.KeyMsg:
		if m.picking {
			return m.updatePicker(msg)
		}
		if m.lamps {
			return m.updateLampPicker(msg)
		}
		if m.help {
			return m.updateHelp(msg)
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
	case "1", "2", "3", "4", "5", "6":
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
	case "?":
		m.help, m.helpScroll = true, 0
	case "l":
		m.lamps, m.lampCursor = true, 0
	case "t":
		m.c.SetLang(string(i18n.Toggle()))
	case "s":
		c := m.c
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			apps, err := c.Sources(ctx)
			return appsMsg{apps, err}
		}
	case "d":
		m.status = i18n.T("looking for lamps…")
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
	case "h":
		m.hideSleeping = !m.hideSleeping
		m.c.SetHideSleeping(m.hideSleeping)
	case "esc", "q", "l":
		m.lamps = false
	}
	return m, nil
}

const (
	bandRelease = 150 * time.Millisecond // bars rise instantly, fall over ~this
	fpsTau      = time.Second
)

// smooth eases the spectrum and lamp fps toward the engine's latest values,
// so the screen doesn't twitch at every analysis frame.
func (m *Model) smooth(now time.Time) {
	dt := now.Sub(m.lastTick)
	first := m.lastTick.IsZero() || dt <= 0 || dt > time.Second
	m.lastTick = now
	k := 1 - math.Exp(-dt.Seconds()/bandRelease.Seconds())
	for i, v := range m.v.Features.Bands {
		if first || v >= m.bands[i] {
			m.bands[i] = v
		} else {
			m.bands[i] += (v - m.bands[i]) * k
		}
	}
	kf := 1 - math.Exp(-dt.Seconds()/fpsTau.Seconds())
	for _, l := range m.v.Lamps {
		old, seen := m.fps[l.Info.ID]
		if first || !seen {
			m.fps[l.Info.ID] = l.Stats.FPS
		} else {
			m.fps[l.Info.ID] = old + (l.Stats.FPS-old)*kf
		}
	}
}

// shownFPS: a lamp keeping up shows its target rate; only real lag shows a
// lower number. The raw 1-second count wobbles between 19 and 20.
func (m Model) shownFPS(l engine.LampView) float64 {
	target := l.Info.MaxFPS
	if target <= 0 {
		target = 20
	}
	fps, ok := m.fps[l.Info.ID]
	if !ok {
		fps = l.Stats.FPS
	}
	if fps >= 0.85*target {
		return target
	}
	return fps
}

const ribbonMax = 512

// advanceRibbon adds exactly one column per UI frame: the beats/kicks/hats
// of all analysis frames that arrived since the previous one.
func (m *Model) advanceRibbon() {
	h, total := m.v.History, m.v.Frames
	if total < m.seenFrames { // engine restarted
		m.seenFrames = 0
	}
	n := int(min(total-m.seenFrames, uint64(len(h))))
	m.seenFrames = total
	var col engine.Marker
	for _, mk := range h[len(h)-n:] {
		col.Beat, col.Low, col.High = col.Beat || mk.Beat, col.Low || mk.Low, col.High || mk.High
	}
	m.ribbon = append(m.ribbon, col)
	if len(m.ribbon) > ribbonMax {
		m.ribbon = append(m.ribbon[:0], m.ribbon[len(m.ribbon)-ribbonMax:]...)
	}
}

// clickCmd: a click on a lamp card (or a floor bar) switches that lamp; a
// click on a group's name switches the whole group (all off if any is on).
func (m Model) clickCmd(msg tea.MouseMsg) tea.Cmd {
	c := m.c
	groups := map[string][]engine.LampView{}
	for _, l := range m.v.Lamps {
		if zone.Get("lamp:" + l.Info.ID).InBounds(msg) {
			return func() tea.Msg { c.SetLampEnabled(l.Info.ID, !l.Enabled); return nil }
		}
		if l.Info.Group != "" {
			groups[l.Info.Group] = append(groups[l.Info.Group], l)
		}
	}
	for name, members := range groups {
		if !zone.Get("group:" + name).InBounds(msg) {
			continue
		}
		on := true
		for _, l := range members {
			if l.Enabled {
				on = false
			}
		}
		return func() tea.Msg {
			for _, l := range members {
				if l.Enabled != on {
					c.SetLampEnabled(l.Info.ID, on)
				}
			}
			return nil
		}
	}
	return nil
}

func (m Model) updateHelp(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		m.helpScroll = max(0, m.helpScroll-1)
	case "down", "j":
		m.helpScroll++
	case "esc", "q", "?", "enter":
		m.help = false
	}
	return m, nil
}
