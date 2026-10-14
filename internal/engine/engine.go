// Package engine connects audio, analysis, modes and lamps.
package engine

import (
	"context"
	"sync"
	"time"

	"lampa/internal/analysis"
	"lampa/internal/audio"
	"lampa/internal/color"
	"lampa/internal/light"
	"lampa/internal/modes"
)

const (
	HistoryLen      = 188
	snapshotTimeout = 2 * time.Second
	restoreTimeout  = 2 * time.Second
	silenceAfter    = time.Second
	maxLatency      = 200 * time.Millisecond
)

type Options struct {
	Lights      []light.Light
	Mode        int
	Palette     int
	Sensitivity float64
	Latency     time.Duration
}

type LampView struct {
	Info       light.Info
	Stats      light.Stats
	Color      color.RGB
	Brightness float64
}

type Marker struct{ Beat, Low, High bool }

type View struct {
	Features    analysis.Features
	Mode        string
	Palette     string
	Sensitivity float64
	Latency     time.Duration
	Paused      bool
	AudioAlive  bool
	Lamps       []LampView
	History     []Marker
}

type lamp struct {
	l      light.Light
	r      *light.Runner
	env    color.Envelope
	state  light.State
	color  color.RGB
	bright float64
}

type Engine struct {
	mu          sync.Mutex
	lamps       []*lamp
	modes       []modes.Mode
	mode        int
	palette     int
	sensitivity float64
	latency     time.Duration
	paused      bool
	an          *analysis.Analyzer
	last        analysis.Features
	lastTime    time.Duration
	history     []Marker
	lastAudio   time.Time
	runCtx      context.Context
	cancelRuns  context.CancelFunc
	wg          sync.WaitGroup
}

func New(o Options) *Engine {
	if o.Sensitivity == 0 {
		o.Sensitivity = 0.5
	}
	e := &Engine{modes: modes.All(), sensitivity: o.Sensitivity, latency: o.Latency}
	e.mode = o.Mode
	if e.mode < 0 || e.mode >= len(e.modes) {
		e.mode = 0
	}
	e.palette = o.Palette % len(color.Palettes)
	for _, l := range o.Lights {
		e.lamps = append(e.lamps, &lamp{l: l, r: light.NewRunner(l)})
	}
	e.runCtx, e.cancelRuns = context.WithCancel(context.Background())
	return e
}

func snapshot(ctx context.Context, lp *lamp) {
	sctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()
	if s, err := lp.l.Snapshot(sctx); err == nil {
		lp.state = s
	}
}

func (e *Engine) startRunner(lp *lamp) {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		lp.r.Run(e.runCtx)
	}()
}

func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	lamps := append([]*lamp(nil), e.lamps...)
	e.mu.Unlock()
	var wg sync.WaitGroup
	for _, lp := range lamps {
		wg.Add(1)
		go func() { defer wg.Done(); snapshot(ctx, lp) }()
	}
	wg.Wait()
	for _, lp := range lamps {
		e.startRunner(lp)
	}
}

func (e *Engine) AddLight(l light.Light) {
	lp := &lamp{l: l, r: light.NewRunner(l)}
	snapshot(context.Background(), lp)
	e.mu.Lock()
	e.lamps = append(e.lamps, lp)
	e.mu.Unlock()
	e.startRunner(lp)
}

func (e *Engine) Run(ctx context.Context, blocks <-chan audio.Block) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-blocks:
			e.mu.Lock()
			e.lastAudio = time.Now()
			e.mu.Unlock()
			e.Process(b)
		case <-tick.C:
			e.mu.Lock()
			silent := time.Since(e.lastAudio) > silenceAfter
			rate := 48000
			if e.an != nil {
				rate = e.an.Rate()
			}
			e.mu.Unlock()
			if silent { // keep time moving so lamps fade to their resting state
				e.Process(audio.Block{Rate: rate, Samples: make([]float32, rate/10)})
			}
		}
	}
}

