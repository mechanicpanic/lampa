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
	wizAddrs := wizTargets(localNets())
	ssdpAddr := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1982}
	probe := func() {
		for _, a := range wizAddrs {
			conn.WriteToUDP([]byte(`{"method":"getSystemConfig","params":{}}`), a)
		}
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

// wizTargets: broadcast, plus every host of our own /24 directly — some WiZ
// lamps (Hero, ESP20_SHRGB_01BT) ignore broadcast and answer only unicast.
func wizTargets(nets []*net.IPNet) []*net.UDPAddr {
	out := []*net.UDPAddr{{IP: net.IPv4bcast, Port: 38899}}
	for _, n := range nets {
		ip := n.IP.To4()
		if ip == nil {
			continue
		}
		mask := n.Mask
		if len(mask) == net.IPv6len {
			mask = mask[12:]
		}
		bcast := make(net.IP, 4)
		for i := range bcast {
			bcast[i] = ip[i] | ^mask[i]
		}
		out = append(out, &net.UDPAddr{IP: bcast, Port: 38899})
		for h := 1; h <= 254; h++ {
			if byte(h) == ip[3] {
				continue
			}
			out = append(out, &net.UDPAddr{IP: net.IPv4(ip[0], ip[1], ip[2], byte(h)), Port: 38899})
		}
	}
	return out
}

func localNets() []*net.IPNet {
	var out []*net.IPNet
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				out = append(out, n)
			}
		}
	}
	return out
}
