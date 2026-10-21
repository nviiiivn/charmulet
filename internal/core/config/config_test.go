package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CHARMULET_CONFIG_DIR", dir)
	// neutralize any ambient env default leaking into assertions
	t.Setenv("CHARMULET_OLLAMA_URL", "")

	in := &Config{Theme: "lolcat", OllamaURL: "http://demo-desktop:11434", ChatModel: "gemma4:e4b"}
	if err := Save(in); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "charmulet.toml")); err != nil {
		t.Fatalf("config file missing: %v", err)
	}

	out, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if out.Theme != "lolcat" || out.OllamaURL != "http://demo-desktop:11434" || out.ChatModel != "gemma4:e4b" {
		t.Fatalf("roundtrip mismatch: %+v", out)
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CHARMULET_CONFIG_DIR", dir) // dir has no charmulet.toml
	t.Setenv("CHARMULET_OLLAMA_URL", "http://env-wins:11434")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Theme != "pride" {
		t.Fatalf("default theme: %q", cfg.Theme)
	}
	if cfg.OllamaURL != "http://env-wins:11434" {
		t.Fatalf("env url should win: %q", cfg.OllamaURL)
	}

	t.Setenv("CHARMULET_OLLAMA_URL", "")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.OllamaURL != "http://localhost:11434" {
		t.Fatalf("hardcoded fallback: %q", cfg.OllamaURL)
	}
	if cfg.ChatModel != "qwen3.5:9b" {
		t.Fatalf("default model: %q", cfg.ChatModel)
	}
}
