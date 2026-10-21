package pages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nviiiivn/charmulet/internal/homelab"
)

func TestSaveListRoundtrip(t *testing.T) {
	t.Setenv("CHARMULET_CONFIG_DIR", t.TempDir())

	p := Page{Name: "lab", Columns: 3, Widgets: []Widget{
		{Kind: "system", Machine: "demo-desktop"},
		{Kind: "note", Title: "scratch", Text: "hello\npages"},
	}}
	if err := Save(p); err != nil {
		t.Fatalf("save: %v", err)
	}

	// corrupt neighbor skipped, not fatal
	if err := os.WriteFile(filepath.Join(Dir(), "bad.toml"), []byte("not [ toml"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := List()
	if len(got) != 1 {
		t.Fatalf("want 1 page, got %d", len(got))
	}
	if got[0].Name != "lab" || got[0].Columns != 3 || len(got[0].Widgets) != 2 {
		t.Fatalf("roundtrip mismatch: %+v", got[0])
	}
	if got[0].Widgets[1].Text != "hello\npages" {
		t.Fatalf("multiline text lost: %q", got[0].Widgets[1].Text)
	}
}

func TestColumnsClamped(t *testing.T) {
	t.Setenv("CHARMULET_CONFIG_DIR", t.TempDir())
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(Dir(), "w.toml"), []byte(
		"name = 'wide'\ncolumns = 9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := List()[0].Columns; got != 2 {
		t.Fatalf("columns want clamp to 2, got %d", got)
	}
}

func TestEnsureDefaultSeedsOnce(t *testing.T) {
	t.Setenv("CHARMULET_CONFIG_DIR", t.TempDir())

	if _, ok := EnsureDefault(); !ok {
		t.Fatal("first call must seed")
	}
	if got := List(); len(got) != 1 || got[0].Name != "home" {
		t.Fatalf("seed failed: %+v", got)
	}
	// user page exists now — seeding must back off
	if err := Save(Page{Name: "aaa"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := EnsureDefault(); ok {
		t.Fatal("second call must not seed")
	}
}

func TestInvalidNamesRejected(t *testing.T) {
	t.Setenv("CHARMULET_CONFIG_DIR", t.TempDir())
	for _, name := range []string{"", ".", "..", "a/b", "../x"} {
		if err := Save(Page{Name: name}); err == nil {
			t.Fatalf("name %q must be rejected", name)
		}
	}
}

func fakeMachines() map[string]homelab.Machine {
	return map[string]homelab.Machine{
		"demo-desktop": {Display: "demo-desktop", Color: "#58a6ff", Widgets: homelab.Widgets{
			System: &homelab.System{
				CPUPercent: 42, MemPercent: 61, DiskPercent: 88,
				MemUsed: "9.7G", MemTotal: "16G", Disk: "1.6T/1.9T (88%)",
				GPU:    "NVIDIA GeForce RTX 3080, 5162 MiB, 20480 MiB, 25 %",
				Uptime: "6d 4h", Temp: "47C",
			},
		}},
	}
}

func TestRegistryRenders(t *testing.T) {
	d := Deps{Width: 30, Now: time.Date(2026, 9, 4, 10, 30, 0, 0, time.UTC), Machines: fakeMachines()}

	sys := Render(Widget{Kind: "system", Machine: "demo-desktop"}, d)
	for _, want := range []string{"cpu", "mem", "disk", "88%", "!", "RTX 3080", "· 25 %"} {
		if !strings.Contains(sys, want) {
			t.Fatalf("system render missing %q:\n%s", want, sys)
		}
	}

	// widget type is unexported on homelab.Machine, so a feed without the
	// ollama widget must render its absence, not panic
	oll := Render(Widget{Kind: "ollama", Machine: "demo-desktop"}, d)
	if !strings.Contains(oll, "no ollama") {
		t.Fatalf("ollama render wrong: %q", oll)
	}

	clk := Render(Widget{Kind: "clock"}, d)
	if !strings.Contains(clk, "10:30:00") || !strings.Contains(clk, "Sep") {
		t.Fatalf("clock render wrong: %q", clk)
	}

	unk := Render(Widget{Kind: "bogus"}, d)
	if !strings.Contains(unk, "unknown component") {
		t.Fatalf("unknown kind must hint: %q", unk)
	}
}
