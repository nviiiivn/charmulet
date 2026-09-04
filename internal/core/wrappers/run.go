// docker runner: shells out to the docker CLI, streams output lines,
// and guarantees container teardown on Stop.
package wrappers

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Running is one attached docker container.
type Running struct {
	Name    string // container name (charmulet-<wrapper>-<n>)
	wrapper Wrapper

	lines chan string
	stdin io.WriteCloser
	cmd   *exec.Cmd

	mu      sync.Mutex
	stopped bool
	done    chan struct{} // closed when docker run exits
}

var seq int64
var seqMu sync.Mutex

func nextName(wrapper string) string {
	seqMu.Lock()
	defer seqMu.Unlock()
	seq++
	return fmt.Sprintf("charmulet-%s-%d", wrapper, seq)
}

// DockerPath resolves the docker binary (CHARMULET_DOCKER_BIN overrides).
func DockerPath() string {
	if v := os.Getenv("CHARMULET_DOCKER_BIN"); v != "" {
		return v
	}
	if p, err := exec.LookPath("docker"); err == nil {
		return p
	}
	return "docker"
}

// dockerArgs builds the full `docker run` argument list for a wrapper.
func dockerArgs(w Wrapper, name string) []string {
	args := []string{"run", "--rm", "--name", name}
	for _, e := range w.Env {
		args = append(args, "-e", e)
	}
	for _, v := range w.Volumes {
		args = append(args, "-v", v)
	}
	for _, p := range w.Ports {
		args = append(args, "-p", p)
	}
	if w.Interactive {
		args = append(args, "-i")
	}
	args = append(args, w.Image)
	args = append(args, w.Cmd...)
	return args
}

// Start launches the wrapper's container. Output (stdout+stderr) streams
// on Lines; with Interactive the returned Running accepts stdin writes.
// The caller must call Stop exactly once.
func Start(ctx context.Context, w Wrapper) (*Running, error) {
	if w.Image == "" {
		return nil, fmt.Errorf("wrapper %q has no image configured — set one in %s", w.Name, path(w.Name))
	}
	name := nextName(w.Name)
	cmd := exec.CommandContext(ctx, DockerPath(), dockerArgs(w, name)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	r := &Running{
		Name:    name,
		wrapper: w,
		lines:   make(chan string, 64),
		stdin:   stdin,
		cmd:     cmd,
		done:    make(chan struct{}),
	}

	var wg sync.WaitGroup
	scan := func(rd io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(rd)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			select {
			case r.lines <- sc.Text():
			case <-r.done:
				return
			}
		}
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	go func() {
		wg.Wait()
		close(r.lines)
	}()
	go func() {
		_ = cmd.Wait()
		close(r.done)
	}()
	return r, nil
}

// Lines is the output stream; it closes when the container exits.
func (r *Running) Lines() <-chan string { return r.lines }

// Done reports container exit.
func (r *Running) Done() <-chan struct{} { return r.done }

// Write sends a line to the container's stdin (newline appended).
func (r *Running) Write(line string) error {
	_, err := io.WriteString(r.stdin, line+"\n")
	return err
}

// CloseStdin ends the container's input without killing it; interactive
// CLIs treat this as "user hung up" and exit on their own.
func (r *Running) CloseStdin() {
	_ = r.stdin.Close()
}

// Stop tears the container down: kills the docker client, force-removes
// the named container, closes stdin. Safe to call twice.
func (r *Running) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return
	}
	r.stopped = true
	if r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
	_ = r.stdin.Close()
	// --rm removes on exit, but a wedged container needs the hammer.
	remove := exec.Command(DockerPath(), "rm", "-f", r.Name)
	_ = remove.Run()
}

// Summary renders the docker command for the confirm gate.
func Summary(w Wrapper) string {
	if w.Image == "" {
		return fmt.Sprintf("%s: no image configured (%s)", w.Name, path(w.Name))
	}
	args := dockerArgs(w, "<name>")
	return strings.Join(args, " ")
}
