// Package wrappers defines docker-backed agent wrappers (hax, crush, qwen,
// claude, codex, opencode, hermes, openclaw, agent-zero, totem) — the locked
// launcher list. Specs ship as builtins and can be overridden or extended
// with TOML files in ~/.config/charmulet/wrappers/.
package wrappers

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Wrapper is a docker-run spec for one agent CLI. Image is deliberately
// empty for builtins until a real image is configured — charmulet never
// invents an image name to pull.
type Wrapper struct {
	Name        string   `toml:"name"`
	Desc        string   `toml:"desc"`
	Image       string   `toml:"image"`
	Cmd         []string `toml:"cmd"`
	Env         []string `toml:"env"`
	Volumes     []string `toml:"volumes"`
	Ports       []string `toml:"ports"`
	Interactive bool     `toml:"interactive"` // wire stdin (line protocol)
}

// builtin returns the locked wrapper list with descriptions from the
// homelab handoff. Images stay empty until she configures them.
func builtin() []Wrapper {
	return []Wrapper{
		{Name: "hax", Desc: "hax agent", Interactive: true},
		{Name: "crush", Desc: "charmbracelet crush — Go agent CLI", Interactive: true},
		{Name: "qwen", Desc: "qwen coding CLI", Interactive: true},
		{Name: "claude", Desc: "claude code CLI", Interactive: true},
		{Name: "codex", Desc: "openai codex CLI", Interactive: true},
		{Name: "opencode", Desc: "opencode agent CLI", Interactive: true},
		{Name: "hermes", Desc: "hermes agent (+server on demo-desktop)", Interactive: true},
		{Name: "openclaw", Desc: "openclaw gateway agent", Interactive: true},
		{Name: "agent-zero", Desc: "agent-zero framework", Interactive: true},
		{Name: "totem", Desc: "totem CLI (config-only on demo-server)", Interactive: true},
	}
}

// Dir returns the wrapper config dir, CHARMULET_CONFIG_DIR-aware.
func Dir() string {
	if v := os.Getenv("CHARMULET_CONFIG_DIR"); v != "" {
		return filepath.Join(v, "wrappers")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".charmulet-wrappers"
	}
	return filepath.Join(home, ".config", "charmulet", "wrappers")
}

func validName(name string) bool {
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return false
	}
	return true
}

func path(name string) string {
	return filepath.Join(Dir(), name+".toml")
}

// List merges builtins with TOML overrides/additions from the config dir,
// sorted by name. A TOML file with the same Name replaces the builtin.
func List() []Wrapper {
	byName := map[string]Wrapper{}
	for _, w := range builtin() {
		byName[w.Name] = w
	}
	entries, err := os.ReadDir(Dir())
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(Dir(), e.Name()))
			if err != nil {
				continue
			}
			var w Wrapper
			if err := toml.Unmarshal(data, &w); err != nil || !validName(w.Name) {
				continue
			}
			byName[w.Name] = w
		}
	}
	out := make([]Wrapper, 0, len(byName))
	for _, w := range byName {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get finds one wrapper by exact name.
func Get(name string) (Wrapper, bool) {
	for _, w := range List() {
		if w.Name == name {
			return w, true
		}
	}
	return Wrapper{}, false
}

// Save writes a wrapper spec to its TOML file.
func Save(w Wrapper) error {
	if !validName(w.Name) {
		return fmt.Errorf("invalid wrapper name %q", w.Name)
	}
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(w)
	if err != nil {
		return err
	}
	return os.WriteFile(path(w.Name), data, 0o644)
}
