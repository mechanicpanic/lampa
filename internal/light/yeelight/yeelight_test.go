package yeelight

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"lampa/internal/light"
)

type command struct {
	ID     int    `json:"id"`
	Method string `json:"method"`
	Params []any  `json:"params"`
}

type fakeYee struct {
	t           *testing.T
	ln          net.Listener
	rejectMusic bool
	mu          sync.Mutex
	ctrl        []command
	music       []command
	musicConns  []net.Conn
}

func newFakeYee(t *testing.T, rejectMusic bool) *fakeYee {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeYee{t: t, ln: ln, rejectMusic: rejectMusic}
	t.Cleanup(func() {
		ln.Close()
		f.mu.Lock()
		for _, c := range f.musicConns {
			c.Close()
		}
		f.mu.Unlock()
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.control(c)
		}
	}()
	return f
}

func (f *fakeYee) control(c net.Conn) {
	defer c.Close()
	sc := bufio.NewScanner(c)
	for sc.Scan() {
		var cmd command
		if json.Unmarshal(sc.Bytes(), &cmd) != nil {
			continue
		}
		f.mu.Lock()
		f.ctrl = append(f.ctrl, cmd)
		f.mu.Unlock()
		switch cmd.Method {
		case "get_prop":
			fmt.Fprintf(c, `{"method":"props","params":{"power":"on"}}`+"\r\n") // unsolicited notification first
			fmt.Fprintf(c, `{"id":%d,"result":["on","80","16711680","4000","1"]}`+"\r\n", cmd.ID)
		case "set_music":
			if f.rejectMusic {
				fmt.Fprintf(c, `{"id":%d,"error":{"code":-1,"message":"method not supported"}}`+"\r\n", cmd.ID)
				continue
			}
			fmt.Fprintf(c, `{"id":%d,"result":["ok"]}`+"\r\n", cmd.ID)
			if on, _ := cmd.Params[0].(float64); on == 1 {
				addr := net.JoinHostPort(cmd.Params[1].(string), fmt.Sprint(cmd.Params[2]))
				mc, err := net.Dial("tcp", addr)
				if err != nil {
					f.t.Errorf("dial back: %v", err)
					continue
				}
				f.mu.Lock()
				f.musicConns = append(f.musicConns, mc)
				f.mu.Unlock()
				go f.musicRead(mc)
			}
		default:
			fmt.Fprintf(c, `{"id":%d,"result":["ok"]}`+"\r\n", cmd.ID)
		}
	}
}

func (f *fakeYee) musicRead(c net.Conn) {
	sc := bufio.NewScanner(c)
	for sc.Scan() {
		var cmd command
		if json.Unmarshal(sc.Bytes(), &cmd) == nil {
			f.mu.Lock()
			f.music = append(f.music, cmd)
			f.mu.Unlock()
		}
	}
}

func (f *fakeYee) find(music bool, method string) (command, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	list := f.ctrl
	if music {
		list = f.music
	}
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].Method == method {
			return list[i], true
		}
	}
	return command{}, false
}

func (f *fakeYee) musicConnCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.musicConns)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func newTestBulb(t *testing.T, f *fakeYee, music bool) *Bulb {
	b := newBulb(f.ln.Addr().String(), light.Info{ID: "y1"}, music, 10*time.Millisecond, 50*time.Millisecond)
	t.Cleanup(func() { b.Close() })
	return b
}

func isOK(b *Bulb) func() bool {
	return func() bool { s, _ := b.Status(); return s == light.StatusOK }
}

func TestMusicModeStreams(t *testing.T) {
	f := newFakeYee(t, false)
	b := newTestBulb(t, f, true)
	waitFor(t, "music connection", isOK(b))
	if b.Info().MaxFPS != 20 || b.Info().Kind != light.KindYeelight {
		t.Fatalf("info %+v", b.Info())
	}
	if err := b.Apply(context.Background(), light.Frame{R: 255, Brightness: 1}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "set_bright on music conn", func() bool { _, ok := f.find(true, "set_bright"); return ok })
	rgb, _ := f.find(true, "set_rgb")
	if rgb.Params[0] != 16711680.0 || rgb.Params[1] != "sudden" {
		t.Fatalf("set_rgb %v", rgb.Params)
	}
	br, _ := f.find(true, "set_bright")
	if br.Params[0] != 100.0 {
		t.Fatalf("set_bright %v", br.Params)
	}
	if _, ok := f.find(true, "set_power"); !ok {
		t.Fatal("bulb not powered on at stream start")
	}
}

func TestReconnectAfterDrop(t *testing.T) {
	f := newFakeYee(t, false)
	b := newTestBulb(t, f, true)
	waitFor(t, "first connection", isOK(b))
	f.mu.Lock()
	f.musicConns[0].Close()
	f.mu.Unlock()
	waitFor(t, "second music connection", func() bool { return f.musicConnCount() == 2 })
	waitFor(t, "ok again", isOK(b))
	if err := b.Apply(context.Background(), light.Frame{G: 255, Brightness: 1}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "green on new conn", func() bool {
		c, ok := f.find(true, "set_rgb")
		return ok && c.Params[0] == float64(255<<8)
	})
}

func TestFallbackWithoutMusic(t *testing.T) {
	f := newFakeYee(t, false)
	b := newTestBulb(t, f, false)
	if b.Info().MaxFPS != 0.45 {
		t.Fatalf("fallback fps %v", b.Info().MaxFPS)
	}
	waitFor(t, "control connection", isOK(b))
	if _, note := b.Status(); !strings.Contains(note, "music") {
		t.Fatalf("note should explain fallback, got %q", note)
	}
	b.Apply(context.Background(), light.Frame{B: 255, Brightness: 1})
	waitFor(t, "smooth set_rgb on control", func() bool {
		c, ok := f.find(false, "set_rgb")
		return ok && c.Params[1] == "smooth"
	})
	if f.musicConnCount() != 0 {
		t.Fatal("music mode used despite music=false")
	}
}

func TestSetMusicRejected(t *testing.T) {
	f := newFakeYee(t, true)
	b := newTestBulb(t, f, true)
	waitFor(t, "lost status", func() bool { s, _ := b.Status(); return s == light.StatusLost })
	if err := b.Apply(context.Background(), light.Frame{R: 1, Brightness: 1}); err == nil {
		t.Fatal("Apply should fail while not connected")
	}
}

func TestSnapshotRestore(t *testing.T) {
	f := newFakeYee(t, false)
	b := newTestBulb(t, f, true)
	waitFor(t, "connection", isOK(b))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s, err := b.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.Data["power"] != "on" || s.Data["bright"] != "80" || s.Data["color_mode"] != "1" {
		t.Fatalf("snapshot %v", s.Data)
	}
	if err := b.Restore(ctx, s); err != nil {
		t.Fatal(err)
	}
	if c, ok := f.find(false, "set_music"); !ok || c.Params[0] != 0.0 {
		t.Fatalf("music mode not turned off: %v", c)
	}
	if c, ok := f.find(false, "set_rgb"); !ok || c.Params[0] != 16711680.0 {
		t.Fatalf("rgb not restored: %v", c)
	}
	if c, ok := f.find(false, "set_bright"); !ok || c.Params[0] != 80.0 {
		t.Fatalf("bright not restored: %v", c)
	}
	n := f.musicConnCount()
	time.Sleep(150 * time.Millisecond)
	if f.musicConnCount() != n {
		t.Fatal("bulb re-entered music mode after Restore")
	}
}
