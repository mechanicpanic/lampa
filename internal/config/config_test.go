package config

import (
	"os"
	"path/filepath"
	"testing"

	"lampa/internal/discovery"
	"lampa/internal/light"
)

func TestMissingFileGivesDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != "pulse" || c.Sensitivity != 0.5 || c.LatencyMS != 60 {
		t.Fatalf("%+v", c)
	}
}

func TestPartialFileKeepsDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.toml")
	os.WriteFile(p, []byte("mode = \"chase\"\n[[lamp]]\nkind = \"wiz\"\nip = \"10.0.0.5\"\nid = \"aa\"\nname = \"left\"\n"), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != "chase" || c.Palette != "sunset" || c.LatencyMS != 60 || len(c.Lamps) != 1 || c.Lamps[0].Name != "left" {
		t.Fatalf("%+v", c)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "c.toml")
	in := Default()
	in.Lamps = []Lamp{{Kind: "yeelight", IP: "10.0.0.9", ID: "0x1", Name: "w3", Music: true, Role: "bass"}}
	if err := Save(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Lamps) != 1 || out.Lamps[0] != in.Lamps[0] || out.Mode != in.Mode {
		t.Fatalf("%+v", out)
	}
}

func TestBadFileIsAnError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.toml")
	os.WriteFile(p, []byte("mode = = broken"), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("want parse error")
	}
}

func TestMerge(t *testing.T) {
	c := Default()
	c.Lamps = []Lamp{{Kind: "wiz", IP: "10.0.0.5", ID: "aa", Name: "left", Role: "bass"}}
	added := c.Merge([]discovery.Found{
		{Kind: light.KindWiZ, IP: "10.0.0.77", ID: "aa", Name: "wiz-00aa", Model: "M"},
		{Kind: light.KindYeelight, IP: "10.0.0.9", ID: "0x1", Name: "yeelight-0x1", Music: true},
	})
	if added != 1 || len(c.Lamps) != 2 {
		t.Fatalf("added %d lamps %+v", added, c.Lamps)
	}
	if l := c.Lamps[0]; l.IP != "10.0.0.77" || l.Name != "left" || l.Role != "bass" || l.Model != "M" {
		t.Fatalf("known lamp not refreshed correctly: %+v", l)
	}
	if l := c.Lamps[1]; l.Kind != "yeelight" || !l.Music {
		t.Fatalf("new lamp %+v", l)
	}
}
