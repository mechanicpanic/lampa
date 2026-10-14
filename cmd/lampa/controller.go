package main

import (
	"context"
	"sync"
	"time"

	"lampa/internal/audio"
	"lampa/internal/config"
	"lampa/internal/discovery"
	"lampa/internal/engine"
)

// controller adds audio-source switching and discovery to the engine.
type controller struct {
	*engine.Engine
	tapPath string
	cfgPath string
	blocks  chan audio.Block
	dry     bool

	mu        sync.Mutex
	cfg       config.Config
	srcCancel context.CancelFunc
	srcName   string
	srcErr    string
}

// tapError records the helper's last error so the TUI can show it
// (stderr is invisible under the alt screen).
func (c *controller) tapError(err error) {
	c.mu.Lock()
	c.srcErr = err.Error()
	c.mu.Unlock()
}

func (c *controller) Sources(ctx context.Context) ([]audio.App, error) {
	return audio.ListApps(ctx, c.tapPath)
}

func (c *controller) SourceName() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.srcErr != "" && !c.View().AudioAlive {
		return c.srcName + " · " + c.srcErr
	}
	return c.srcName
}

// SetSource (re)starts the tap; nil = whole system.
func (c *controller) SetSource(app *audio.App) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.srcCancel != nil {
		c.srcCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.srcCancel = cancel
	src := &audio.TapSource{Path: c.tapPath, OnError: c.tapError}
	c.srcName, c.cfg.SourceBundle, c.srcErr = "whole system", "", ""
	if app != nil {
		src.PID = app.PID
		c.srcName, c.cfg.SourceBundle = app.Name, app.BundleID
	}
	go src.Run(ctx, c.blocks)
}

func (c *controller) Discover(ctx context.Context) (int, error) {
	found, err := discovery.Discover(ctx, 2*time.Second)
	if err != nil {
		return 0, err
	}
	c.mu.Lock()
	before := len(c.cfg.Lamps)
	added := c.cfg.Merge(found)
	newLamps := append([]config.Lamp(nil), c.cfg.Lamps[before:]...)
	err = config.Save(c.cfgPath, c.cfg)
	c.mu.Unlock()
	if !c.dry && len(newLamps) > 0 {
		for _, l := range buildLights(config.Config{Lamps: newLamps}, false) {
			c.AddLight(l)
		}
	}
	return added, err
}

func (c *controller) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.srcCancel != nil {
		c.srcCancel()
	}
}

// SetLampEnabled switches a lamp on/off and remembers the choice.
func (c *controller) SetLampEnabled(id string, on bool) {
	c.SetEnabled(id, on)
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.cfg.Lamps {
		if c.cfg.Lamps[i].ID == id {
			c.cfg.Lamps[i].Disabled = !on
		}
	}
	config.Save(c.cfgPath, c.cfg)
}

// SetLang remembers the UI language chosen in the TUI.
func (c *controller) SetLang(lang string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg.Lang = lang
	config.Save(c.cfgPath, c.cfg)
}
