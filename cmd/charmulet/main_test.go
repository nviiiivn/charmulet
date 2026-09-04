package main

import (
	"path/filepath"
	"testing"

	"github.com/nvii/charmulet/internal/core/config"
	"github.com/nvii/charmulet/internal/core/session"
	"github.com/nvii/charmulet/internal/tui/theme"
)

func openStore(t *testing.T) *session.Store {
	t.Helper()
	st, err := session.OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestResumeTabSubstrings(t *testing.T) {
	st := openStore(t)
	q, err := st.Create("QWEN3.5", "qwen3.5:9b")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.Append(q.ID, "user", "hi"); err != nil {
		t.Fatalf("append: %v", err)
	}
	g, err := st.Create("GEMMA4", "gemma4:e4b")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.Append(g.ID, "assistant", "yo"); err != nil {
		t.Fatalf("append: %v", err)
	}

	th := theme.Default()

	// title substring
	tab := resumeTab(st, th, nil, "gemma")
	if tab == nil || tab.Title() != "GEMMA4" {
		t.Fatalf("substring resume: %v", tab)
	}

	// model substring
	tab = resumeTab(st, th, nil, "qwen3.5")
	if tab == nil || tab.Title() != "QWEN3.5" {
		t.Fatalf("model resume: %v", tab)
	}

	// no match → nil (caller starts a fresh chat + prints a notice)
	if tab := resumeTab(st, th, nil, "does-not-exist"); tab != nil {
		t.Fatalf("no-match must be nil, got %v", tab)
	}
}

func TestResumeTabLastAndEmpty(t *testing.T) {
	st := openStore(t)
	meta, err := st.Create("SOLO", "deepseek-coder-v2:16b")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.Append(meta.ID, "user", "ping"); err != nil {
		t.Fatalf("append: %v", err)
	}

	th := theme.Default()
	for _, q := range []string{"last", ""} {
		tab := resumeTab(st, th, nil, q)
		if tab == nil || tab.Title() != "SOLO" {
			t.Fatalf("resume %q: %v", q, tab)
		}
	}
}

func TestResumeTabNilStore(t *testing.T) {
	if tab := resumeTab(nil, theme.Default(), nil, "anything"); tab != nil {
		t.Fatalf("nil store must resume nothing, got %v", tab)
	}
}

// TestConfigExistsRespectsEnv: no file → false; file present → true.
func TestConfigExistsRespectsEnv(t *testing.T) {
	t.Setenv("CHARMULET_CONFIG_DIR", t.TempDir())
	if configExists() {
		t.Fatal("empty config dir must report no config")
	}
	cfg := &config.Config{OllamaURL: "http://x", ChatModel: "m", Theme: "pride"}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !configExists() {
		t.Fatal("saved config must be found")
	}
}
