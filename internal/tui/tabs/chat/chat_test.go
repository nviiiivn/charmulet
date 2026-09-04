package chat

import (
	"strings"
	"testing"

	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
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
