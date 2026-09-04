// Package runs fans out a prompt to multiple models in parallel, streams
// their responses, and lets you pick/fuse the best.
package runs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/nvii/charmulet/internal/core/provider"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
)

// flushInterval coalesces streamed chunks into one repaint per tick.
const flushInterval = 30 * time.Millisecond

type runModel struct {
	name      string
	selected  bool
	streaming bool
	done      bool
	err       error
	buf       strings.Builder
	output    string
	cancel    context.CancelFunc
	cur       provider.Stream
}

type Model struct {
	promptTA textarea.Model
	vp       viewport.Model
	models   []*runModel
	w, h     int
	th       theme.Theme

	prov       provider.Provider
	streaming  bool
	inputMode  bool // true = editing prompt, false = viewing results
	flushing   bool // a flush tick is armed
	promptText string
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme, prov provider.Provider) *Model {
	m := &Model{th: th, prov: prov}
	m.promptTA = textarea.New()
	m.promptTA.Placeholder = "prompt to fan out… (enter to run, tab to switch to picker)"
	m.promptTA.SetHeight(3)
	m.vp = viewport.New()
	m.inputMode = true

	if prov != nil {
		for _, name := range prov.Models() {
			m.models = append(m.models, &runModel{name: name})
		}
	}
	return m
}

func (m *Model) Title() string { return "RUNS" }
func (m *Model) ID() string    { return "runs" }

func (m *Model) Init() tea.Cmd { return textarea.Blink }

func (m *Model) Activate() tea.Cmd { return nil }
func (m *Model) Deactivate()       {}

// Esc cancels every in-flight run while in results mode.
func (m *Model) Esc() bool {
	if m.inputMode {
		return true // composing a prompt — esc does nothing destructive
	}
	for _, rm := range m.models {
		if rm.streaming && rm.cancel != nil {
			rm.cancel()
		}
	}
	return true
}

func (m *Model) Resize(w, h int) {
	m.w, m.h = w, h
	m.promptTA.SetWidth(w)
	if m.inputMode {
		m.vp.SetHeight(h - 5)
	} else {
		m.vp.SetHeight(h - 3)
	}
	m.vp.SetWidth(w)
}

func (m *Model) start() tea.Cmd {
	m.streaming = true
	m.inputMode = false
	m.promptText = strings.TrimSpace(m.promptTA.Value())
	m.promptTA.Reset()

	for _, rm := range m.models {
		if rm.selected {
			rm.streaming = true
			rm.done = false
			rm.err = nil
			rm.buf.Reset()
			rm.output = ""
		}
	}

	var cmds []tea.Cmd
	for _, rm := range m.models {
		if rm.selected {
			ctx, cancel := context.WithCancel(context.Background())
			rm.cancel = cancel
			cmds = append(cmds, m.streamModel(ctx, rm))
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) streamModel(ctx context.Context, rm *runModel) tea.Cmd {
	return func() tea.Msg {
		msgs := []provider.Message{{Role: "user", Content: m.promptText}}
		s, err := m.prov.Complete(ctx, rm.name, msgs)
		if err != nil {
			return runErrMsg{model: rm.name, err: err}
		}
		return runStartMsg{model: rm.name, stream: s}
	}
}

func (m *Model) pump(rm *runModel) tea.Cmd {
	return func() tea.Msg {
		chunk, ok, err := rm.cur()
		if err != nil {
			return runErrMsg{model: rm.name, err: err}
		}
		if !ok {
			return runDoneMsg{model: rm.name}
		}
		return runChunkMsg{model: rm.name, text: chunk}
	}
}

type runStartMsg struct {
	model  string
	stream provider.Stream
}

func (m runStartMsg) TabID() string { return "runs" }

type runChunkMsg struct {
	model string
	text  string
}

func (m runChunkMsg) TabID() string { return "runs" }

type runDoneMsg struct {
	model string
}

func (m runDoneMsg) TabID() string { return "runs" }

type runErrMsg struct {
	model string
	err   error
}

func (m runErrMsg) TabID() string { return "runs" }

// flushMsg triggers the deferred repaint that batches streamed chunks.
type flushMsg struct{}

func (m flushMsg) TabID() string { return "runs" }

func flushCmd() tea.Cmd {
	return tea.Tick(flushInterval, func(time.Time) tea.Msg { return flushMsg{} })
}

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tabs.SetThemeMsg:
		m.th = msg.Theme
		return m, nil
	case tea.KeyPressMsg:
		return m.updateKeys(msg)
	case runStartMsg:
		return m.applyRunMsg(msg.model, func(rm *runModel) tea.Cmd {
			rm.cur = msg.stream
			return m.pump(rm)
		}, false)
	case runChunkMsg:
		return m.applyRunMsg(msg.model, func(rm *runModel) tea.Cmd {
			rm.buf.WriteString(msg.text)
			return m.pump(rm)
		}, true)
	case runDoneMsg:
		return m.applyRunMsg(msg.model, func(rm *runModel) tea.Cmd {
			rm.streaming = false
			rm.done = true
			rm.output = strings.TrimSpace(rm.buf.String())
			return nil
		}, false)
	case runErrMsg:
		return m.applyRunMsg(msg.model, func(rm *runModel) tea.Cmd {
			rm.streaming = false
			rm.done = true
			rm.err = msg.err
			return nil
		}, false)
	case flushMsg:
		m.flushing = false
	}

	m.repaint()
	return m, nil
}

