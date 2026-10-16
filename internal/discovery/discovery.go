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
	nets := localNets()
	wizAddrs, yeeAddrs := wizTargets(nets), yeelightTargets(nets)
	probe := func() {
		for _, a := range wizAddrs {
			conn.WriteToUDP([]byte(`{"method":"getSystemConfig","params":{}}`), a)
		}
		for _, a := range yeeAddrs {
			conn.WriteToUDP([]byte(ssdpSearch), a)
		}
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
		if b := directedBroadcast(n); b != nil {
			out = append(out, &net.UDPAddr{IP: b, Port: 38899})
		}
		for _, h := range lanHosts(n) {
			out = append(out, &net.UDPAddr{IP: h, Port: 38899})
		}
	}
	return out
}

// yeelightTargets: the SSDP multicast group, plus every host of our /24 —
// many routers (offices, hotels) drop multicast, and Yeelight answers the
// same search sent straight to its address.
func yeelightTargets(nets []*net.IPNet) []*net.UDPAddr {
	out := []*net.UDPAddr{{IP: net.IPv4(239, 255, 255, 250), Port: 1982}}
	for _, n := range nets {
		for _, h := range lanHosts(n) {
			out = append(out, &net.UDPAddr{IP: h, Port: 1982})
		}
	}
	return out
}

func directedBroadcast(n *net.IPNet) net.IP {
	ip := n.IP.To4()
	if ip == nil {
		return nil
	}
	mask := n.Mask
	if len(mask) == net.IPv6len {
		mask = mask[12:]
	}
	b := make(net.IP, 4)
	for i := range b {
		b[i] = ip[i] | ^mask[i]
	}
	return b
}

// lanHosts: every other address of our own /24 — only on private networks
// (home, office Wi-Fi). On a public range (a university uplink) probing
// every address would look like a scan, so we don't.
func lanHosts(n *net.IPNet) []net.IP {
	ip := n.IP.To4()
	if ip == nil || !ip.IsPrivate() {
		return nil
	}
	var out []net.IP
	for h := 1; h <= 254; h++ {
		if byte(h) != ip[3] {
			out = append(out, net.IPv4(ip[0], ip[1], ip[2], byte(h)))
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
