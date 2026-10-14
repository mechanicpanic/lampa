// Package yeelight drives Yeelight bulbs, streaming through music mode
// (the bulb connects back to us; no 60 commands/min quota).
package yeelight

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"lampa/internal/light"
)

const (
	ControlPort  = 55443
	musicFPS     = 20
	fallbackFPS  = 0.45
	dialTimeout  = 3 * time.Second
	acceptWait   = 5 * time.Second
	writeTimeout = 500 * time.Millisecond
)

var errNotConnected = errors.New("yeelight: not connected")

type Bulb struct {
	info       light.Info
	addr       string
	music      bool
	backoffMin time.Duration
	backoffMax time.Duration
	nextID     atomic.Int64
	// one streaming session: started by Apply, ended by Restore/Close
	done    chan struct{} // guarded by mu; closed when the session ends
	running bool          // guarded by mu
	closed  bool          // guarded by mu; Close is final

	mu     sync.Mutex
	stream net.Conn
	status light.Status
	note   string
}

func New(ip string, info light.Info, music bool) *Bulb {
	return newBulb(net.JoinHostPort(ip, strconv.Itoa(ControlPort)), info, music, 500*time.Millisecond, 8*time.Second)
}

func newBulb(addr string, info light.Info, music bool, bmin, bmax time.Duration) *Bulb {
	info.Kind = light.KindYeelight
	if !music {
		info.MaxFPS = fallbackFPS
	} else if info.MaxFPS <= 0 {
		info.MaxFPS = musicFPS
	}
	b := &Bulb{info: info, addr: addr, music: music, backoffMin: bmin, backoffMax: bmax,
		status: light.StatusConnecting}
	return b
}

func (b *Bulb) Info() light.Info { return b.info }

func (b *Bulb) Status() (light.Status, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status, b.note
}

func (b *Bulb) setStatus(s light.Status, note string) {
	b.mu.Lock()
	b.status, b.note = s, note
	b.mu.Unlock()
}

func (b *Bulb) cmd(method string, params ...any) []byte {
	msg, _ := json.Marshal(map[string]any{"id": b.nextID.Add(1), "method": method, "params": params})
	return append(msg, '\r', '\n')
}

