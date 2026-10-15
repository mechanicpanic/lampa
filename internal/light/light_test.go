package light

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSplit(t *testing.T) {
	r, g, b, lvl := Split(Frame{R: 255, G: 128, B: 0, Brightness: 1}, 0.1)
	if r != 255 || g != 128 || b != 0 || lvl != 1 {
		t.Fatalf("full: %d %d %d %v", r, g, b, lvl)
	}
	_, _, _, lvl = Split(Frame{R: 255, Brightness: 0.5}, 0.1)
	if lvl != 0.25 { // gamma 2
		t.Fatalf("gamma: %v", lvl)
	}
	// (no white in this color, so only the brightness scaling applies)
	r, g, b, lvl = Split(Frame{R: 200, G: 100, B: 0, Brightness: 0.2}, 0.1)
	// p = 0.04 < floor: level stays at floor, rgb scaled by 0.4
	if lvl != 0.1 || r != 80 || g != 40 || b != 0 {
		t.Fatalf("below floor: %d %d %d %v", r, g, b, lvl)
	}
	r, g, b, _ = Split(Frame{R: 10, G: 0, B: 0, Brightness: 0}, 0.1)
	if r < 1 && g < 1 && b < 1 {
		t.Fatalf("all channels zero: %d %d %d", r, g, b)
	}
	r, g, b, _ = Split(Frame{Brightness: 1}, 0.1)
	if r == 0 && g == 0 && b == 0 {
		t.Fatal("black input must still produce a non-zero channel")
	}
}

type fakeLight struct {
	info  Info
	delay time.Duration
	fail  bool
	mu    sync.Mutex
	got   []Frame
}

func (f *fakeLight) Info() Info { return f.info }
func (f *fakeLight) Apply(ctx context.Context, fr Frame) error {
	time.Sleep(f.delay)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("boom")
	}
	f.got = append(f.got, fr)
	return nil
}
func (f *fakeLight) Snapshot(context.Context) (State, error) { return State{}, nil }
func (f *fakeLight) Restore(context.Context, State) error    { return nil }
func (f *fakeLight) Close() error                            { return nil }
func (f *fakeLight) frames() []Frame {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Frame(nil), f.got...)
}

func run(t *testing.T, r *Runner) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go r.Run(ctx)
	t.Cleanup(cancel)
	return cancel
}

func TestLatestFrameWins(t *testing.T) {
	l := &fakeLight{info: Info{MaxFPS: 100}, delay: 30 * time.Millisecond}
	r := NewRunner(l)
	run(t, r)
	for i := 1; i <= 100; i++ {
		r.Set(Frame{R: uint8(i), Brightness: 1})
		time.Sleep(time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond)
	got := l.frames()
	if len(got) == 0 || got[len(got)-1].R != 100 {
		t.Fatalf("last applied %v", got)
	}
	if len(got) > 20 {
		t.Fatalf("applied %d frames, should have skipped stale ones", len(got))
	}
}

func TestSlowLightDoesNotBlockFast(t *testing.T) {
	slow := &fakeLight{info: Info{MaxFPS: 20}, delay: 300 * time.Millisecond}
	fast := &fakeLight{info: Info{MaxFPS: 20}}
	rs, rf := NewRunner(slow), NewRunner(fast)
	run(t, rs)
	run(t, rf)
	for i := 0; i < 50; i++ {
		f := Frame{R: uint8(i), Brightness: 1}
		rs.Set(f)
		rf.Set(f)
		time.Sleep(10 * time.Millisecond)
	}
	if n := len(fast.frames()); n < 8 {
		t.Fatalf("fast light got only %d frames", n)
	}
}

func TestDedupe(t *testing.T) {
	l := &fakeLight{info: Info{MaxFPS: 100}}
	r := NewRunner(l)
	run(t, r)
	for i := 0; i < 10; i++ {
		r.Set(Frame{R: 1, Brightness: 1})
		time.Sleep(15 * time.Millisecond)
	}
	if n := len(l.frames()); n != 1 {
		t.Fatalf("applied %d identical frames", n)
	}
}

func TestRateLimit(t *testing.T) {
	l := &fakeLight{info: Info{MaxFPS: 10}}
	r := NewRunner(l)
	run(t, r)
	deadline := time.Now().Add(500 * time.Millisecond)
	for i := 0; time.Now().Before(deadline); i++ {
		r.Set(Frame{R: uint8(i), Brightness: 1})
		time.Sleep(5 * time.Millisecond)
	}
	if n := len(l.frames()); n > 7 {
		t.Fatalf("applied %d frames in 0.5 s at 10 fps", n)
	}
}

func TestStatusOnFailure(t *testing.T) {
	l := &fakeLight{info: Info{MaxFPS: 50}, fail: true}
	r := NewRunner(l)
	run(t, r)
	r.Set(Frame{R: 1, Brightness: 1})
	time.Sleep(50 * time.Millisecond)
	if s := r.Stats(); s.Status != StatusLost || s.Errors == 0 {
		t.Fatalf("stats %+v", s)
	}
}

func TestVirtual(t *testing.T) {
	v := NewVirtual(Info{ID: "v1", Kind: KindVirtual, MaxFPS: 30})
	v.Apply(context.Background(), Frame{G: 9, Brightness: 0.5})
	if f, n := v.Last(); f.G != 9 || n != 1 {
		t.Fatalf("%+v %d", f, n)
	}
}

func TestSuspendDropsPendingAndWaitsForInFlight(t *testing.T) {
	l := &fakeLight{info: Info{MaxFPS: 100}, delay: 50 * time.Millisecond}
	r := NewRunner(l)
	run(t, r)
	r.Set(Frame{R: 1, Brightness: 1})
	time.Sleep(10 * time.Millisecond) // frame 1 is now being applied
	r.Set(Frame{R: 2, Brightness: 1}) // pending
	r.Suspend()
	if got := l.frames(); len(got) != 1 || got[0].R != 1 {
		t.Fatalf("after Suspend: %v (in-flight frame must have finished, pending dropped)", got)
	}
	r.Set(Frame{R: 3, Brightness: 1})
	time.Sleep(80 * time.Millisecond)
	if n := len(l.frames()); n != 1 {
		t.Fatalf("suspended runner applied %d frames", n)
	}
	r.Resume()
	r.Set(Frame{R: 4, Brightness: 1})
	time.Sleep(80 * time.Millisecond)
	if got := l.frames(); got[len(got)-1].R != 4 {
		t.Fatalf("after Resume: %v", got)
	}
}

func TestSplitMakesScreenColorsVividForLamps(t *testing.T) {
	// A screen green (78,167,57) looked white on a WiZ: the shared part of
	// r,g,b is rendered as white light. Most of it is removed for lamps.
	r, g, b, _ := Split(Frame{R: 78, G: 167, B: 57, Brightness: 1}, 0.1)
	if g != 167 || float64(r) > 0.35*float64(g) || float64(b) > 0.35*float64(g) {
		t.Fatalf("green still washed out: %d %d %d", r, g, b)
	}
	if r, g, b, _ := Split(Frame{R: 255, Brightness: 1}, 0.1); r != 255 || g != 0 || b != 0 {
		t.Fatalf("pure red changed: %d %d %d", r, g, b)
	}
	if r, g, b, _ := Split(Frame{R: 200, G: 200, B: 200, Brightness: 1}, 0.1); r != 200 || g != 200 || b != 200 {
		t.Fatalf("white must stay white: %d %d %d", r, g, b)
	}
}
