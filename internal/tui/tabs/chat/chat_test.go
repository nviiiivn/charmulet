package chat

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/nviiiivn/charmulet/internal/core/provider"
	"github.com/nviiiivn/charmulet/internal/tui/tabs"
	"github.com/nviiiivn/charmulet/internal/tui/theme"
)

func TestSessionTitle(t *testing.T) {
	cases := map[string]string{
		"qwen3.5:9b": "QWEN3.5",
		"hf.co/NousResearch/Hermes-4.3-36B-GGUF:q4_K_M": "HERMES-4.3-36…",
		"gemma4:e4b": "GEMMA4",
	}
	for in, want := range cases {
		if got := sessionTitle(in); got != want {
			t.Errorf("sessionTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestTokenBatchesUntilFlush pins the 30ms coalescing: a token must NOT
// repaint the transcript; the flush tick materializes the whole tail once.
func TestTokenBatchesUntilFlush(t *testing.T) {
	m := New(theme.Default(), nil, "test")
	m.Resize(80, 24)
	m.streaming = true

	tab, cmd := m.Update(tokenMsg{id: m.id, text: "hello"})
	if tab != tabs.Tab(m) {
		t.Fatal("update must return the same tab")
	}
	if cmd == nil {
		t.Fatal("token must arm pump (+flush)")
	}
	if got := m.vp.View(); strings.Contains(got, "hello") {
		t.Fatal("token repaint leaked before flush tick")
	}

	_, _ = m.Update(flushMsg{id: m.id})
	if got := m.vp.View(); !strings.Contains(got, "hello") {
		t.Fatal("flush must materialize the streamed tail")
	}

	// foreign session ids are ignored entirely
	_, _ = m.Update(tokenMsg{id: "other", text: "x"})
	if strings.Contains(m.buf.String(), "x") {
		t.Fatal("foreign token leaked into buffer")
	}
}

// TestRenameFlow drives the ctrl+r title editor: enter mode, type, commit,
// and esc-cancel restoring nothing.
func TestRenameFlow(t *testing.T) {
	m := New(theme.Default(), nil, "test")
	m.Resize(80, 24)

	if _, _ = m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}); !m.renaming {
		t.Fatal("ctrl+r must open the title editor")
	}
	if _, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}); m.renameBuf != "CHATx" {
		t.Fatalf("typed rune must append: %q", m.renameBuf)
	}
	if _, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"}); m.renameBuf != "CHATxz" {
		t.Fatalf("buffer = %q", m.renameBuf)
	}
	if _, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace}); m.renameBuf != "CHATx" {
		t.Fatalf("backspace must trim: %q", m.renameBuf)
	}
	if _, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); m.renaming {
		t.Fatal("enter must commit the rename")
	}
	if m.Title() != "CHATx" {
		t.Fatalf("title = %q", m.Title())
	}

	// esc cancels a rename without touching the title
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if !m.Esc() || m.renaming {
		t.Fatal("esc must close the editor")
	}
	if m.Title() != "CHATx" {
		t.Fatalf("canceled rename leaked: %q", m.Title())
	}
}

// TestExportWritesMarkdown pins the ctrl+e export path: file exists, holds the
// transcript turns.
func TestExportWritesMarkdown(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CHARMULET_EXPORT_DIR", dir)

	m := New(theme.Default(), nil, "test")
	m.hist = append(m.hist,
		provider.Message{Role: "user", Content: "hello there"},
		provider.Message{Role: "assistant", Content: "general kenobi"},
	)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+e must arm the export cmd")
	}
	msg := cmd()
	em, ok := msg.(exportedMsg)
	if !ok {
		t.Fatalf("export cmd yielded %T", msg)
	}
	if em.err != nil {
		t.Fatalf("export: %v", em.err)
	}
	b, err := os.ReadFile(em.path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "## you") || !strings.Contains(s, "hello there") ||
		!strings.Contains(s, "## charm") || !strings.Contains(s, "general kenobi") {
		t.Fatalf("transcript missing from export:\n%s", s)
	}

	// flash lands in the log once the msg round-trips
	m.Update(em)
	found := false
	for _, l := range m.renderLog() {
		if strings.Contains(l, em.path) {
			found = true
		}
	}
	if !found {
		t.Fatal("export flash missing from transcript")
	}
}

// TestTypingReachesTheBox pins that plain keys land in the input box, and
// that "e" and "i" are text, not the export/rename shortcuts.
func TestTypingReachesTheBox(t *testing.T) {
	m := New(theme.Default(), nil, "test")
	m.Resize(80, 24)
	for _, r := range "hi e3" {
		if _, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)}); cmd != nil {
			if _, ok := cmd().(exportedMsg); ok {
				t.Fatal("typing e must not export")
			}
		}
	}
	if m.renaming {
		t.Fatal("typing i must not open the title editor")
	}
	if got := m.ta.Value(); got != "hi e3" {
		t.Fatalf("box = %q, want %q", got, "hi e3")
	}
	if !m.Typing() {
		t.Fatal("a box with text must report Typing")
	}
	// no provider here, so nothing is sent, but enter must not add a line
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.ta.Value(); got != "hi e3" {
		t.Fatalf("enter must never insert a newline, box = %q", got)
	}
}
