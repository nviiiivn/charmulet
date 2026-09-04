package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repo builds a real git repo: initial commit, then a modified, a staged,
// an untracked, and a deleted file.
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sh := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	sh("init", "-q")
	sh("config", "user.email", "t@t")
	sh("config", "user.name", "t")
	write(t, dir, "keep.txt", "keep\n")
	write(t, dir, "mod.txt", "one\n")
	write(t, dir, "gone.txt", "gone\n")
	sh("add", ".")
	sh("commit", "-qm", "init")

	write(t, dir, "mod.txt", "one\ntwo\n")    // unstaged modify
	write(t, dir, "new.txt", "fresh\n")       // untracked
	write(t, dir, "stage.txt", "staged\n")    // to-be-staged
	sh("add", "stage.txt")                    // staged add
	os.Remove(filepath.Join(dir, "gone.txt")) // unstaged delete
	return dir
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStatusParses(t *testing.T) {
	dir := repo(t)
	files, err := Status(context.Background(), dir)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("want 4 changed files, got %d: %+v", len(files), files)
	}
	byPath := map[string]File{}
	for _, f := range files {
		byPath[f.Path] = f
	}
	if f := byPath["mod.txt"]; f.X != ' ' || f.Y != 'M' || f.Staged {
		t.Fatalf("mod.txt codes wrong: %+v", f)
	}
	if f := byPath["stage.txt"]; !f.Staged || f.X != 'A' {
		t.Fatalf("stage.txt should be staged add: %+v", f)
	}
	if f := byPath["new.txt"]; f.X != '?' || f.Label() != "??" {
		t.Fatalf("new.txt should be untracked: %+v", f)
	}
	if f := byPath["gone.txt"]; !f.Deleted {
		t.Fatalf("gone.txt should be deleted: %+v", f)
	}
}

func TestBranch(t *testing.T) {
	dir := repo(t)
	if b := Branch(context.Background(), dir); b == "" || b == "?" {
		t.Fatalf("branch = %q", b)
	}
}

func TestDiffVariants(t *testing.T) {
	dir := repo(t)
	files, err := Status(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]File{}
	for _, f := range files {
		byPath[f.Path] = f
	}
	ctx := context.Background()

	// unstaged modify: shows the added line and a hunk header
	d, err := Diff(ctx, dir, byPath["mod.txt"])
	if err != nil || !strings.Contains(d, "+two") || !strings.Contains(d, "@@") {
		t.Fatalf("mod diff wrong: %v %q", err, d)
	}

	// staged add: full content as additions
	d, err = Diff(ctx, dir, byPath["stage.txt"])
	if err != nil || !strings.Contains(d, "+staged") {
		t.Fatalf("staged diff wrong: %v %q", err, d)
	}

	// untracked: full content via --no-index
	d, err = Diff(ctx, dir, byPath["new.txt"])
	if err != nil || !strings.Contains(d, "+fresh") {
		t.Fatalf("untracked diff wrong: %v %q", err, d)
	}

	// delete: shows removed content
	d, err = Diff(ctx, dir, byPath["gone.txt"])
	if err != nil || !strings.Contains(d, "-gone") {
		t.Fatalf("delete diff wrong: %v %q", err, d)
	}
}

func TestStatusNotARepo(t *testing.T) {
	if _, err := Status(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected error outside a repo")
	}
}
