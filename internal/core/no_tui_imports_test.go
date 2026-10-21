// Package core_test enforces the structural rule from BLUEPRINT.md §3:
// nothing under internal/core may import internal/tui. Machinery, not prose.
package core_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreNeverImportsTUI(t *testing.T) {
	root := "."
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), `"github.com/nviiiivn/charmulet/internal/tui`) ||
			strings.Contains(string(raw), `"github.com/nviiiivn/charmulet/internal/server`) {
			t.Errorf("core file imports tui/server (forbidden): %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
