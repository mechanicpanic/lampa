// Package nowplaying reads the system "Now Playing" track (Control Center).
//
// Since macOS 15.4 MediaRemote answers only Apple-signed processes, so we
// load our tiny libnowplaying.dylib into /usr/bin/perl (Apple-signed) and
// let it ask. If Apple closes this, Fetch just returns nothing.
package nowplaying

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

type Track struct {
	Title  string `json:"Title"`
	Artist string `json:"Artist"`
}

func (t Track) String() string {
	if t.Artist == "" {
		return t.Title
	}
	return t.Artist + " — " + t.Title
}

const Perl = "/usr/bin/perl"

// script loads the dylib and calls its XSUB-shaped entry point, which prints
// one JSON object and returns.
const script = `use DynaLoader;
my $l = DynaLoader::dl_load_file($ARGV[0], 0) or die DynaLoader::dl_error();
my $s = DynaLoader::dl_find_symbol($l, "lampa_nowplaying") or die DynaLoader::dl_error();
DynaLoader::dl_install_xsub("main::np", $s);
np();`

func Fetch(ctx context.Context, perl, lib string) (Track, error) {
	if abs, err := filepath.Abs(lib); err == nil {
		lib = abs // Apple's perl is hardened: dlopen refuses relative paths
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, perl, "-e", script, lib)
	cmd.WaitDelay = 500 * time.Millisecond // don't wait on a stuck child's pipes
	out, err := cmd.Output()
	if err != nil {
		return Track{}, fmt.Errorf("now playing: %w", err)
	}
	var t Track
	if err := json.Unmarshal(out, &t); err != nil {
		return Track{}, fmt.Errorf("now playing: %w", err)
	}
	return t, nil
}

// Poll calls onChange with the current track, then whenever it changes,
// until ctx is done. Errors count as "nothing playing".
func Poll(ctx context.Context, perl, lib string, every time.Duration, onChange func(Track)) {
	last, first := Track{}, true
	for {
		t, _ := Fetch(ctx, perl, lib)
		if ctx.Err() != nil {
			return
		}
		if first || t != last {
			onChange(t)
			last, first = t, false
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}
