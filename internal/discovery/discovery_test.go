package discovery

import (
	"testing"

	"lampa/internal/light"
)

func TestParseWiZ(t *testing.T) {
	data := []byte(`{"method":"getSystemConfig","env":"pro","result":{"mac":"a8bb50c0ffee","homeId":1,"moduleName":"ESP01_SHRGB1C_31","fwVersion":"1.30.0"}}`)
	f, ok := ParseWiZ("192.168.1.50", data)
	if !ok {
		t.Fatal("not parsed")
	}
	if f.Kind != light.KindWiZ || f.IP != "192.168.1.50" || f.ID != "a8bb50c0ffee" || f.Model != "ESP01_SHRGB1C_31" || f.Name != "wiz-ffee" {
		t.Fatalf("%+v", f)
	}
	if _, ok := ParseWiZ("1.2.3.4", []byte(`{"method":"setPilot","result":{"success":true}}`)); ok {
		t.Fatal("non-config reply accepted")
	}
	if _, ok := ParseWiZ("1.2.3.4", []byte("garbage")); ok {
		t.Fatal("garbage accepted")
	}
}

func TestParseYeelight(t *testing.T) {
	reply := "HTTP/1.1 200 OK\r\nCache-Control: max-age=3600\r\nLocation: yeelight://192.168.1.239:55443\r\n" +
		"Server: POSIX UPnP/1.0 YGLC/1\r\nid: 0x000000000015243f\r\nmodel: color\r\nfw_ver: 18\r\n" +
		"support: get_prop set_default set_power toggle set_bright set_rgb set_music\r\npower: on\r\nname: \r\n\r\n"
	f, ok := ParseYeelight([]byte(reply))
	if !ok {
		t.Fatal("not parsed")
	}
	if f.Kind != light.KindYeelight || f.IP != "192.168.1.239" || f.ID != "0x000000000015243f" || f.Model != "color" || !f.Music || f.Name != "yeelight-243f" {
		t.Fatalf("%+v", f)
	}
	noMusic := "HTTP/1.1 200 OK\r\nLocation: yeelight://10.0.0.2:55443\r\nid: 0x1\r\nmodel: mono\r\nsupport: get_prop set_power\r\nname: desk\r\n\r\n"
	f, ok = ParseYeelight([]byte(noMusic))
	if !ok || f.Music || f.Name != "desk" {
		t.Fatalf("%+v %v", f, ok)
	}
	if _, ok := ParseYeelight([]byte("M-SEARCH * HTTP/1.1\r\nST: wifi_bulb\r\n\r\n")); ok {
		t.Fatal("our own request accepted as a reply")
	}
}
