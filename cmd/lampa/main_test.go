package main

import (
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"lampa/internal/config"
	"lampa/internal/light"
)

func TestBuildLightsDryUsesConfigNames(t *testing.T) {
	cfg := config.Default()
	cfg.Lamps = []config.Lamp{
		{Kind: "wiz", IP: "10.0.0.5", ID: "a", Name: "left", Role: "bass"},
		{Kind: "yeelight", IP: "10.0.0.6", ID: "b", Name: "w3", Disabled: true},
	}
	ls := buildLights(cfg, true)
	if len(ls) != 1 || ls[0].Info().Name != "left" || ls[0].Info().Kind != light.KindVirtual || ls[0].Info().Role != "bass" {
		t.Fatalf("%+v", ls)
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
