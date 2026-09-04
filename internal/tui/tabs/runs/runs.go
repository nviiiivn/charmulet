// Package runs fans out a prompt to multiple models in parallel, streams
// their responses, and lets you pick/fuse the best.
package runs

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/nvii/charmulet/internal/core/provider"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
)

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
	cursor   int
	w, h     int
	th       theme.Theme

	prov       provider.Provider
	streaming  bool
	inputMode  bool // true = editing prompt, false = viewing results
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

func (m *Model) Resize(w, h int) {
	m.w, m.h = w, h
	m.promptTA.SetWidth(w)
	if m.inputMode {
		m.vp.SetWidth(w)
		m.vp.SetHeight(h - 5)
	} else {
		m.vp.SetWidth(w)
		m.vp.SetHeight(h - 3)
	}
}

func (m *Model) start() tea.Cmd {
	m.streaming = true
	m.inputMode = false
	m.promptText = strings.TrimSpace(m.promptTA.Value())
	m.promptTA.Reset()

	// reset model states
	for _, rm := range m.models {
		if rm.selected {
			rm.streaming = true
			rm.done = false
			rm.err = nil
			rm.buf.Reset()
			rm.output = ""
		}
	}

	// fire one cmd per selected model
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

type runChunkMsg struct {
	model string
	text  string
}

type runDoneMsg struct {
	model string
}

type runErrMsg struct {
	model string
	err   error
}

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tabs.SetThemeMsg:
		m.th = msg.Theme
		return m, nil
	case tea.KeyPressMsg:
		if m.inputMode {
			switch msg.String() {
			case "enter":
				if strings.TrimSpace(m.promptTA.Value()) != "" {
					cmds = append(cmds, m.start())
				}
			case "tab":
				m.inputMode = false
				return m, nil
			}
			var cmd tea.Cmd
			m.promptTA, cmd = m.promptTA.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}

		// results viewing mode
		switch msg.String() {
		case "tab":
			m.inputMode = true
			return m, nil
		case "esc":
			// cancel all in-flight
			for _, rm := range m.models {
				if rm.streaming && rm.cancel != nil {
					rm.cancel()
				}
			}
			return m, nil
		case "p":
			// pick-best heuristic: longest non-empty output
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
		case "f":
			// fuse: concat all outputs with headers
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

	case runStartMsg:
		for _, rm := range m.models {
			if rm.name == msg.model {
				rm.cur = msg.stream
				cmds = append(cmds, m.pump(rm))
				break
			}
		}

	case runChunkMsg:
		for _, rm := range m.models {
			if rm.name == msg.model {
				rm.buf.WriteString(msg.text)
				cmds = append(cmds, m.pump(rm))
				break
			}
		}

	case runDoneMsg:
		for _, rm := range m.models {
			if rm.name == msg.model {
				rm.streaming = false
				rm.done = true
				rm.output = strings.TrimSpace(rm.buf.String())
				break
			}
		}
		// check if all done
		allDone := true
		for _, rm := range m.models {
			if rm.selected && !rm.done {
				allDone = false
				break
			}
		}
		if allDone {
			m.streaming = false
		}

	case runErrMsg:
		for _, rm := range m.models {
			if rm.name == msg.model {
				rm.streaming = false
				rm.done = true
				rm.err = msg.err
				break
			}
		}
	}

	// always route to viewport for scrolling
	var vcmd tea.Cmd
	m.vp, vcmd = m.vp.Update(msg)
	cmds = append(cmds, vcmd)

	m.vp.SetContent(m.render())
	return m, tea.Batch(cmds...)
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
			lines := strings.Split(content, "\n")
			for _, ln := range lines {
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
