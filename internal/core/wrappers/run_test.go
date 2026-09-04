package wrappers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeDocker writes a shell script masquerading as docker: it echoes the
// args to the log, streams two lines, then relays stdin until EOF.
func fakeDocker(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "docker")
	script := `#!/bin/sh
echo "ARGS: $@"
echo "hello from container"
while IFS= read -r line; do
  echo "ECHO: $line"
done
sleep 0.2
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustLine(t *testing.T, r *Running, timeout time.Duration) string {
	t.Helper()
	select {
	case line, ok := <-r.Lines():
		if !ok {
			t.Fatal("lines channel closed early")
		}
		return line
	case <-time.After(timeout):
		t.Fatal("timed out waiting for container output")
		return ""
	}
}

func TestStartStreamWriteStop(t *testing.T) {
	t.Setenv("CHARMULET_DOCKER_BIN", fakeDocker(t))

	w := Wrapper{Name: "testy", Image: "img:1", Cmd: []string{"run", "me"}, Interactive: true}
	r, err := Start(context.Background(), w)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer r.Stop()

	first := mustLine(t, r, 3*time.Second)
	if !strings.Contains(first, "ARGS:") || !strings.Contains(first, "run") {
		t.Fatalf("arg echo wrong: %q", first)
	}
	if got := mustLine(t, r, 3*time.Second); got != "hello from container" {
		t.Fatalf("stream line wrong: %q", got)
	}

	if err := r.Write("ping"); err != nil {
		t.Fatalf("write: %v", err)
	}
	relay := mustLine(t, r, 3*time.Second)
	if relay != "ECHO: ping" {
		t.Fatalf("stdin relay wrong: %q", relay)
	}
}

func TestStopIsIdempotentAndSignalsDone(t *testing.T) {
	t.Setenv("CHARMULET_DOCKER_BIN", fakeDocker(t))

	w := Wrapper{Name: "stopme", Image: "img:1", Interactive: true}
	r, err := Start(context.Background(), w)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	r.Stop()
	r.Stop() // must not panic or double-close

	select {
	case _, ok := <-r.Done():
		if ok {
			t.Fatal("done channel should be closed, not signalled")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("done never closed after stop")
	}
}

func TestStartRequiresImage(t *testing.T) {
	w := Wrapper{Name: "ghost", Desc: "no image"}
	_, err := Start(context.Background(), w)
	if err == nil || !strings.Contains(err.Error(), "no image") {
		t.Fatalf("imageless start must fail clearly: %v", err)
	}
}

func TestStdinClosedDrainsLines(t *testing.T) {
	t.Setenv("CHARMULET_DOCKER_BIN", fakeDocker(t))

	w := Wrapper{Name: "drain", Image: "img:1", Interactive: true}
	r, err := Start(context.Background(), w)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// Closing stdin ends the relay loop; process exits; lines close.
	r.CloseStdin()

	deadline := time.Now().Add(3 * time.Second)
	for {
		_, ok := <-r.Lines()
		if !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lines never closed after stdin EOF")
		}
	}
	r.Stop()
}
