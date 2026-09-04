package session

import (
	"path/filepath"
	"testing"
	"time"
)

func open(t *testing.T) *Store {
	t.Helper()
	st, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestRoundtrip(t *testing.T) {
	st := open(t)

	meta, err := st.Create("QWEN3.5", "qwen3.5:9b")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if meta.ID == 0 {
		t.Fatal("expected nonzero session id")
	}

	if err := st.Append(meta.ID, "user", "hello there"); err != nil {
		t.Fatalf("append user: %v", err)
	}
	if err := st.Append(meta.ID, "assistant", "general kenobi"); err != nil {
		t.Fatalf("append assistant: %v", err)
	}
	// Empty content must be a no-op, not a row.
	if err := st.Append(meta.ID, "user", ""); err != nil {
		t.Fatalf("append empty: %v", err)
	}

	// Second, older session so List ordering is observable.
	older, err := st.Create("GEMMA4", "gemma4:e4b")
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	if err := st.Append(older.ID, "user", "hi"); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	// SQLite timestamp resolution can equal-out in fast tests; force ordering.
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := st.db.Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`, future, older.ID); err != nil {
		t.Fatalf("bump: %v", err)
	}

	metas, err := st.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(metas) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(metas))
	}
	if metas[0].Title != "GEMMA4" {
		t.Fatalf("newest-first order broken: %q", metas[0].Title)
	}
	if metas[0].MsgCount != 1 || metas[1].MsgCount != 2 {
		t.Fatalf("msg counts wrong: %+v", metas)
	}

	msgs, err := st.Messages(meta.ID)
	if err != nil {
		t.Fatalf("messages: %v", err)
	}
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[0].Content != "hello there" ||
		msgs[1].Role != "assistant" || msgs[1].Content != "general kenobi" {
		t.Fatalf("transcript mismatch: %+v", msgs)
	}
}

func TestNilStoreNoOps(t *testing.T) {
	var st *Store
	if _, err := st.Create("x", "y"); err != nil {
		t.Fatalf("nil create: %v", err)
	}
	if err := st.Append(0, "user", "hi"); err != nil {
		t.Fatalf("nil append: %v", err)
	}
	if m, err := st.List(); m != nil || err != nil {
		t.Fatalf("nil list: %v %v", m, err)
	}
	if m, err := st.Messages(1); m != nil || err != nil {
		t.Fatalf("nil messages: %v %v", m, err)
	}
}

func TestRename(t *testing.T) {
	st := open(t)
	meta, err := st.Create("OLD", "m")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.Rename(meta.ID, "NEW"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	metas, _ := st.List()
	if metas[0].Title != "NEW" {
		t.Fatalf("rename not persisted: %q", metas[0].Title)
	}
	if err := st.Rename(meta.ID, "  "); err == nil {
		t.Fatal("empty rename must error")
	}
}