// updateKeys handles both prompt-input and results-viewing modes.
func (m *Model) updateKeys(msg tea.KeyPressMsg) (tabs.Tab, tea.Cmd) {
	if m.inputMode {
		switch msg.String() {
		case "enter":
			if strings.TrimSpace(m.promptTA.Value()) != "" {
				cmd := m.start()
				m.repaint()
				return m, cmd
			}
		case "tab":
			m.inputMode = false
			m.repaint()
			return m, nil
		}
		var cmd tea.Cmd
		m.promptTA, cmd = m.promptTA.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "tab":
		m.inputMode = true
	case "p":
		m.pickBest()
	case "f":
		m.fuse()
	}
	if m.inputMode {
		m.repaint()
		return m, nil
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// pickBest loads the longest non-empty output into the prompt editor.
func (m *Model) pickBest() {
	best := ""
	for _, rm := range m.models {
		if rm.output != "" && len(rm.output) > len(best) {
			best = rm.output
		}
	}
	if best != "" {
		m.promptTA.SetValue(best)
		m.inputMode = true
	}
}

// fuse concatenates all outputs with headers into the prompt editor.
func (m *Model) fuse() {
	var parts []string
	for _, rm := range m.models {
		if rm.output != "" {
			parts = append(parts, fmt.Sprintf("## %s\n\n%s", rm.name, rm.output))
		}
	}
	if len(parts) > 0 {
		m.promptTA.SetValue(strings.Join(parts, "\n\n---\n\n"))
		m.inputMode = true
	}
}

// applyRunMsg targets one model in the fan-out, marks streaming done when
// every selected model finished, and repaints on state changes only —
// chunk appends (isChunk) are coalesced into flush ticks.
func (m *Model) applyRunMsg(model string, apply func(*runModel) tea.Cmd, isChunk bool) (tabs.Tab, tea.Cmd) {
	var cmd tea.Cmd
	for _, rm := range m.models {
		if rm.name == model {
			cmd = apply(rm)
			break
		}
	}
	if isChunk {
		if !m.flushing {
			m.flushing = true
			return m, tea.Batch(cmd, flushCmd())
		}
		return m, cmd
	}
	m.checkAllDone()
	m.repaint()
	return m, cmd
}

func (m *Model) checkAllDone() {
	for _, rm := range m.models {
		if rm.selected && !rm.done {
			return
		}
	}
	m.streaming = false
}

// repaint pushes rendered content into the viewport; chunkCmds can arm a
// flush tick so bursts of chunks repaint once per interval.
func (m *Model) repaint() {
	m.vp.SetContent(m.render())
}

func (m *Model) render() string {
	if m.inputMode {
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n\n", m.th.TabActive().Render(" PROMPT "))
		b.WriteString(m.promptTA.View())
		b.WriteString("\n\n")
		fmt.Fprintf(&b, "%s", m.th.Muted().Render("enter run · tab model picker · esc cancel"))
		return b.String()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", m.th.TabActive().Render(" RESULTS: "+m.promptText))
	for _, rm := range m.models {
		if !rm.selected {
			continue
		}
		header := m.th.Accent().Render(" ▸ " + rm.name + " ")
		if rm.streaming {
			header += m.th.Secondary().Render(" ◌ streaming…")
		} else if rm.done {
			if rm.err != nil {
				header += m.th.TabActive().Render(" ✗ " + rm.err.Error())
			} else {
				header += m.th.Secondary().Render(" ✓ done")
			}
		}
		b.WriteString(header + "\n")

		content := rm.output
		if rm.streaming && rm.buf.Len() > 0 {
			content = rm.buf.String() + "▌"
		}
		if content != "" {
			for _, ln := range strings.Split(content, "\n") {
				b.WriteString("  " + ln + "\n")
			}
		}
		b.WriteString("\n")
	}
	b.WriteString(m.th.Muted().Render("p pick-best · f fuse · tab prompt · esc cancel"))
	return b.String()
}

func (m *Model) View() string {
	if m.inputMode {
		return m.promptTA.View()
	}
	return m.vp.View()
}
