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
	offline  int  // Snapshot fails this many times, then the lamp "comes online"
	snaps    int
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
	f.mu.Lock()
	f.snaps++
	down := f.snaps <= f.offline
	f.mu.Unlock()
	if down {
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

func TestTimeContinuesAcrossRateChange(t *testing.T) {
	e := started(t, light.NewVirtual(light.Info{ID: "a"}))
	for _, blk := range kicks(3, 48000) {
		e.Process(blk)
	}
	before := e.View().Features.Time
	for _, blk := range kicks(1, 44100) {
		e.Process(blk)
	}
	if after := e.View().Features.Time; after <= before {
		t.Fatalf("time went back from %v to %v after a rate change", before, after)
	}
}

func TestLampOnlineLaterIsSnapshottedBeforeDriving(t *testing.T) {
	old := snapshotRetry
	snapshotRetry = 50 * time.Millisecond
	t.Cleanup(func() { snapshotRetry = old })
	l := &fakeLight{info: light.Info{ID: "late", MaxFPS: 100}, offline: 3}
	e := New(Options{Lights: []light.Light{l}})
	e.Start(context.Background())
	for _, blk := range kicks(0.1, 48000) {
		e.Process(blk)
	}
	time.Sleep(20 * time.Millisecond)
	if n := l.count(); n != 0 {
		t.Fatalf("lamp driven %d times before its state was saved", n)
	}
	time.Sleep(300 * time.Millisecond) // retries succeed on the 4th snapshot
	for _, blk := range kicks(1, 48000) {
		e.Process(blk)
	}
	time.Sleep(30 * time.Millisecond)
	if l.count() == 0 {
		t.Fatal("lamp not driven after it came online")
	}
	e.Close(context.Background())
	if len(l.restored) != 1 || l.restored[0].Data["id"] != "late" {
		t.Fatalf("restore %+v", l.restored)
	}
}

func (f *fakeLight) snapCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snaps
}

func (f *fakeLight) restoreCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.restored)
}

func lampEnabled(v View, id string) bool {
	for _, l := range v.Lamps {
		if l.Info.ID == id {
			return l.Enabled
		}
	}
	panic("no lamp " + id)
}

func TestDisabledAtStartIsUntouched(t *testing.T) {
	a := &fakeLight{info: light.Info{ID: "a", MaxFPS: 100}}
	b := &fakeLight{info: light.Info{ID: "b", MaxFPS: 100}}
	e := New(Options{Lights: []light.Light{a, b}, Disabled: []string{"b"}})
	e.Start(context.Background())
	for _, blk := range kicks(1, 48000) {
		e.Process(blk)
	}
	time.Sleep(30 * time.Millisecond)
	v := e.View()
	if !lampEnabled(v, "a") || lampEnabled(v, "b") {
		t.Fatalf("enabled flags wrong: %+v", v.Lamps)
	}
	if b.snapCount() != 0 || b.count() != 0 {
		t.Fatalf("disabled lamp touched: %d snapshots, %d frames", b.snapCount(), b.count())
	}
	if a.count() == 0 {
		t.Fatal("enabled lamp not driven")
	}
	e.Close(context.Background())
	if b.restoreCount() != 0 {
		t.Fatal("never-touched lamp was restored on Close")
	}
}

func TestDisableRestoresThenReenableResumes(t *testing.T) {
	a := &fakeLight{info: light.Info{ID: "a", MaxFPS: 100}}
	b := &fakeLight{info: light.Info{ID: "b", MaxFPS: 100}}
	e := New(Options{Lights: []light.Light{a, b}})
	e.Start(context.Background())
	t.Cleanup(func() { e.Close(context.Background()) })
	for _, blk := range kicks(1, 48000) {
		e.Process(blk)
	}
	time.Sleep(30 * time.Millisecond)

	e.SetEnabled("b", false)
	if b.restoreCount() != 1 || b.restored[0].Data["id"] != "b" {
		t.Fatalf("disable did not restore b: %+v", b.restored)
	}
	n := b.count()
	for _, blk := range kicks(1, 48000) {
		e.Process(blk)
	}
	time.Sleep(30 * time.Millisecond)
	if b.count() != n {
		t.Fatalf("disabled lamp still driven (%d → %d)", n, b.count())
	}
	if lampEnabled(e.View(), "b") {
		t.Fatal("view still shows b enabled")
	}

	e.SetEnabled("b", true)
	if b.snapCount() != 2 {
		t.Fatalf("re-enable must take a fresh snapshot, got %d snapshots", b.snapCount())
	}
	for _, blk := range kicks(1, 48000) {
		e.Process(blk)
	}
	time.Sleep(30 * time.Millisecond)
	if b.count() == n {
		t.Fatal("re-enabled lamp not driven")
	}
}
