package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"lampa/internal/config"
	"lampa/internal/engine"
	"lampa/internal/i18n"
	"lampa/internal/light"
	"lampa/internal/nowplaying"
)

func TestBuildLightsDryUsesConfigNames(t *testing.T) {
	cfg := config.Default()
	cfg.Lamps = []config.Lamp{
		{Kind: "wiz", IP: "10.0.0.5", ID: "a", Name: "left", Role: "bass"},
		{Kind: "yeelight", IP: "10.0.0.6", ID: "b", Name: "w3", Disabled: true},
	}
	ls := buildLights(cfg, true)
	// disabled lamps are built too: the TUI lists them so they can be switched back on
	if len(ls) != 2 || ls[0].Info().Name != "left" || ls[0].Info().Kind != light.KindVirtual || ls[0].Info().Role != "bass" {
		t.Fatalf("%+v", ls)
	}
	if d := disabledIDs(cfg); len(d) != 1 || d[0] != "b" {
		t.Fatalf("disabled ids %v", d)
	}
}

func TestBuildLightsNoConfigGivesFourVirtual(t *testing.T) {
	ls := buildLights(config.Default(), false)
	if len(ls) != 4 || ls[0].Info().Kind != light.KindVirtual {
		t.Fatalf("%d lights", len(ls))
	}
}

func TestBuildLightsRealKinds(t *testing.T) {
	cfg := config.Default()
	cfg.Lamps = []config.Lamp{
		{Kind: "wiz", IP: "127.0.0.1", ID: "a", Name: "w"},
		{Kind: "yeelight", IP: "127.0.0.1", ID: "b", Name: "y", Music: true},
		{Kind: "hue", IP: "127.0.0.1", ID: "c", Name: "?"},
	}
	ls := buildLights(cfg, false)
	defer func() {
		for _, l := range ls {
			l.Close()
		}
	}()
	if len(ls) != 2 || ls[0].Info().Kind != light.KindWiZ || ls[1].Info().Kind != light.KindYeelight {
		t.Fatalf("%+v", ls)
	}
}

func TestFindTap(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "my-tap")
	os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755)
	if got := findTap(p); got != p {
		t.Fatalf("explicit flag: %s", got)
	}
}

func TestClosingTheTerminalRestoresLamps(t *testing.T) {
	// Closing a herdr pane / Ghostty tab sends SIGHUP; it must go through
	// the same shutdown (and lamp restore) as Ctrl-C.
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP} {
		if !slices.Contains(shutdownSignals, sig) {
			t.Fatalf("%v does not trigger shutdown", sig)
		}
	}
}

func TestSetLampEnabledPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.toml")
	cfg := config.Default()
	cfg.Lamps = []config.Lamp{{Kind: "wiz", ID: "a", Name: "hero"}, {Kind: "wiz", ID: "b", Name: "top"}}
	eng := engine.New(engine.Options{Lights: buildLights(cfg, true)})
	eng.Start(context.Background())
	defer eng.Close(context.Background())
	c := &controller{Engine: eng, cfgPath: path, cfg: cfg}

	c.SetLampEnabled("b", false)
	saved, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Lamps[0].Disabled || !saved.Lamps[1].Disabled {
		t.Fatalf("saved %+v", saved.Lamps)
	}
	for _, l := range eng.View().Lamps {
		if l.Info.ID == "b" && l.Enabled {
			t.Fatal("engine still drives b")
		}
	}
	c.SetLampEnabled("b", true)
	if saved, _ = config.Load(path); saved.Lamps[1].Disabled {
		t.Fatal("re-enable not saved")
	}
}

func TestSetLangPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.toml")
	c := &controller{cfgPath: path, cfg: config.Default()}
	c.SetLang("en")
	if saved, _ := config.Load(path); saved.Lang != "en" {
		t.Fatalf("lang not saved: %+v", saved)
	}
}

func TestLangFlagOverridesConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Lang = "en"
	if got := chooseLang("", cfg); got != i18n.EN {
		t.Fatalf("config ignored: %s", got)
	}
	if got := chooseLang("ru", cfg); got != i18n.RU {
		t.Fatalf("flag ignored: %s", got)
	}
}

func TestNowPlayingLibSitsNextToTap(t *testing.T) {
	dir := t.TempDir()
	tap := filepath.Join(dir, "lampa-tap")
	if got := nowPlayingLib(tap); got != "" {
		t.Fatalf("no lib yet, got %q", got)
	}
	lib := filepath.Join(dir, "libnowplaying.dylib")
	os.WriteFile(lib, nil, 0o644)
	if got := nowPlayingLib(tap); got != lib {
		t.Fatalf("got %q", got)
	}
}

func TestControllerTrack(t *testing.T) {
	c := &controller{}
	if c.NowPlaying() != "" {
		t.Fatal("empty at start")
	}
	c.setTrack(nowplaying.Track{Title: "Fly-day Chinatown", Artist: "Yasuha"})
	if got := c.NowPlaying(); got != "Yasuha — Fly-day Chinatown" {
		t.Fatalf("%q", got)
	}
}

func TestSourceNameTranslatedWithError(t *testing.T) {
	defer i18n.Set(i18n.Current())
	i18n.Set(i18n.RU)
	eng := engine.New(engine.Options{})
	c := &controller{Engine: eng, srcName: "whole system", srcErr: "tap header: EOF"}
	if got := c.SourceName(); got != "вся система · tap header: EOF" {
		t.Fatalf("%q", got)
	}
}

func TestGroupAndLevelReachTheLamp(t *testing.T) {
	cfg := config.Default()
	cfg.Lamps = []config.Lamp{{Kind: "wiz", ID: "a", Name: "ikea top", Group: "ikea", Level: 3}}
	l := buildLights(cfg, true)[0].Info()
	if l.Group != "ikea" || l.Level != 3 {
		t.Fatalf("%+v", l)
	}
}

func TestFindTapInHomebrewLayout(t *testing.T) {
	// /opt/homebrew/bin/lampa -> Cellar/lampa/0.1.0/bin/lampa; helpers in libexec.
	root := t.TempDir()
	cellar := filepath.Join(root, "Cellar", "lampa", "0.1.0")
	os.MkdirAll(filepath.Join(cellar, "bin"), 0o755)
	os.MkdirAll(filepath.Join(cellar, "libexec"), 0o755)
	os.MkdirAll(filepath.Join(root, "bin"), 0o755)
	real := filepath.Join(cellar, "bin", "lampa")
	os.WriteFile(real, nil, 0o755)
	tap := filepath.Join(cellar, "libexec", "lampa-tap")
	os.WriteFile(tap, nil, 0o755)
	link := filepath.Join(root, "bin", "lampa")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	got, _ := filepath.EvalSymlinks(findTapFrom(link))
	want, _ := filepath.EvalSymlinks(tap)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
