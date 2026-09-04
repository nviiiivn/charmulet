// Package sessions lists persisted chat sessions and reopens them —
// the crush-style resume picker. Read-only until enter spawns a live tab.
package sessions

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nvii/charmulet/internal/core/provider"
	"github.com/nvii/charmulet/internal/core/session"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/tabs/chat"
	"github.com/nvii/charmulet/internal/tui/theme"
)

type listedMsg struct {
	metas []session.SessionMeta
	err   error
}

type Model struct {
	th   theme.Theme
	prov provider.Provider
	st   *session.Store
	w, h int

	metas  []session.SessionMeta
	sel    int
	top    int
	err    string
	loaded bool
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme, prov provider.Provider, st *session.Store) *Model {
	return &Model{th: th, prov: prov, st: st}
}

func (m *Model) Title() string { return "SESSIONS" }
func (m *Model) ID() string    { return "sessions" }
func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Activate() tea.Cmd { return listCmd(m.st) }
func (m *Model) Deactivate()       {}

func listCmd(st *session.Store) tea.Cmd {
	return func() tea.Msg {
		metas, err := st.List()
		return listedMsg{metas: metas, err: err}
	}
}

// resumeCmd loads the transcript then asks the shell to spawn a live tab.
func (m *Model) resumeCmd(meta session.SessionMeta) tea.Cmd {
	th, prov, st := m.th, m.prov, m.st
	return func() tea.Msg {
		msgs, err := st.Messages(meta.ID)
		if err != nil {
			return listedMsg{err: err}
		}
		return tabs.SpawnMsg{Tab: chat.NewResume(th, prov, st, meta, msgs)}
	}
}

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case listedMsg:
		m.loaded = true
		m.err = ""
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.metas = msg.metas
		if m.sel >= len(m.metas) {
			m.sel = len(m.metas) - 1
		}
		if m.sel < 0 {
			m.sel = 0
		}
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "j", "down":
			if m.sel < len(m.metas)-1 {
				m.sel++
			}
		case "k", "up":
			if m.sel > 0 {
				m.sel--
			}
		case "r", "R":
			return m, listCmd(m.st)
		case "enter":
			if len(m.metas) > 0 {
				return m, m.resumeCmd(m.metas[m.sel])
			}
		}
	}
	return m, nil
}

func (m *Model) Resize(w, h int) { m.w, m.h = w, h }

// relTime renders "5m", "3h", "2d" style ages.
func relTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func (m *Model) View() string {
	var b strings.Builder
	b.WriteString(m.th.Secondary().Render("sessions") + "  " +
		m.th.Muted().Render("j/k move · enter resume · r refresh") + "\n")

	switch {
	case m.err != "":
		b.WriteString(m.th.Tertiary().Render("⚠ " + m.err))
	case !m.loaded:
		b.WriteString(m.th.Muted().Render("loading…"))
	case len(m.metas) == 0:
		b.WriteString(m.th.Muted().Render("no sessions yet — spawn one from LAUNCH (6)"))
	default:
		vis := m.h - 4
		if vis < 3 {
			vis = 3
		}
		if vis > len(m.metas) {
			vis = len(m.metas)
		}
		if m.sel < m.top {
			m.top = m.sel
		}
		if m.sel >= m.top+vis {
			m.top = m.sel - vis + 1
		}

		titleW, modelW := 26, 24
		head := lipgloss.NewStyle().
			Foreground(m.th.AccentColor()).
			Render(pad("SESSION", titleW+2) + pad("MODEL", modelW+2) + "MS   AGE")
		b.WriteString(head + "\n")
		for i := m.top; i < m.top+vis && i < len(m.metas); i++ {
			meta := m.metas[i]
			row := pad(trunc(meta.Title, titleW), titleW+2) +
				pad(trunc(meta.Model, modelW), modelW+2) +
				pad(fmt.Sprintf("%d", meta.MsgCount), 5) +
				relTime(meta.UpdatedAt)
			if i == m.sel {
				b.WriteString(m.th.Accent().Render("▶ "+row) + "\n")
			} else {
				b.WriteString(m.th.Muted().Render("  "+row) + "\n")
			}
		}
		count := fmt.Sprintf("%d/%d", m.sel+1, len(m.metas))
		b.WriteString(m.th.Secondary().Render(count))
	}
	return b.String()
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
