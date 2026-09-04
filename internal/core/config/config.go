// Package config resolves layered configuration: global -> project -> runtime
// overrides. Live reload via config.changed events lands in P4.
package config

import (
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

type Config struct {
	Theme     string `toml:"theme"`
	OllamaURL string `toml:"ollama_url"`
	ChatModel string `toml:"chat_model"`

	DataDir string `toml:"-"`
}

// Paths returns config locations in precedence order (later wins).
func Paths() []string {
	if root := os.Getenv("CHARMULET_CONFIG_DIR"); root != "" {
		return []string{
			filepath.Join(root, "charmulet.toml"),
			".charmulet.toml",
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return []string{
			filepath.Join(home, ".config", "charmulet", "charmulet.toml"),
			".charmulet.toml",
		}
	}
	return []string{".charmulet.toml"}
}

// Save persists the user-level config layer (creating the dir on demand).
func Save(cfg *Config) error {
	var p string
	if root := os.Getenv("CHARMULET_CONFIG_DIR"); root != "" {
		p = filepath.Join(root, "charmulet.toml")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		p = filepath.Join(home, ".config", "charmulet", "charmulet.toml")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
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
		if layer.OllamaURL != "" {
			cfg.OllamaURL = layer.OllamaURL
		}
		if layer.ChatModel != "" {
			cfg.ChatModel = layer.ChatModel
		}
		cfg.DataDir = filepath.Dir(p)
	}
	if cfg.Theme == "" {
		cfg.Theme = "pride"
	}
	if cfg.OllamaURL == "" {
		if v := os.Getenv("CHARMULET_OLLAMA_URL"); v != "" {
			cfg.OllamaURL = v
		} else {
			cfg.OllamaURL = "http://192.0.2.24:11434"
		}
	}
	if cfg.ChatModel == "" {
		cfg.ChatModel = "qwen3.5:9b"
	}
	return cfg, nil
}
