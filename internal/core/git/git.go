// Package git shells out to the git CLI for the CHANGES tab: repo status
// as a file list, per-file diffs for the walkthrough viewer. Read-only —
// it never mutates the working tree or index.
package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// File is one entry from git status --porcelain.
type File struct {
	Path    string
	X, Y    byte // index/worktree status codes
	Staged  bool
	Deleted bool
}

// Label returns the short status glyph for list rendering.
func (f File) Label() string {
	switch {
	case f.X == '?' && f.Y == '?':
		return "??"
	case f.X == 'A':
		return "A "
	case f.X == 'D' || f.Y == 'D':
		return "D "
	case f.X == 'M' || f.Y == 'M':
		return "M "
	case f.X == 'R':
		return "R "
	default:
		return string(f.X) + string(f.Y)
	}
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("git: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

// Status lists working-tree changes for a repo directory.
func Status(ctx context.Context, dir string) ([]File, error) {
	out, err := run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	var files []File
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		f := File{Path: line[3:], X: line[0], Y: line[1]}
		f.Staged = f.X != ' ' && f.X != '?'
		f.Deleted = f.X == 'D' || f.Y == 'D'
		files = append(files, f)
	}
	return files, nil
}

// Branch returns the current branch name (short).
func Branch(ctx context.Context, dir string) string {
	out, err := run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(out)
}

// Diff renders the diff for one file. Staged files diff against HEAD via
// --cached; untracked files are shown in full via --no-index (git exits 1
// with a valid diff there — not an error for us).
func Diff(ctx context.Context, dir string, f File) (string, error) {
	var out string
	var err error
	switch {
	case f.X == '?' && f.Y == '?':
		out, err = runNoIndex(ctx, dir, f.Path)
		if err != nil {
			return "", err
		}
	case f.Staged:
		out, err = run(ctx, dir, "diff", "--cached", "--", f.Path)
	default:
		out, err = run(ctx, dir, "diff", "--", f.Path)
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) == "" {
		// Staged-only file with no textual diff (e.g. mode change).
		out = "(no textual diff for " + f.Path + ")"
	}
	return out, nil
}

// runNoIndex diffs an untracked file against /dev/null. git returns exit
// code 1 for "differences found", so stderr-clean non-zero is success.
func runNoIndex(ctx context.Context, dir, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "diff", "--no-index", "--", "/dev/null", path)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if code := ee.ExitCode(); code == 1 && len(ee.Stderr) == 0 {
				return string(out), nil
			}
			return "", fmt.Errorf("git: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}
