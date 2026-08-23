// Package config resolves layered configuration: global -> project -> runtime
// overrides. Live reload via config.changed events lands in P4.
package config

import (
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

type Config struct {
	Theme string `toml:"theme"`

	DataDir string `toml:"-"`
}

// Paths returns config locations in precedence order (later wins).
func Paths() []string {
	if home, err := os.UserHomeDir(); err == nil {
		return []string{
			filepath.Join(home, ".config", "charmulet", "charmulet.toml"),
			".charmulet.toml",
		}
	}
	return []string{".charmulet.toml"}
}

// Load merges every existing layer in order.
func Load() (*Config, error) {
	cfg := &Config{}
	for _, p := range Paths() {
		raw, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return cfg, err
		}
		var layer Config
		if err := toml.Unmarshal(raw, &layer); err != nil {
			return cfg, err
		}
		if layer.Theme != "" {
			cfg.Theme = layer.Theme
		}
		cfg.DataDir = filepath.Dir(p)
	}
	if cfg.Theme == "" {
		cfg.Theme = "cartridge"
	}
	return cfg, nil
}