func (e *Engine) Process(b audio.Block) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.an == nil || e.an.Rate() != b.Rate {
		e.an = analysis.New(analysis.Config{SampleRate: b.Rate, Sensitivity: e.sensitivity, Latency: e.latency})
		e.lastTime = 0
	}
	for _, f := range e.an.Push(b.Samples) {
		e.step(f)
	}
}

// step must be called with e.mu held.
func (e *Engine) step(f analysis.Features) {
	m := e.modes[e.mode]
	infos := make([]light.Info, len(e.lamps))
	for i, lp := range e.lamps {
		infos[i] = lp.l.Info()
	}
	scene := m.Update(f, modes.Context{Palette: color.Palettes[e.palette], Lights: infos})
	att, rel := m.Envelope()
	dt := f.Time - e.lastTime
	if dt <= 0 || dt > 100*time.Millisecond {
		dt = 10 * time.Millisecond
	}
	e.lastTime = f.Time
	for i, lp := range e.lamps {
		if i >= len(scene) {
			break
		}
		lp.env.Attack, lp.env.Release = att, rel
		lp.bright = lp.env.Step(scene[i].Brightness, dt)
		lp.color = scene[i].Color.ToRGB()
		if !e.paused {
			r, g, b := lp.color.Bytes()
			lp.r.Set(light.Frame{R: r, G: g, B: b, Brightness: lp.bright})
		}
	}
	e.last = f
	e.history = append(e.history, Marker{Beat: f.Beat, Low: f.OnsetLow.Hit, High: f.OnsetHigh.Hit})
	if len(e.history) > HistoryLen {
		e.history = e.history[len(e.history)-HistoryLen:]
	}
}

func (e *Engine) View() View {
	e.mu.Lock()
	defer e.mu.Unlock()
	v := View{
		Features:    e.last,
		Mode:        e.modes[e.mode].Name(),
		Palette:     color.Palettes[e.palette].Name,
		Sensitivity: e.sensitivity,
		Latency:     e.latency,
		Paused:      e.paused,
		AudioAlive:  !e.lastAudio.IsZero() && time.Since(e.lastAudio) <= silenceAfter,
		History:     append([]Marker(nil), e.history...),
	}
	for _, lp := range e.lamps {
		v.Lamps = append(v.Lamps, LampView{Info: lp.l.Info(), Stats: lp.r.Stats(), Color: lp.color, Brightness: lp.bright})
	}
	return v
}

func (e *Engine) SetMode(i int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if i >= 0 && i < len(e.modes) {
		e.mode = i
	}
}

func (e *Engine) NextPalette() {
	e.mu.Lock()
	e.palette = (e.palette + 1) % len(color.Palettes)
	e.mu.Unlock()
}

func (e *Engine) AdjustSensitivity(d float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sensitivity = max(0, min(1, e.sensitivity+d))
	e.sensitivity = float64(int(e.sensitivity*100+0.5)) / 100 // keep steps clean for display
	if e.an != nil {
		e.an.SetSensitivity(e.sensitivity)
	}
}

func (e *Engine) AdjustLatency(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.latency = max(-maxLatency, min(maxLatency, e.latency+d))
	if e.an != nil {
		e.an.SetLatency(e.latency)
	}
}

func (e *Engine) TogglePause() {
	e.mu.Lock()
	e.paused = !e.paused
	e.mu.Unlock()
}

func (e *Engine) Close(ctx context.Context) {
	e.cancelRuns()
	e.wg.Wait()
	e.mu.Lock()
	lamps := append([]*lamp(nil), e.lamps...)
	e.mu.Unlock()
	var wg sync.WaitGroup
	for _, lp := range lamps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rctx, cancel := context.WithTimeout(ctx, restoreTimeout)
			defer cancel()
			lp.l.Restore(rctx, lp.state)
			lp.l.Close()
		}()
	}
	wg.Wait()
}
