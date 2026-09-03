// Package chat is the main communication window: real streaming conversation
// with the configured provider, esc cancels in-flight generations.
package chat

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nvii/charmulet/internal/core/provider"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
)

type errMsg struct{ err error }

type tokenMsg struct{ text string }

type streamDoneMsg struct{ err error }

type Model struct {
	ta   textarea.Model
	vp   viewport.Model
	log  []string
	w, h int
	th   theme.Theme

	prov  provider.Provider
	model string
	hist  []provider.Message

	streaming bool
	cancel    context.CancelFunc
	cur       provider.Stream
	buf       strings.Builder
	pending   string // user text of the in-flight turn

	title string // tab-bar label; "CHAT" for the default session
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme, prov provider.Provider, model string) *Model {
	return newModel(th, prov, model, "CHAT")
}

// NewSession creates a spawned chat tab with a model-derived title
// (used by the launcher for parallel sessions).
func NewSession(th theme.Theme, prov provider.Provider, model string) *Model {
	return newModel(th, prov, model, sessionTitle(model))
}

func newModel(th theme.Theme, prov provider.Provider, model, title string) *Model {
	m := &Model{
		th:    th,
		prov:  prov,
		model: model,
		title: title,
	}
	m.ta = textarea.New()
	m.ta.Placeholder = "say something… (enter to send, esc to cancel)"
	if prov != nil {
		m.log = []string{m.th.Secondary().Render("charmulet chat — " + prov.Name() + " · " + model), ""}
	} else {
		m.log = []string{m.th.Secondary().Render("charmulet chat — no provider configured"), ""}
	}
	return m
}

// sessionTitle shortens "hf.co/NousResearch/Hermes-4.3-36B-GGUF:q4_K_M"
// to "HERMES-4.3-36B-G…" style labels for the tab bar.
func sessionTitle(model string) string {
	base := model
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.Index(base, ":"); i > 0 {
		base = base[:i]
	}
	base = strings.ToUpper(base)
	if len(base) > 14 {
		base = base[:13] + "…"
	}
	return base
}

func (m *Model) Title() string { return m.title }
func (m *Model) ID() string    { return "chat" }

func (m *Model) Init() tea.Cmd { return textarea.Blink }

func (m *Model) Activate() tea.Cmd { return nil }
func (m *Model) Deactivate()       {}

func (m *Model) Resize(w, h int) {
	m.w, m.h = w, h
	inputH := 3
	m.vp.SetWidth(w)
	m.vp.SetHeight(h - inputH - 2)
	m.ta.SetWidth(w)
	m.ta.SetHeight(inputH)
}

// pump reads the next chunk off the live stream as a self-rescheduling cmd.
func pump(s provider.Stream) tea.Cmd {
	return func() tea.Msg {
		chunk, ok, err := s()
		switch {
		case err != nil:
			return streamDoneMsg{err}
		case !ok:
			return streamDoneMsg{}
		default:
			return tokenMsg{chunk}
		}
	}
}

func (m *Model) start(userText string) tea.Cmd {
	msgs := make([]provider.Message, 0, len(m.hist)+1)
	msgs = append(msgs, m.hist...)
	msgs = append(msgs, provider.Message{Role: "user", Content: userText})

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.streaming = true
	m.buf.Reset()
	m.pending = userText

	s, err := m.prov.Complete(ctx, m.model, msgs)
	if err != nil {
		cancel()
		m.streaming = false
		return func() tea.Msg { return errMsg{err} }
	}
	m.cur = s
	return pump(s)
}

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			val := strings.TrimSpace(m.ta.Value())
			if val != "" && !m.streaming && m.prov != nil {
				m.log = append(m.log,
					m.th.TabActive().Render(" you ")+" "+val,
				)
				m.ta.Reset()
				cmds = append(cmds, m.start(val))
			}
		case "esc":
			if m.streaming && m.cancel != nil {
				m.cancel()
			}
		}

	case tokenMsg:
		m.buf.WriteString(msg.text)
		cmds = append(cmds, pump(m.cur))

	case streamDoneMsg:
		text := strings.TrimSpace(m.buf.String())
		if text != "" {
			m.log = append(m.log,
				m.th.Accent().Render(" charm ")+" "+text, "",
			)
		} else if msg.err != nil {
			m.log = append(m.log, m.th.TabActive().Render(" err ")+fmtErr(msg.err), "")
		}
		m.hist = append(m.hist, provider.Message{Role: "user", Content: m.pending})
		if text != "" {
			m.hist = append(m.hist, provider.Message{Role: "assistant", Content: text})
		}
		m.pending = ""
		m.streaming = false
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}

	case errMsg:
		m.log = append(m.log, m.th.TabActive().Render(" err ")+msg.err.Error(), "")
		m.streaming = false
	}

	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	cmds = append(cmds, cmd)

	content := lipgloss.NewStyle().Width(m.w).Render(strings.Join(m.renderLog(), "\n"))
	m.vp.SetContent(content)
	m.vp.GotoBottom()

	return m, tea.Batch(cmds...)
}

// renderLog materializes log lines plus any in-flight buffer tail.
func (m *Model) renderLog() []string {
	out := make([]string, len(m.log))
	copy(out, m.log)
	if m.streaming && m.buf.Len() > 0 {
		out = append(out, m.th.Accent().Render(" charm ")+" "+m.buf.String()+"▌")
	}
	return out
}

func fmtErr(err error) string { return "⚠ " + err.Error() }

func (m *Model) View() string {
	input := m.ta.View()
	if pad := m.w - lipgloss.Width(input); pad > 0 {
		lines := strings.Split(input, "\n")
		last := len(lines) - 1
		lines[last] += strings.Repeat(" ", pad)
		input = strings.Join(lines, "\n")
	}
	return m.vp.View() + "\n" + input
}
