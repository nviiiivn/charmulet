package fronts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFallsBackToDefaults(t *testing.T) {
	t.Setenv("CHARMULET_CONFIG_DIR", t.TempDir())
	got := load()
	if len(got) != len(defaults) {
		t.Fatalf("no fronts.toml should give the defaults, got %+v", got)
	}
}

func TestLoadReadsFrontsFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CHARMULET_CONFIG_DIR", dir)
	body := "[[front]]\nname = \"site\"\nurl = \"https://example.com\"\n\n" +
		"[[front]]\nname = \"no url, skipped\"\n"
	if err := os.WriteFile(filepath.Join(dir, "fronts.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := load()
	if len(got) != 1 || got[0].Name != "site" || got[0].URL != "https://example.com" {
		t.Fatalf("fronts.toml not read: %+v", got)
	}
}

func TestLoadBadFileFallsBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CHARMULET_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "fronts.toml"), []byte("not = [toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := load(); len(got) != len(defaults) {
		t.Fatalf("broken fronts.toml should give the defaults, got %+v", got)
	}
}
