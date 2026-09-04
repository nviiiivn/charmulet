// Package wrap is a live docker-backed wrapper session: container output
// streams into a viewport, input lines go to the container's stdin.
package wrap

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/nvii/charmulet/internal/core/wrappers"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
)

var nextID atomic.Int64

// lineMsg carries one container output line; exitMsg reports teardown.
type lineMsg struct {
	id   int64
	text string
}

func (m lineMsg) TabID() string { return fmt.Sprintf("wrap-%d", m.id) }

type exitMsg struct {
	id  int64
	err error
}

func (m exitMsg) TabID() string { return fmt.Sprintf("wrap-%d", m.id) }

type inputEcho struct{ text string }

type Model struct {
	th      theme.Theme
	wrapper wrappers.Wrapper
	id      int64
	w, h    int

	run   *wrappers.Running
	ctx   context.CancelFunc
	log   []string
	vp    viewport.Model
	ta    textarea.Model
	dead  bool // container exited
	errSt string
}

var _ tabs.Tab = (*Model)(nil)

// New starts the wrapper's container immediately.
func New(th theme.Theme, w wrappers.Wrapper) (*Model, error) {
	ctx, cancel := context.WithCancel(context.Background())
	r, err := wrappers.Start(ctx, w)
	if err != nil {
		cancel()
		return nil, err
	}
	m := &Model{
		th: th, wrapper: w, id: nextID.Add(1),
		run: r, ctx: cancel,
	}
	m.ta = textarea.New()
	m.ta.Placeholder = "send to container…"
	m.vp = viewport.New()
	return m, nil
}

func (m *Model) Title() string { return strings.ToUpper(m.wrapper.Name) }
func (m *Model) ID() string    { return fmt.Sprintf("wrap-%d", m.id) }
func (m *Model) Init() tea.Cmd { return m.pump() }

func (m *Model) Activate() tea.Cmd { return nil }
func (m *Model) Deactivate()       {}

// pump reads one output line per cmd, self-rescheduling while the
// container lives; channel close becomes exitMsg.
func (m *Model) pump() tea.Cmd {
	r := m.run
	return func() tea.Msg {
		line, ok := <-r.Lines()
		if !ok {
			return exitMsg{id: m.id}
		}
		return lineMsg{id: m.id, text: line}
	}
}

// OnClose stops the container when the tab is closed (ctrl+w).
func (m *Model) OnClose() {
	if m.run != nil {
		m.run.Stop()
	}
	if m.ctx != nil {
		m.ctx()
	}
}

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tabs.SetThemeMsg:
		m.th = msg.Theme
		return m, nil

	case lineMsg:
		if msg.id != m.id {
			return m, nil
		}
		m.log = append(m.log, msg.text)
		m.repaint()
		return m, m.pump()

	case exitMsg:
		if msg.id != m.id {
			return m, nil
		}
		m.dead = true
		if msg.err != nil {
			m.errSt = msg.err.Error()
		}
		m.log = append(m.log, "— container exited —")
		m.repaint()
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			val := strings.TrimSpace(m.ta.Value())
			if val == "" || m.dead || !m.wrapper.Interactive || m.run == nil {
				return m, nil
			}
			if err := m.run.Write(val); err != nil {
				m.log = append(m.log, "⚠ stdin: "+err.Error())
				m.repaint()
				return m, nil
			}
			m.log = append(m.log, m.th.Accent().Render(" ▸ "+val))
			m.ta.Reset()
			m.repaint()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	return m, cmd
}

// Esc backs out: stop the container if it's still running.
func (m *Model) Esc() bool {
	if !m.dead && m.run != nil {
		m.OnClose()
		m.dead = true
		m.log = append(m.log, "— stopped —")
		m.repaint()
		return true
	}
	return false
}

func (m *Model) Resize(w, h int) {
	m.w, m.h = w, h
	inputH := 3
	if h > 10 {
		inputH = h / 5
		if inputH > 5 {
			inputH = 5
		}
	}
	m.vp.SetWidth(w)
	m.vp.SetHeight(h - inputH - 1)
	m.ta.SetWidth(w)
	m.ta.SetHeight(inputH)
	m.repaint()
}

func (m *Model) repaint() {
	m.vp.SetContent(strings.Join(m.log, "\n"))
	m.vp.GotoBottom()
}

func (m *Model) View() string {
	status := m.th.Secondary().Render(m.wrapper.Image)
	if m.dead {
		status = m.th.Muted().Render("exited")
		if m.errSt != "" {
			status += " " + m.th.Tertiary().Render("⚠ "+m.errSt)
		}
	}
	input := m.ta.View()
	if m.wrapper.Interactive && !m.dead {
		input += "\n" + m.th.Muted().Render("enter send · esc stop container")
	}
	return status + "\n" + m.vp.View() + "\n" + input
}
