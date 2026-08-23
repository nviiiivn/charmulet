// Package chat is the main communication window. P0 is a local echo loop —
// the provider wiring (streaming, cancel, markdown) lands in P1.
package chat

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
)

type Model struct {
	ta   textarea.Model
	vp   viewport.Model
	log  []string
	w, h int
	th   theme.Theme
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme) *Model {
	m := &Model{th: th}
	m.ta = textarea.New()
	m.log = []string{m.th.Secondary().Render("charmulet chat — local echo mode (P0)"), ""}
	return m
}

func (m *Model) Title() string { return "CHAT" }
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

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	var cmds []tea.Cmd

	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "enter" {
		val := strings.TrimSpace(m.ta.Value())
		if val != "" {
			you := m.th.TabActive().Render(" you ") + " " + val
			m.log = append(m.log, you, "")
			m.ta.Reset()
		}
	}

	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	cmds = append(cmds, cmd)

	content := lipgloss.NewStyle().Width(m.w).Render(strings.Join(m.log, "\n"))
	m.vp.SetContent(content)
	m.vp.GotoBottom()

	return m, tea.Batch(cmds...)
}

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
