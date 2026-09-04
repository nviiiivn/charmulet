package launch

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/nvii/charmulet/internal/core/provider"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/tabs/chat"
	"github.com/nvii/charmulet/internal/tui/theme"
)

type fakeProv struct{}

func (fakeProv) Name() string     { return "fake" }
func (fakeProv) Models() []string { return []string{"alpha:9b", "beta:3b"} }
func (fakeProv) Complete(ctx context.Context, model string, msgs []provider.Message) (provider.Stream, error) {
	return nil, nil
}

func key(s string) tea.KeyPressMsg {
	if s == "enter" {
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func TestPaneSwitchAndSpawn(t *testing.T) {
	th := theme.Default()
	m := New(th, fakeProv{}, nil)

	// Activate kicks off the model load; execute cmd and feed msg back.
	cmd := m.Activate()
	if cmd == nil {
		t.Fatal("Activate should start loading models")
	}
	if msg := cmd(); msg != nil {
		m.Update(msg)
	}
	if len(m.model) != 2 {
		t.Fatalf("want 2 models, got %d", len(m.model))
	}

	// enter on left pane (ollama selected) -> right pane
	if _, c := m.Update(key("enter")); c != nil {
		t.Fatal("pane switch should not emit a cmd")
	}
	if m.pane != 1 {
		t.Fatalf("enter should focus model pane, pane=%d", m.pane)
	}

	// j moves selection to second model
	m.Update(key("j"))
	if m.selR != 1 {
		t.Fatalf("j should move model cursor, selR=%d", m.selR)
	}

	// enter spawns a chat session tab via SpawnMsg
	_, spawnCmd := m.Update(key("enter"))
	if spawnCmd == nil {
		t.Fatal("enter on model should produce a spawn cmd")
	}
	msg := spawnCmd()
	spawn, ok := msg.(tabs.SpawnMsg)
	if !ok {
		t.Fatalf("cmd should yield tabs.SpawnMsg, got %T", msg)
	}
	cm, ok := spawn.Tab.(*chat.Model)
	if !ok {
		t.Fatalf("spawned tab should be *chat.Model, got %T", spawn.Tab)
	}
	if cm.Title() != "BETA" {
		t.Fatalf("session title should be BETA, got %q", cm.Title())
	}
}

func TestEnterWhileLoadingQueuesPaneHop(t *testing.T) {
	m := New(theme.Default(), fakeProv{}, nil)
	m.loading = true // load in flight
	m.Update(key("enter"))
	if m.wantR != true {
		t.Fatal("enter while loading should set wantR")
	}
	m.Update(modelsMsg{models: []string{"x"}})
	if m.pane != 1 {
		t.Fatalf("models arriving should honor wantR, pane=%d", m.pane)
	}
}