func isDone(done chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func (b *Bulb) loop(done chan struct{}) {
	backoff := b.backoffMin
	for !isDone(done) {
		b.setStatus(light.StatusConnecting, "")
		conn, err := b.connect()
		if err != nil {
			b.setStatus(light.StatusLost, err.Error())
			select {
			case <-done:
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, b.backoffMax)
			continue
		}
		backoff = b.backoffMin
		note := ""
		if !b.music {
			note = "без music mode: ≈0,5 кадра/с"
		}
		b.mu.Lock()
		if isDone(done) { // shutdown raced with connect
			b.mu.Unlock()
			conn.Close()
			return
		}
		b.stream, b.status, b.note = conn, light.StatusOK, note
		b.mu.Unlock()
		io.Copy(io.Discard, conn) // returns when the bulb drops the connection
		b.mu.Lock()
		if b.stream == conn {
			b.stream = nil
		}
		b.mu.Unlock()
		conn.Close()
	}
}

// connect returns the connection frames are written to: the bulb's
// music-mode callback connection, or the control connection in fallback.
func (b *Bulb) connect() (net.Conn, error) {
	ctrl, err := net.DialTimeout("tcp", b.addr, dialTimeout)
	if err != nil {
		return nil, err
	}
	if !b.music {
		if _, err := ctrl.Write(b.cmd("set_power", "on", "smooth", 300)); err != nil {
			ctrl.Close()
			return nil, err
		}
		return ctrl, nil
	}
	defer ctrl.Close()
	localIP := ctrl.LocalAddr().(*net.TCPAddr).IP
	ln, err := net.ListenTCP("tcp", &net.TCPAddr{IP: localIP})
	if err != nil {
		return nil, err
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if _, err := ctrl.Write(b.cmd("set_music", 1, localIP.String(), port)); err != nil {
		return nil, err
	}
	ctrl.SetReadDeadline(time.Now().Add(dialTimeout))
	if line, err := bufio.NewReader(ctrl).ReadString('\n'); err != nil {
		return nil, fmt.Errorf("set_music: %w", err)
	} else if strings.Contains(line, `"error"`) {
		return nil, fmt.Errorf("set_music отклонён: %s", strings.TrimSpace(line))
	}
	ln.SetDeadline(time.Now().Add(acceptWait))
	conn, err := ln.Accept()
	if err != nil {
		return nil, fmt.Errorf("лампа не подключилась в music mode: %w", err)
	}
	if _, err := conn.Write(b.cmd("set_power", "on", "sudden", 30)); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// Apply starts streaming on first use, so Snapshot (taken before the first
// frame) sees the bulb as the user left it, not after our set_power on.
func (b *Bulb) Apply(_ context.Context, f light.Frame) error {
	b.mu.Lock()
	if !b.running && !b.closed {
		b.running, b.done = true, make(chan struct{})
		go b.loop(b.done)
	}
	conn := b.stream
	b.mu.Unlock()
	if conn == nil {
		return errNotConnected
	}
	r, g, bl, lvl := light.Split(f, 0.01)
	rgb := int(r)<<16 | int(g)<<8 | int(bl)
	bright := max(1, min(100, int(math.Round(lvl*100))))
	effect, dur := "sudden", 30
	if !b.music {
		effect, dur = "smooth", 900
	}
	msg := append(b.cmd("set_rgb", rgb, effect, dur), b.cmd("set_bright", bright, effect, dur)...)
	conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	if _, err := conn.Write(msg); err != nil {
		conn.Close() // loop notices and reconnects
		return err
	}
	return nil
}

var snapshotProps = []string{"power", "bright", "rgb", "ct", "color_mode"}

func (b *Bulb) Snapshot(ctx context.Context) (light.State, error) {
	var d net.Dialer
	ctrl, err := d.DialContext(ctx, "tcp", b.addr)
	if err != nil {
		return light.State{}, err
	}
	defer ctrl.Close()
	if dl, ok := ctx.Deadline(); ok {
		ctrl.SetDeadline(dl)
	}
	params := make([]any, len(snapshotProps))
	for i, p := range snapshotProps {
		params[i] = p
	}
	req := b.cmd("get_prop", params...)
	var sent struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(req, &sent)
	if _, err := ctrl.Write(req); err != nil {
		return light.State{}, err
	}
	rd := bufio.NewReader(ctrl)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return light.State{}, fmt.Errorf("get_prop: %w", err)
		}
		var resp struct {
			ID     int64    `json:"id"`
			Result []string `json:"result"`
		}
		if json.Unmarshal([]byte(line), &resp) != nil || resp.ID != sent.ID {
			continue // notification or unrelated reply
		}
		data := map[string]any{}
		for i, p := range snapshotProps {
			if i < len(resp.Result) {
				data[p] = resp.Result[i]
			}
		}
		return light.State{Data: data}, nil
	}
}

// Restore stops streaming, leaves music mode and puts the bulb back.
func (b *Bulb) Restore(ctx context.Context, s light.State) error {
	b.shutdown()
	var d net.Dialer
	ctrl, err := d.DialContext(ctx, "tcp", b.addr)
	if err != nil {
		return err
	}
	defer ctrl.Close()
	ctrl.SetDeadline(time.Now().Add(2 * time.Second))
	var msgs [][]byte
	if b.music {
		msgs = append(msgs, b.cmd("set_music", 0))
	}
	str := func(k string) string { v, _ := s.Data[k].(string); return v }
	atoi := func(k string) int { n, _ := strconv.Atoi(str(k)); return n }
	if s.Data != nil {
		if str("power") == "off" {
			msgs = append(msgs, b.cmd("set_power", "off", "smooth", 500))
		} else {
			switch str("color_mode") {
			case "1":
				msgs = append(msgs, b.cmd("set_rgb", atoi("rgb"), "smooth", 500))
			case "2":
				msgs = append(msgs, b.cmd("set_ct_abx", atoi("ct"), "smooth", 500))
			}
			if br := atoi("bright"); br > 0 {
				msgs = append(msgs, b.cmd("set_bright", br, "smooth", 500))
			}
		}
	}
	for _, m := range msgs {
		if _, err := ctrl.Write(m); err != nil {
			return err
		}
	}
	rd := bufio.NewReader(ctrl) // wait for replies so commands land before we close
	for range msgs {
		if _, err := rd.ReadString('\n'); err != nil {
			break
		}
	}
	return nil
}

// shutdown ends the streaming session without waiting for an in-flight dial
// (an offline bulb must not delay exit); loop() re-checks done under mu.
// The next Apply starts a new session.
func (b *Bulb) shutdown() {
	b.mu.Lock()
	if b.running {
		close(b.done)
		b.running = false
	}
	b.status, b.note = light.StatusConnecting, ""
	if b.stream != nil {
		b.stream.Close()
		b.stream = nil
	}
	b.mu.Unlock()
}

func (b *Bulb) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.shutdown()
	return nil
}
