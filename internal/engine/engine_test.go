package engine

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"lampa/internal/audio"
	"lampa/internal/light"
)

type fakeLight struct {
	info     light.Info
	hang     bool // Snapshot blocks until ctx is done (offline lamp)
	mu       sync.Mutex
	applied  int
	restored []light.State
	closed   bool
}

func (f *fakeLight) Info() light.Info { return f.info }
func (f *fakeLight) Apply(context.Context, light.Frame) error {
	f.mu.Lock()
	f.applied++
	f.mu.Unlock()
	return nil
}
func (f *fakeLight) Snapshot(ctx context.Context) (light.State, error) {
	if f.hang {
		<-ctx.Done()
		return light.State{}, errors.New("offline")
	}
	return light.State{Data: map[string]any{"id": f.info.ID}}, nil
}
func (f *fakeLight) Restore(_ context.Context, s light.State) error {
	f.mu.Lock()
	f.restored = append(f.restored, s)
	f.mu.Unlock()
	return nil
}
func (f *fakeLight) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}
func (f *fakeLight) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.applied
}

// kicks: 60 Hz bursts every 0.5 s, as 512-sample blocks.
func kicks(seconds float64, rate int) []audio.Block {
	n := int(seconds * float64(rate))
	s := make([]float32, n)
	for t := 0.25; t < seconds; t += 0.5 {
		start := int(t * float64(rate))
		for i := 0; i < rate/8 && start+i < n; i++ {
			x := float64(i) / float64(rate)
			s[start+i] = float32(0.8 * math.Exp(-x/0.03) * math.Sin(2*math.Pi*60*x))
		}
	}
	var out []audio.Block
	for i := 0; i < n; i += 512 {
		out = append(out, audio.Block{Rate: rate, Samples: s[i:min(i+512, n)]})
	}
	return out
}

func started(t *testing.T, lights ...light.Light) *Engine {
	e := New(Options{Lights: lights, Sensitivity: 0.5, Latency: 0})
	e.Start(context.Background())
	t.Cleanup(func() { e.Close(context.Background()) })
	return e
}

func TestEngineDrivesLights(t *testing.T) {
	a, b := light.NewVirtual(light.Info{ID: "a", MaxFPS: 100}), light.NewVirtual(light.Info{ID: "b", MaxFPS: 100})
	e := started(t, a, b)
	for _, blk := range kicks(4, 48000) {
		e.Process(blk)
		time.Sleep(200 * time.Microsecond)
	}
	time.Sleep(50 * time.Millisecond)
	if _, n := a.Last(); n == 0 {
		t.Fatal("lamp a got no frames")
	}
	if _, n := b.Last(); n == 0 {
		t.Fatal("lamp b got no frames")
	}
	v := e.View()
	if len(v.Lamps) != 2 || len(v.History) == 0 || v.Mode != "Pulse" {
		t.Fatalf("view %+v", v)
	}
	low := 0
	for _, m := range v.History {
		if m.Low {
			low++
		}
	}
	if low == 0 {
		t.Fatal("no kick markers in history")
	}
}

func TestEngineSnapshotFailure(t *testing.T) {
	off := &fakeLight{info: light.Info{ID: "off", MaxFPS: 50}, hang: true}
	on := &fakeLight{info: light.Info{ID: "on", MaxFPS: 50}}
	e := New(Options{Lights: []light.Light{off, on}})
	t0 := time.Now()
	e.Start(context.Background())
	if d := time.Since(t0); d > 3*time.Second {
		t.Fatalf("Start took %v", d)
	}
	for _, blk := range kicks(1, 48000) {
		e.Process(blk)
	}
	time.Sleep(50 * time.Millisecond)
	if on.count() == 0 {
		t.Fatal("healthy lamp got no frames")
	}
	done := make(chan struct{})
	go func() { e.Close(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close hung")
	}
	if len(off.restored) != 1 || off.restored[0].Data != nil {
		t.Fatalf("offline lamp restore %+v", off.restored)
	}
	if len(on.restored) != 1 || on.restored[0].Data["id"] != "on" {
		t.Fatalf("online lamp restore %+v", on.restored)
	}
	if !off.closed || !on.closed {
		t.Fatal("lamps not closed")
	}
}

func TestPause(t *testing.T) {
	l := light.NewVirtual(light.Info{ID: "a", MaxFPS: 100})
	e := started(t, l)
	e.TogglePause()
	for _, blk := range kicks(1, 48000) {
		e.Process(blk)
	}
	time.Sleep(30 * time.Millisecond)
	if _, n := l.Last(); n != 0 {
		t.Fatalf("paused engine sent %d frames", n)
	}
	if !e.View().Paused {
		t.Fatal("view not paused")
	}
}

func TestRateChange(t *testing.T) {
	e := started(t, light.NewVirtual(light.Info{ID: "a"}))
	e.Process(audio.Block{Rate: 48000, Samples: make([]float32, 4096)})
	e.Process(audio.Block{Rate: 44100, Samples: make([]float32, 4096)})
	if e.an.Rate() != 44100 {
		t.Fatalf("analyzer rate %d", e.an.Rate())
	}
}

func TestControls(t *testing.T) {
	e := started(t)
	e.SetMode(2)
	if v := e.View(); v.Mode != "Chase" {
		t.Fatalf("mode %s", v.Mode)
	}
	e.SetMode(99) // ignored
	if v := e.View(); v.Mode != "Chase" {
		t.Fatalf("mode %s after bad index", v.Mode)
	}
	p0 := e.View().Palette
	e.NextPalette()
	if e.View().Palette == p0 {
		t.Fatal("palette did not change")
	}
	for i := 0; i < 20; i++ {
		e.AdjustSensitivity(0.1)
		e.AdjustLatency(50 * time.Millisecond)
	}
	if v := e.View(); v.Sensitivity != 1 || v.Latency != 200*time.Millisecond {
		t.Fatalf("clamps: %v %v", v.Sensitivity, v.Latency)
	}
}

func TestSilenceWhenAudioStops(t *testing.T) {
	e := started(t, light.NewVirtual(light.Info{ID: "a"}))
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	e.Run(ctx, make(chan audio.Block))
	v := e.View()
	if v.AudioAlive || v.Features.Time == 0 {
		t.Fatalf("alive %v time %v", v.AudioAlive, v.Features.Time)
	}
}

func TestAddLight(t *testing.T) {
	e := started(t)
	l := light.NewVirtual(light.Info{ID: "late", MaxFPS: 100})
	e.AddLight(l)
	for _, blk := range kicks(1, 48000) {
		e.Process(blk)
	}
	time.Sleep(30 * time.Millisecond)
	if _, n := l.Last(); n == 0 || len(e.View().Lamps) != 1 {
		t.Fatal("added light not driven")
	}
}
