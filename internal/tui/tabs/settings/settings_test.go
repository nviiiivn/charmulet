package settings

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/nviiiivn/charmulet/internal/core/config"
	"github.com/nviiiivn/charmulet/internal/tui/tabs"
	"github.com/nviiiivn/charmulet/internal/tui/theme"
)

// TestApplyThemeLiveAndPersisted: enter on a selected theme emits
// SetThemeMsg and the choice lands in charmulet.toml.
func TestApplyThemeLiveAndPersisted(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CHARMULET_CONFIG_DIR", root)

	// Two theme files on disk to enumerate.
	td := filepath.Join(root, "themes")
	if err := os.MkdirAll(td, 0o755); err != nil {
		t.Fatal(err)
	}
	alt := `name = "alt"
[colors]
accent = "#ff0000"
`
	if err := os.WriteFile(filepath.Join(td, "alt.toml"), []byte(alt), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(theme.Default(), nil, td, "test-version")
	m.Resize(100, 30)
	if cmd := m.Activate(); cmd != nil {
		t.Fatal("Activate should not emit a cmd")
	}
	if len(m.themes) < 2 {
		t.Fatalf("want 2 themes enumerated, got %d", len(m.themes))
	}

	// j moves to the alt theme, enter applies it.
	tab, cmd := m.Update(tea.KeyPressMsg{Code: 'j'})
	if cmd != nil {
		t.Fatal("j should not emit a cmd")
	}
	tab, cmd = tab.(*Model).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should emit SetThemeMsg cmd")
	}
	msg := cmd()
	stm, ok := msg.(tabs.SetThemeMsg)
	if !ok {
		t.Fatalf("cmd yielded %T", msg)
	}
	if stm.Theme.Name != "alt" {
		t.Fatalf("applied %q, want alt", stm.Theme.Name)
	}
	_ = tab

	// Persisted layer should now say alt.
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Theme != "alt" {
		t.Fatalf("persisted theme %q, want alt", cfg.Theme)
	}

	// Round-trip: the tab restyles on SetThemeMsg.
	if _, c := m.Update(tabs.SetThemeMsg{Theme: stm.Theme}); c != nil {
		t.Fatal("SetThemeMsg should not emit a cmd")
	}
	if m.th.Name != "alt" {
		t.Fatalf("tab theme not updated: %q", m.th.Name)
	}
}

// TestViewRenders: view paints both theme list and config block.
func TestViewRenders(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CHARMULET_CONFIG_DIR", root)
	m := New(theme.Default(), &config.Config{OllamaURL: "http://x:1", ChatModel: "m"}, filepath.Join(root, "themes"), "v")
	m.Resize(80, 24)
	m.Activate()
	if v := m.View(); v == "" {
		t.Fatal("empty view")
	}
}
