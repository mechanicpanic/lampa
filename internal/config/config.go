// Package config loads and saves ~/.config/lampa/config.toml.
package config

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"lampa/internal/discovery"
)

type Lamp struct {
	Kind     string  `toml:"kind"`
	IP       string  `toml:"ip"`
	ID       string  `toml:"id"`
	Name     string  `toml:"name"`
	Model    string  `toml:"model,omitempty"`
	Role     string  `toml:"role,omitempty"`
	Music    bool    `toml:"music"`
	MaxFPS   float64 `toml:"max_fps,omitempty"`
	Disabled bool    `toml:"disabled,omitempty"`
}

type Config struct {
	Mode         string  `toml:"mode"`
	Palette      string  `toml:"palette"`
	Sensitivity  float64 `toml:"sensitivity"`
	LatencyMS    int     `toml:"latency_ms"`
	SourceBundle string  `toml:"source_bundle,omitempty"`
	Lamps        []Lamp  `toml:"lamp"`
}

func Default() Config {
	return Config{Mode: "pulse", Palette: "sunset", Sensitivity: 0.5, LatencyMS: 60}
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(home, ".config", "lampa", "config.toml")
}

func Load(path string) (Config, error) {
	c := Default()
	_, err := toml.DecodeFile(path, &c)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	return c, err
}

func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c *Config) Merge(found []discovery.Found) (added int) {
	for _, f := range found {
		known := false
		for i := range c.Lamps {
			l := &c.Lamps[i]
			if l.Kind == string(f.Kind) && l.ID == f.ID {
				l.IP, l.Model, l.Music = f.IP, f.Model, f.Music
				known = true
				break
			}
		}
		if !known {
			c.Lamps = append(c.Lamps, Lamp{Kind: string(f.Kind), IP: f.IP, ID: f.ID, Name: f.Name, Model: f.Model, Music: f.Music})
			added++
		}
	}
	return added
}
