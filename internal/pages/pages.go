// Package pages holds user-defined dashboard pages: TOML files that lay
// out registered components into a grid (wtfutil-style — the page file is
// the config, components are modules).
package pages

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/pelletier/go-toml/v2"
)

// Widget is one component instance on a page.
type Widget struct {
	Kind    string `toml:"kind"`              // component type (registry key)
	Title   string `toml:"title,omitempty"`   // box title override
	Machine string `toml:"machine,omitempty"` // host filter (system/disk/ollama)
	Text    string `toml:"text,omitempty"`    // note body
}

// Page is a named grid of widgets.
type Page struct {
	Name    string   `toml:"name"`
	Columns int      `toml:"columns"` // 1-3
	Widgets []Widget `toml:"widget"`  // flow order = fill order
}

// Dir is where page files live (CHARMULET_CONFIG_DIR-aware).
func Dir() string {
	if v := os.Getenv("CHARMULET_CONFIG_DIR"); v != "" {
		return filepath.Join(v, "pages")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "pages"
	}
	return filepath.Join(home, ".config", "charmulet", "pages")
}

// Default is the home page written on first use: the machines that matter
// front and center, ollama status beside them.
func Default() Page {
	return Page{
		Name:    "home",
		Columns: 2,
		Widgets: []Widget{
			{Kind: "system", Machine: "demo-desktop"},
			{Kind: "system", Machine: "demo-server"},
			{Kind: "ollama", Machine: "demo-desktop"},
			{Kind: "clock"},
			{Kind: "disk", Machine: "demo-laptop"},
			{Kind: "disk", Machine: "demo-tablet"},
		},
	}
}

// EnsureDefault writes the default page when the pages dir has no TOML at
// all, so the PAGES tab never opens empty. Returns the page it wrote.
func EnsureDefault() (Page, bool) {
	dir := Dir()
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".toml" {
				return Page{}, false // user has pages already
			}
		}
	}
	p := Default()
	if err := Save(p); err != nil {
		return p, false
	}
	return p, true
}

// Save marshals a page to <dir>/<name>.toml.
func Save(p Page) error {
	if err := validName(p.Name); err != nil {
		return err
	}
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	b, err := toml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(Dir(), p.Name+".toml"), b, 0o644)
}

// List loads every page in Dir, sorted by name. Corrupt files are skipped,
// not fatal. Columns defaults to 2 when unset.
func List() []Page {
	out := []Page{}
	entries, err := os.ReadDir(Dir())
	if err != nil {
		return out
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".toml" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, n := range names {
		var p Page
		b, err := os.ReadFile(filepath.Join(Dir(), n))
		if err != nil {
			continue
		}
		if err := toml.Unmarshal(b, &p); err != nil || p.Name == "" {
			continue
		}
		if p.Columns < 1 || p.Columns > 3 {
			p.Columns = 2
		}
		out = append(out, p)
	}
	return out
}

func validName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid page name %q", name)
	}
	if filepath.Base(name) != name {
		return fmt.Errorf("invalid page name %q", name)
	}
	return nil
}
