package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"lampa/internal/audio"
	"lampa/internal/color"
	"lampa/internal/config"
	"lampa/internal/discovery"
	"lampa/internal/engine"
	"lampa/internal/light"
	"lampa/internal/light/wiz"
	"lampa/internal/light/yeelight"
	"lampa/internal/modes"
	"lampa/internal/tui"
)

// shutdownSignals end the session cleanly: lamps are restored on all of them.
var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "discover" {
		os.Exit(runDiscover(os.Args[2:]))
	}
	os.Exit(run(os.Args[1:]))
}

func info(l config.Lamp) light.Info {
	return light.Info{ID: l.ID, Name: l.Name, Model: l.Model, Role: l.Role, MaxFPS: l.MaxFPS}
}

func buildLights(cfg config.Config, dry bool) []light.Light {
	var out []light.Light
	for _, l := range cfg.Lamps {
		if dry {
			i := info(l)
			i.Kind = light.KindVirtual
			out = append(out, light.NewVirtual(i))
			continue
		}
		switch light.Kind(l.Kind) {
		case light.KindWiZ:
			b, err := wiz.Dial(l.IP, info(l))
			if err != nil {
				fmt.Fprintf(os.Stderr, "lampa: %s: %v\n", l.Name, err)
				continue
			}
			out = append(out, b)
		case light.KindYeelight:
			out = append(out, yeelight.New(l.IP, info(l), l.Music))
		default:
			fmt.Fprintf(os.Stderr, "lampa: %s: unknown kind %q, skipped\n", l.Name, l.Kind)
		}
	}
	if len(cfg.Lamps) == 0 {
		for i := 1; i <= 4; i++ {
			out = append(out, light.NewVirtual(light.Info{ID: fmt.Sprint("v", i), Name: fmt.Sprint("virtual ", i), MaxFPS: 30}))
		}
	}
	return out
}

// disabledIDs lists lamps the user switched off; the engine keeps them
// listed but never touches them.
func disabledIDs(cfg config.Config) []string {
	var ids []string
	for _, l := range cfg.Lamps {
		if l.Disabled {
			ids = append(ids, l.ID)
		}
	}
	return ids
}

func findTap(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "lampa-tap")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("lampa-tap"); err == nil {
		return p
	}
	return "lampa-tap"
}

func paletteIndex(name string) int {
	for i, p := range color.Palettes {
		if p.Name == name {
			return i
		}
	}
	return 0
}

func run(args []string) int {
	fs := flag.NewFlagSet("lampa", flag.ExitOnError)
	dry := fs.Bool("dry", false, "no real lamps: virtual lamps on screen only")
	wavPath := fs.String("wav", "", "play this WAV file instead of system audio")
	cfgPath := fs.String("config", config.DefaultPath(), "config file")
	tapFlag := fs.String("tap", "", "path to lampa-tap")
	fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lampa: config: %v\n", err)
		return 1
	}
	eng := engine.New(engine.Options{
		Lights:      buildLights(cfg, *dry),
		Disabled:    disabledIDs(cfg),
		Mode:        modes.Index(cfg.Mode),
		Palette:     paletteIndex(cfg.Palette),
		Sensitivity: cfg.Sensitivity,
		Latency:     time.Duration(cfg.LatencyMS) * time.Millisecond,
	})
	ctx, cancel := signal.NotifyContext(context.Background(), shutdownSignals...)
	defer cancel()
	eng.Start(ctx)

	ctl := &controller{Engine: eng, tapPath: findTap(*tapFlag), cfgPath: *cfgPath,
		blocks: make(chan audio.Block, 64), cfg: cfg, dry: *dry}

	if *wavPath != "" {
		f, err := os.Open(*wavPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lampa: %v\n", err)
			return 1
		}
		rate, samples, err := audio.ReadWAV(f)
		f.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "lampa: %v\n", err)
			return 1
		}
		ctl.srcName = filepath.Base(*wavPath)
		go (&audio.WAVSource{Rate: rate, Samples: samples, Realtime: true}).Run(ctx, ctl.blocks)
	} else {
		var app *audio.App
		if cfg.SourceBundle != "" { // reopen the last chosen app if it is playing
			if apps, err := audio.ListApps(ctx, ctl.tapPath); err == nil {
				for i := range apps {
					if apps[i].BundleID == cfg.SourceBundle {
						app = &apps[i]
					}
				}
			}
		}
		ctl.SetSource(app)
	}
	go eng.Run(ctx, ctl.blocks)

	p := tea.NewProgram(tui.New(ctl), tea.WithAltScreen(), tea.WithContext(ctx))
	_, runErr := p.Run()

	cancel()
	ctl.stop()
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 4*time.Second)
	eng.Close(closeCtx)
	closeCancel()

	v := eng.View()
	ctl.mu.Lock()
	cfg = ctl.cfg
	ctl.mu.Unlock()
	cfg.Mode, cfg.Palette = v.Mode, v.Palette
	cfg.Sensitivity, cfg.LatencyMS = v.Sensitivity, int(v.Latency.Milliseconds())
	if err := config.Save(*cfgPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "lampa: save config: %v\n", err)
	}
	if runErr != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "lampa: %v\n", runErr)
		return 1
	}
	return 0
}

func runDiscover(args []string) int {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	save := fs.Bool("save", false, "add found lamps to the config")
	cfgPath := fs.String("config", config.DefaultPath(), "config file")
	fs.Parse(args)
	found, err := discovery.Discover(context.Background(), 3*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lampa: %v\n", err)
		return 1
	}
	if len(found) == 0 {
		fmt.Println("ламп не найдено (WiZ: включено ли локальное управление? Yeelight: LAN Control в приложении)")
	}
	for _, f := range found {
		music := ""
		if f.Kind == light.KindYeelight {
			music = " music mode: нет"
			if f.Music {
				music = " music mode: да"
			}
		}
		fmt.Printf("%-9s %-16s %-20s %s%s\n", f.Kind, f.IP, f.Name, f.Model, music)
	}
	if *save {
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lampa: %v\n", err)
			return 1
		}
		n := cfg.Merge(found)
		if err := config.Save(*cfgPath, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "lampa: %v\n", err)
			return 1
		}
		fmt.Printf("добавлено в %s: %d\n", *cfgPath, n)
	}
	return 0
}
