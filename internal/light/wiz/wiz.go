// Package wiz drives Philips WiZ bulbs over their local UDP API.
package wiz

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"strconv"
	"sync"
	"time"

	"lampa/internal/light"
)

const (
	Port        = 38899
	dimmingMin  = 0.10
	defaultFPS  = 20
	heartbeat   = time.Second
	lostTimeout = 3 * time.Second
)

type Bulb struct {
	info      light.Info
	conn      *net.UDPConn
	pilots    chan map[string]any
	done      chan struct{}
	closeOnce sync.Once
	hb, lost  time.Duration

	mu       sync.Mutex
	lastSeen time.Time
}

func Dial(ip string, info light.Info) (*Bulb, error) {
	return dial(net.JoinHostPort(ip, strconv.Itoa(Port)), info, heartbeat, lostTimeout)
}

func dial(addr string, info light.Info, hb, lost time.Duration) (*Bulb, error) {
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}
	info.Kind = light.KindWiZ
	if info.MaxFPS <= 0 {
		info.MaxFPS = defaultFPS
	}
	b := &Bulb{info: info, conn: conn, pilots: make(chan map[string]any, 1),
		done: make(chan struct{}), hb: hb, lost: lost}
	go b.readLoop()
	go b.heartbeatLoop()
	return b, nil
}

func (b *Bulb) Info() light.Info { return b.info }

func (b *Bulb) send(method string, params map[string]any) error {
	msg, err := json.Marshal(map[string]any{"method": method, "params": params})
	if err != nil {
		return err
	}
	_, err = b.conn.Write(msg)
	return err
}

func (b *Bulb) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := b.conn.Read(buf)
		if err != nil {
			select {
			case <-b.done:
				return
			default:
				continue // e.g. ICMP port unreachable while the bulb is off
			}
		}
		var msg struct {
			Method string         `json:"method"`
			Result map[string]any `json:"result"`
		}
		if json.Unmarshal(buf[:n], &msg) != nil {
			continue
		}
		b.mu.Lock()
		b.lastSeen = time.Now()
		b.mu.Unlock()
		if msg.Method == "getPilot" && msg.Result != nil {
			select {
			case b.pilots <- msg.Result:
			default:
			}
		}
	}
}

func (b *Bulb) heartbeatLoop() {
	t := time.NewTicker(b.hb)
	defer t.Stop()
	b.send("getPilot", map[string]any{})
	for {
		select {
		case <-b.done:
			return
		case <-t.C:
			b.send("getPilot", map[string]any{})
		}
	}
}

func (b *Bulb) Status() (light.Status, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case b.lastSeen.IsZero():
		return light.StatusConnecting, "waiting for reply"
	case time.Since(b.lastSeen) > b.lost:
		return light.StatusLost, "no reply"
	}
	return light.StatusOK, ""
}

func (b *Bulb) Apply(_ context.Context, f light.Frame) error {
	r, g, bl, lvl := light.Split(f, dimmingMin)
	dimming := int(math.Round(lvl * 100))
	dimming = max(10, min(100, dimming))
	return b.send("setPilot", map[string]any{"state": true, "r": r, "g": g, "b": bl, "dimming": dimming})
}

func (b *Bulb) Snapshot(ctx context.Context) (light.State, error) {
	select { // drop a stale heartbeat reply
	case <-b.pilots:
	default:
	}
	if err := b.send("getPilot", map[string]any{}); err != nil {
		return light.State{}, err
	}
	retry := time.NewTicker(300 * time.Millisecond)
	defer retry.Stop()
	for {
		select {
		case p := <-b.pilots:
			return light.State{Data: p}, nil
		case <-retry.C:
			b.send("getPilot", map[string]any{})
		case <-ctx.Done():
			return light.State{}, errors.New("wiz: no reply to getPilot")
		}
	}
}

func pick(src map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := src[k]; ok {
			out[k] = v
		}
	}
	return out
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

func (b *Bulb) Restore(ctx context.Context, s light.State) error {
	d := s.Data
	if d == nil {
		return nil
	}
	var params map[string]any
	switch {
	case d["state"] == false:
		params = map[string]any{"state": false}
	case num(d["sceneId"]) > 0:
		params = pick(d, "sceneId", "speed", "dimming")
	case num(d["temp"]) > 0:
		params = pick(d, "temp", "dimming")
	default:
		params = pick(d, "r", "g", "b", "c", "w", "dimming")
	}
	if d["state"] != false {
		params["state"] = true
	}
	// UDP: repeat a few times so a single dropped packet doesn't leave the lamp wrong.
	var err error
	for i := 0; i < 3; i++ {
		err = b.send("setPilot", params)
		select {
		case <-ctx.Done():
			return err
		case <-time.After(50 * time.Millisecond):
		}
	}
	return err
}

func (b *Bulb) Close() error {
	var err error
	b.closeOnce.Do(func() {
		close(b.done)
		err = b.conn.Close()
	})
	return err
}
