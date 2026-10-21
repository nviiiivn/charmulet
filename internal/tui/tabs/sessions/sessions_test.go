package sessions

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/nviiiivn/charmulet/internal/core/session"
	"github.com/nviiiivn/charmulet/internal/tui/tabs"
	"github.com/nviiiivn/charmulet/internal/tui/tabs/chat"
	"github.com/nviiiivn/charmulet/internal/tui/theme"
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

func key(s string) tea.KeyPressMsg {
	if s == "\r" {
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

// TestListAndResume: Activate lists rows; enter produces a SpawnMsg whose
// tab is a resumed chat carrying the stored transcript.
func TestListAndResume(t *testing.T) {
	st := openStore(t)
	meta, err := st.Create("QWEN3.5", "qwen3.5:9b")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.Append(meta.ID, "user", "hello"); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := st.Append(meta.ID, "assistant", "hi back"); err != nil {
		t.Fatalf("append: %v", err)
	}

	m := New(theme.Default(), nil, st)
	m.Resize(100, 30)

	cmd := m.Activate()
	msg := cmd()
	lm, ok := msg.(listedMsg)
	if !ok {
		t.Fatalf("Activate cmd yielded %T", msg)
	}
	if len(lm.metas) != 1 || lm.err != nil {
		t.Fatalf("listed %+v err=%v", lm.metas, lm.err)
	}
	// Route the listing back through Update like the shell would.
	if _, c := m.Update(lm); c != nil {
		t.Fatal("listedMsg should not emit a cmd")
	}

	// enter → cmd → SpawnMsg → resumed chat tab
	tab, enterCmd := m.Update(key("\r"))
	if enterCmd == nil {
		t.Fatal("enter produced no cmd")
	}
	msg = enterCmd()
	sp, ok := msg.(tabs.SpawnMsg)
	if !ok {
		t.Fatalf("resume cmd yielded %T", msg)
	}
	cm, ok := sp.Tab.(*chat.Model)
	if !ok {
		t.Fatalf("spawned tab is %T", sp.Tab)
	}
	if cm.Title() != "QWEN3.5" {
		t.Fatalf("resumed title %q", cm.Title())
	}
	if cm.ID() != "chat" {
		t.Fatalf("resumed id %q", cm.ID())
	}
	if tab.(*Model).sel != 0 {
		t.Fatalf("selection moved unexpectedly")
	}
}

// TestEmptyAndRefresh: empty store shows the empty state; r re-lists.
func TestEmptyAndRefresh(t *testing.T) {
	st := openStore(t)
	m := New(theme.Default(), nil, st)
	m.Resize(100, 30)

	if cmd := m.Activate(); cmd == nil {
		t.Fatal("Activate should list")
	}
	msg := m.Activate()()
	if lm := msg.(listedMsg); len(lm.metas) != 0 || lm.err != nil {
		t.Fatalf("expected empty list, got %+v", lm)
	}

	v := m.View()
	if v == "" {
		t.Fatal("empty view rendered nothing")
	}
	if _, cmd := m.Update(key("r")); cmd == nil {
		t.Fatal("r should trigger refresh cmd")
	}
}
