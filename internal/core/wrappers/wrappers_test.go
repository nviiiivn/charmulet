package wrappers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinsListAndMerge(t *testing.T) {
	t.Setenv("CHARMULET_CONFIG_DIR", t.TempDir())

	got := List()
	if len(got) != 10 {
		t.Fatalf("want 10 builtin wrappers, got %d", len(got))
	}
	names := map[string]bool{}
	for _, w := range got {
		names[w.Name] = true
		if w.Image != "" {
			t.Fatalf("builtin %s must ship image-less, got %q", w.Name, w.Image)
		}
	}
	for _, want := range []string{"hax", "crush", "qwen", "claude", "codex", "opencode", "hermes", "openclaw", "agent-zero", "totem"} {
		if !names[want] {
			t.Fatalf("missing builtin %q", want)
		}
	}

	// A TOML override merges over the builtin entry and re-sorts stably.
	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	override := Wrapper{Name: "claude", Desc: "her claude", Image: "claude:latest", Interactive: true, Cmd: []string{"claude"}}
	if err := Save(override); err != nil {
		t.Fatalf("save: %v", err)
	}
	got = List()
	for _, w := range got {
		if w.Name == "claude" {
			if w.Image != "claude:latest" || w.Desc != "her claude" {
				t.Fatalf("override not applied: %+v", w)
			}
		}
	}
	if got[0].Name != "agent-zero" {
		t.Fatalf("list not sorted: %q", got[0].Name)
	}

	g, ok := Get("claude")
	if !ok || g.Image != "claude:latest" {
		t.Fatalf("get: %v %+v", ok, g)
	}
	if _, ok := Get("nope"); ok {
		t.Fatal("get unknown should miss")
	}
}

func TestInvalidNamesRejected(t *testing.T) {
	t.Setenv("CHARMULET_CONFIG_DIR", t.TempDir())
	for _, bad := range []string{"../evil", "a/b", `a\b`, "..", ""} {
		if err := Save(Wrapper{Name: bad}); err == nil {
			t.Fatalf("Save(%q) should fail", bad)
		}
	}
}

func TestSummary(t *testing.T) {
	w := Wrapper{Name: "hermes", Image: "hermes:1", Cmd: []string{"hermes", "serve"}, Env: []string{"A=1"}, Interactive: true}
	s := Summary(w)
	for _, want := range []string{"hermes:1", "hermes serve", "-e A=1", "-i"} {
		if !strings.Contains(s, want) {
			t.Fatalf("summary %q missing %q", s, want)
		}
	}
	if s2 := Summary(Wrapper{Name: "x"}); !strings.Contains(s2, "no image configured") {
		t.Fatalf("empty-image summary wrong: %q", s2)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CHARMULET_CONFIG_DIR", dir)

	w := Wrapper{Name: "qwen", Desc: "d", Image: "qwen:9", Cmd: []string{"qwen", "chat"},
		Env: []string{"K=V"}, Volumes: []string{"/a:/b"}, Ports: []string{"8080:80"}, Interactive: true}
	if err := Save(w); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok := Get("qwen")
	if !ok {
		t.Fatal("get qwen: miss")
	}
	if got.Desc != "d" || got.Image != "qwen:9" || len(got.Cmd) != 2 || got.Cmd[1] != "chat" ||
		len(got.Env) != 1 || got.Env[0] != "K=V" || len(got.Volumes) != 1 || len(got.Ports) != 1 || !got.Interactive {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}

	// corrupt files are skipped, not fatal
	if err := os.WriteFile(filepath.Join(Dir(), "bad.toml"), []byte("not [ toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if l := List(); len(l) != 10 {
		t.Fatalf("corrupt file must be skipped, got %d", len(l))
	}
}
