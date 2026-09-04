// Package agents stores charmulet agent definitions as TOML files under
// ~/.config/charmulet/agents/ — one file per agent, editable at runtime.
package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Agent is a reusable chat persona: model + system prompt (+ future wrapper).
type Agent struct {
	Name         string `toml:"name"`
	Model        string `toml:"model"`
	SystemPrompt string `toml:"system_prompt,omitempty"`
	Wrapper      string `toml:"wrapper,omitempty"`
}

// Dir returns the agents directory, creating it on first use.
// CHARMULET_CONFIG_DIR relocates the whole charmulet config tree (tests).
func Dir() (string, error) {
	root := os.Getenv("CHARMULET_CONFIG_DIR")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".config", "charmulet")
	}
	d := filepath.Join(root, "agents")
	return d, os.MkdirAll(d, 0o755)
}

func agentPath(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid agent name %q", name)
	}
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, name+".toml"), nil
}

// List returns all agents sorted by name. Missing dir = empty list, not error.
func List() ([]Agent, error) {
	d, err := Dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(d)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Agent
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(d, e.Name()))
		if err != nil {
			continue
		}
		var a Agent
		if err := toml.Unmarshal(b, &a); err != nil {
			continue // skip corrupt files rather than break the tab
		}
		if a.Name == "" {
			a.Name = strings.TrimSuffix(e.Name(), ".toml")
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Save writes the agent's TOML file (create or overwrite by name).
func Save(a Agent) error {
	p, err := agentPath(a.Name)
	if err != nil {
		return err
	}
	b, err := toml.Marshal(a)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// Remove deletes an agent file — used only for rename-on-save, never exposed
// as a tab action.
func Remove(name string) error {
	p, err := agentPath(name)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
