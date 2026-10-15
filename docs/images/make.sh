#!/bin/bash
# Regenerate the README screenshots (docs/images/hero-{dark,light}.svg).
# Requires termframe (https://github.com/pamburus/termframe).
# The frame is the real TUI rendered by internal/tui/shot_test.go with
# made-up but plausible music and lamps.
set -euo pipefail
cd "$(dirname "$0")/../.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
for mode in dark light; do
  LAMPA_SHOT="$tmp/$mode.ansi" LAMPA_SHOT_THEME=$mode go test ./internal/tui -run TestShot -count=1 >/dev/null
  termframe --title lampa --mode "$mode" --embed-fonts true -W 96 -H 30 \
    -o "docs/images/hero-$mode.svg" -- cat "$tmp/$mode.ansi" </dev/null 2>/dev/null
done
echo "Wrote docs/images/hero-dark.svg and docs/images/hero-light.svg"
