// Package discovery finds WiZ and Yeelight bulbs on the local network.
package discovery

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"lampa/internal/light"
)

type Found struct {
	Kind    light.Kind
	IP      string
	ID      string
	Model   string
	Name    string
	Music   bool
	Support []string
}

func suffix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func ParseWiZ(ip string, data []byte) (Found, bool) {
	var msg struct {
		Method string `json:"method"`
		Result struct {
			Mac        string `json:"mac"`
			ModuleName string `json:"moduleName"`
		} `json:"result"`
	}
	if json.Unmarshal(data, &msg) != nil || msg.Method != "getSystemConfig" || msg.Result.Mac == "" {
		return Found{}, false
	}
	return Found{Kind: light.KindWiZ, IP: ip, ID: msg.Result.Mac, Model: msg.Result.ModuleName,
		Name: "wiz-" + suffix(msg.Result.Mac, 4)}, true
}

func ParseYeelight(data []byte) (Found, bool) {
	lines := strings.Split(string(data), "\r\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "HTTP/1.1 200") {
		return Found{}, false
	}
	h := map[string]string{}
	for _, l := range lines[1:] {
		if k, v, ok := strings.Cut(l, ":"); ok {
			h[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	u, err := url.Parse(h["location"])
	if err != nil || u.Scheme != "yeelight" || h["id"] == "" {
		return Found{}, false
	}
	support := strings.Fields(h["support"])
	f := Found{Kind: light.KindYeelight, IP: u.Hostname(), ID: h["id"], Model: h["model"],
		Name: h["name"], Support: support, Music: slices.Contains(support, "set_music")}
	if f.Name == "" {
		f.Name = "yeelight-" + suffix(f.ID, 4)
	}
	return f, true
}

const ssdpSearch = "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1982\r\nMAN: \"ssdp:discover\"\r\nST: wifi_bulb\r\n\r\n"

func Discover(ctx context.Context, wait time.Duration) ([]Found, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	wizAddr := &net.UDPAddr{IP: net.IPv4bcast, Port: 38899}
	ssdpAddr := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1982}
	probe := func() {
		conn.WriteToUDP([]byte(`{"method":"getSystemConfig","params":{}}`), wizAddr)
		conn.WriteToUDP([]byte(ssdpSearch), ssdpAddr)
	}
	probe()
	deadline := time.Now().Add(wait)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	resent := false
	seen := map[string]Found{}
	buf := make([]byte, 4096)
	for time.Now().Before(deadline) {
		if !resent && time.Until(deadline) < wait/2 {
			probe() // UDP is lossy: ask twice
			resent = true
		}
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, from, err := conn.ReadFromUDP(buf)
		if ctx.Err() != nil {
			break
		}
		if err != nil {
			continue
		}
		f, ok := ParseWiZ(from.IP.String(), buf[:n])
		if !ok {
			f, ok = ParseYeelight(buf[:n])
		}
		if ok {
			seen[string(f.Kind)+"/"+f.ID] = f
		}
	}
	out := make([]Found, 0, len(seen))
	for _, f := range seen {
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b Found) int {
		if c := strings.Compare(string(a.Kind), string(b.Kind)); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}
