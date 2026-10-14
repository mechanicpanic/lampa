# lampa ✿

Your Mac's music → Yeelight and WiZ bulbs, in real time. A cozy terminal UI with a live spectrum, a beat ribbon and a card for every lamp.

[Русская версия](README.ru.md)

- Listens to the whole system or a single app (Core Audio process tap, no BlackHole needed)
- Finds the beat and lands flashes *on* it, with latency compensation
- Yeelight via **music mode** — no 60 commands/min limit; WiZ via local UDP
- Puts every lamp back the way it was when you quit

## Build

```bash
make            # bin/lampa-tap (Swift) + bin/lampa (Go)
make test
```

Needs macOS ≥ 14.4, Go and Swift. On first run macOS asks to allow audio recording for your terminal.

## Run

```bash
bin/lampa discover --save   # find lamps, save them to ~/.config/lampa/config.toml
bin/lampa                   # whole-system audio
bin/lampa --dry             # no lamps, screen only
bin/lampa --wav track.wav   # from a file
bin/lampa --lang en         # English UI (default: Russian; press t to switch)
```

Keys: `1–5` mode (Pulse, Spectrum, Chase, Ambient, Drop), `p` palette, `←/→` sensitivity, `↑/↓` light delay, `s` audio source, `l` choose lamps (on/off, remembered), `d` find lamps, `t` language, `space` pause, `q` quit (lamps go back to how they were).

## Lamp roles

In the config, `role = "bass" | "mid" | "high" | "lowmid" | "sub"` sets which band a lamp follows in Spectrum mode.

## Notes

- **Yeelight:** enable "LAN Control" in the Yeelight app. lampa switches the bulb to music mode itself; if a model doesn't support it, it runs slowly and says so on its card.
- **WiZ:** some lamps (e.g. WiZ Hero) ignore broadcast discovery; lampa also asks every address on your /24 directly.
