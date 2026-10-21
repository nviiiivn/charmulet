package launch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/nviiiivn/charmulet/internal/core/provider"
	"github.com/nviiiivn/charmulet/internal/core/wrappers"
	"github.com/nviiiivn/charmulet/internal/tui/tabs"
	"github.com/nviiiivn/charmulet/internal/tui/tabs/chat"
	"github.com/nviiiivn/charmulet/internal/tui/tabs/wrap"
	"github.com/nviiiivn/charmulet/internal/tui/theme"
)

// fakeDockerScript masquerades as docker for the confirm-gate spawn test.
func fakeDockerScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "docker")
	script := `#!/bin/sh
echo "ARGS: $@"
while IFS= read -r line; do echo "ECHO: $line"; done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

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

// TestWrapperConfirmGate pins the docker spawn flow: enter arms the y/n
// gate, n cancels, y spawns a WRAP tab (via the fake docker binary).
func TestWrapperConfirmGate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CHARMULET_CONFIG_DIR", dir)
	t.Setenv("CHARMULET_DOCKER_BIN", fakeDockerScript(t))

	// give one builtin an image so the spawn path can run
	spec := wrappers.Wrapper{Name: "agent-zero", Image: "fake:1", Interactive: true}
	if err := wrappers.Save(spec); err != nil {
		t.Fatalf("save: %v", err)
	}

	m := New(theme.Default(), fakeProv{}, nil)
	m.Resize(100, 30)
	if len(m.fleet) != 10 {
		t.Fatalf("fleet = %d, want 10", len(m.fleet))
	}

	// selL=1 is the first wrapper (sources = ollama + fleet)
	m.selL = 1
	if m.sources()[m.selL].kind != srcWrapper {
		t.Fatalf("source 1 is %v, want wrapper", m.sources()[m.selL].kind)
	}

	// enter arms the confirm gate; gate swallows unrelated keys
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("arming enter must not emit a cmd")
	}
	if m.pending == nil || m.pending.Name != "agent-zero" {
		t.Fatalf("gate not armed: %+v", m.pending)
	}
	if _, c := m.Update(tea.KeyPressMsg{Code: 'x'}); c != nil {
		t.Fatal("gate must swallow unrelated keys")
	}

	// view shows the docker command
	if v := m.View(); !strings.Contains(v, "RUN IN DOCKER?") {
		t.Fatal("confirm overlay missing from view")
	}

	// n cancels cleanly
	_, _ = m.Update(tea.KeyPressMsg{Code: 'n'})
	if m.pending != nil || m.status != "canceled" {
		t.Fatalf("cancel broken: %+v %q", m.pending, m.status)
	}

	// re-arm and spawn for real
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'y'})
	if cmd == nil {
		t.Fatal("y must spawn a wrap tab")
	}
	msg := cmd()
	sp, ok := msg.(tabs.SpawnMsg)
	if !ok {
		t.Fatalf("spawn cmd yielded %T", msg)
	}
	if _, ok := sp.Tab.(*wrap.Model); !ok {
		t.Fatalf("spawned tab is %T, want *wrap.Model", sp.Tab)
	}
}
