package nowplaying

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakePerl writes a script that stands in for /usr/bin/perl.
func fakePerl(t *testing.T, body string) string {
	p := filepath.Join(t.TempDir(), "perl")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFetch(t *testing.T) {
	perl := fakePerl(t, `echo '{"Title":"Fly-day Chinatown","Artist":"Yasuha","Album":"x"}'`)
	tr, err := Fetch(context.Background(), perl, "/lib.dylib")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Title != "Fly-day Chinatown" || tr.Artist != "Yasuha" {
		t.Fatalf("%+v", tr)
	}
	if s := tr.String(); s != "Yasuha — Fly-day Chinatown" {
		t.Fatalf("String() = %q", s)
	}
}

func TestStringWithoutArtist(t *testing.T) {
	if s := (Track{Title: "Air"}).String(); s != "Air" {
		t.Fatalf("%q", s)
	}
	if s := (Track{}).String(); s != "" {
		t.Fatalf("%q", s)
	}
}

func TestFetchNothingPlaying(t *testing.T) {
	tr, err := Fetch(context.Background(), fakePerl(t, `echo '{}'`), "/lib.dylib")
	if err != nil || tr != (Track{}) {
		t.Fatalf("%+v %v", tr, err)
	}
}

func TestFetchFailures(t *testing.T) {
	if _, err := Fetch(context.Background(), fakePerl(t, `echo boom >&2; exit 3`), "/lib.dylib"); err == nil {
		t.Fatal("want error on non-zero exit")
	}
	if _, err := Fetch(context.Background(), fakePerl(t, `echo not-json`), "/lib.dylib"); err == nil {
		t.Fatal("want error on garbage")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	t0 := time.Now()
	if _, err := Fetch(ctx, fakePerl(t, `sleep 5`), "/lib.dylib"); err == nil || time.Since(t0) > 2*time.Second {
		t.Fatalf("hung helper must time out quickly (err %v after %v)", err, time.Since(t0))
	}
}

func TestPollReportsChanges(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "title")
	os.WriteFile(state, []byte("A"), 0o644)
	perl := fakePerl(t, `printf '{"Title":"%s"}' "$(cat `+state+`)"`)
	got := make(chan Track, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Poll(ctx, perl, "/lib.dylib", 20*time.Millisecond, func(tr Track) { got <- tr })
	if tr := <-got; tr.Title != "A" {
		t.Fatalf("first %+v", tr)
	}
	os.WriteFile(state, []byte("B"), 0o644)
	select {
	case tr := <-got:
		if tr.Title != "B" {
			t.Fatalf("second %+v", tr)
		}
	case <-time.After(time.Second):
		t.Fatal("change not reported")
	}
	select {
	case tr := <-got:
		t.Fatalf("reported %+v although nothing changed", tr)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestLibPathIsMadeAbsolute(t *testing.T) {
	// Apple's perl is hardened: dlopen refuses relative paths.
	perl := fakePerl(t, `case "$3" in /*) echo '{"Title":"ok"}' ;; *) exit 9 ;; esac`)
	tr, err := Fetch(context.Background(), perl, "bin/libnowplaying.dylib")
	if err != nil || tr.Title != "ok" {
		t.Fatalf("%+v %v", tr, err)
	}
}
