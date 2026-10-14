package wiz

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"lampa/internal/light"
)

type fakeBulb struct {
	conn   *net.UDPConn
	silent bool
	mu     sync.Mutex
	sets   []map[string]any
}

func newFake(t *testing.T, silent bool) *fakeBulb {
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeBulb{conn: c, silent: silent}
	t.Cleanup(func() { c.Close() })
	go f.serve()
	return f
}

func (f *fakeBulb) serve() {
	buf := make([]byte, 2048)
	for {
		n, from, err := f.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		var msg struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		json.Unmarshal(buf[:n], &msg)
		var reply string
		switch msg.Method {
		case "setPilot":
			f.mu.Lock()
			f.sets = append(f.sets, msg.Params)
			f.mu.Unlock()
			reply = `{"method":"setPilot","env":"pro","result":{"success":true}}`
		case "getPilot":
			reply = `{"method":"getPilot","env":"pro","result":{"mac":"aa","state":true,"sceneId":0,"r":10,"g":20,"b":30,"dimming":50}}`
		}
		if !f.silent && reply != "" {
			f.conn.WriteToUDP([]byte(reply), from)
		}
	}
}

func (f *fakeBulb) lastSet(t *testing.T) map[string]any {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := len(f.sets)
		var last map[string]any
		if n > 0 {
			last = f.sets[n-1]
		}
		f.mu.Unlock()
		if last != nil {
			return last
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no setPilot received")
	return nil
}

func dialFake(t *testing.T, f *fakeBulb) *Bulb {
	b, err := dial(f.conn.LocalAddr().String(), light.Info{ID: "w1"}, 50*time.Millisecond, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

func TestApply(t *testing.T) {
	f := newFake(t, false)
	b := dialFake(t, f)
	if b.Info().Kind != light.KindWiZ || b.Info().MaxFPS != 20 {
		t.Fatalf("info %+v", b.Info())
	}
	if err := b.Apply(context.Background(), light.Frame{R: 255, Brightness: 1}); err != nil {
		t.Fatal(err)
	}
	p := f.lastSet(t)
	if p["r"] != 255.0 || p["g"] != 0.0 || p["dimming"] != 100.0 || p["state"] != true {
		t.Fatalf("params %v", p)
	}
}

func TestDimmingFloor(t *testing.T) {
	f := newFake(t, false)
	b := dialFake(t, f)
	b.Apply(context.Background(), light.Frame{R: 200, Brightness: 0.05})
	if p := f.lastSet(t); p["dimming"] != 10.0 {
		t.Fatalf("dimming %v", p["dimming"])
	}
}

func TestSnapshotRestoreRGB(t *testing.T) {
	f := newFake(t, false)
	b := dialFake(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s, err := b.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(ctx, s); err != nil {
		t.Fatal(err)
	}
	p := f.lastSet(t)
	if p["r"] != 10.0 || p["g"] != 20.0 || p["b"] != 30.0 || p["dimming"] != 50.0 {
		t.Fatalf("restore params %v", p)
	}
}

func TestRestoreSceneAndOff(t *testing.T) {
	f := newFake(t, false)
	b := dialFake(t, f)
	ctx := context.Background()
	b.Restore(ctx, light.State{Data: map[string]any{"state": true, "sceneId": 4.0, "dimming": 60.0}})
	if p := f.lastSet(t); p["sceneId"] != 4.0 || p["r"] != nil {
		t.Fatalf("scene restore %v", p)
	}
	b.Restore(ctx, light.State{Data: map[string]any{"state": false}})
	time.Sleep(200 * time.Millisecond)
	if p := f.lastSet(t); p["state"] != false {
		t.Fatalf("off restore %v", p)
	}
}

func TestStatus(t *testing.T) {
	ok := dialFake(t, newFake(t, false))
	lost := dialFake(t, newFake(t, true))
	time.Sleep(400 * time.Millisecond)
	if s, _ := ok.Status(); s != light.StatusOK {
		t.Fatalf("responsive bulb status %s", s)
	}
	if s, _ := lost.Status(); s != light.StatusConnecting && s != light.StatusLost {
		t.Fatalf("silent bulb status %s", s)
	}
}

func TestSnapshotTimesOut(t *testing.T) {
	b := dialFake(t, newFake(t, true))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := b.Snapshot(ctx); err == nil {
		t.Fatal("want timeout error")
	}
}
