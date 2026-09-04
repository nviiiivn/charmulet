// Package launch is the launcher tab: pick a model (or wrapper) and spawn
// parallel chat sessions. Wrappers run in docker, confirm-gated: enter
// shows the exact docker command, y spawns an attached WRAP tab.
package launch

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nvii/charmulet/internal/core/provider"
	"github.com/nvii/charmulet/internal/core/session"
	"github.com/nvii/charmulet/internal/core/wrappers"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/tabs/chat"
	"github.com/nvii/charmulet/internal/tui/tabs/wrap"
	"github.com/nvii/charmulet/internal/tui/theme"
)

type srcKind int

const (
	srcModels srcKind = iota
	srcWrapper
)

type source struct {
	name string
	desc string
	kind srcKind
}

// wrappers fleet comes from the wrappers package (builtins + TOML overrides).
// sources() materializes the launcher list: ollama first, then the fleet.

type modelsMsg struct {
	models []string
	err    error
}
type Model struct {
	th      theme.Theme
	prov    provider.Provider
	st      *session.Store
	w, h    int
	pane    int // 0 = sources, 1 = detail
	selL    int
	selR    int
	top     int // model list scroll window
	model   []string
	loading bool
	loadErr string
	spawned int
	wantR   bool // enter pressed while loading — hop to model pane when it arrives
	fleet   []wrappers.Wrapper
	pending *wrappers.Wrapper // confirm gate: docker spawn awaiting y/n
	status  string
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme, prov provider.Provider, st *session.Store) *Model {
	return &Model{th: th, prov: prov, st: st, fleet: wrappers.List()}
}
func (m *Model) Title() string { return "LAUNCH" }
func (m *Model) ID() string    { return "launch" }
func (m *Model) Init() tea.Cmd { return nil }
func (m *Model) Activate() tea.Cmd {
	if m.prov != nil && m.model == nil && !m.loading {
		m.loading = true
		return loadCmd(m.prov)
	}
	return nil
}
func (m *Model) Deactivate() {}

// Esc has no in-tab meaning here; the shell may quit.
func (m *Model) Esc() bool { return false }

func loadCmd(prov provider.Provider) tea.Cmd {
	return func() tea.Msg {
		ms := prov.Models()
		if ms == nil {
			return modelsMsg{err: fmt.Errorf("no models (is demo-desktop ollama up?)")}
		}
		return modelsMsg{models: ms}
	}
}
func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tabs.SetThemeMsg:
		m.th = msg.Theme
		return m, nil
	case modelsMsg:
		m.loading = false
		if msg.err != nil {
			m.loadErr = msg.err.Error()
			return m, nil
		}
		m.loadErr = ""
		m.model = msg.models
		if m.wantR {
			m.wantR = false
			m.pane = 1
		}
		return m, nil
	case tea.KeyPressMsg:
		// confirm gate swallows everything until y/n
		if m.pending != nil {
			switch msg.String() {
			case "y", "Y":
				w := *m.pending
				m.pending = nil
				np, err := wrap.New(m.th, w)
				if err != nil {
					m.status = "⚠ " + err.Error()
					return m, nil
				}
				m.spawned++
				return m, func() tea.Msg { return tabs.SpawnMsg{Tab: np} }
			case "n", "N", "esc":
				m.pending = nil
				m.status = "canceled"
			}
			return m, nil
		}
		switch msg.String() {
		case "h", "left":
			m.pane = 0
		case "l", "right":
			if m.sources()[m.selL].kind == srcModels && len(m.model) > 0 {
				m.pane = 1
			}
		case "r", "R":
			if m.sources()[m.selL].kind == srcModels && m.prov != nil {
				m.loading = true
				m.loadErr = ""
				return m, loadCmd(m.prov)
			}
			m.fleet = wrappers.List() // rescan wrapper specs too
			m.status = "wrappers rescanned"
		case "enter":
			if m.pane == 0 {
				src := m.sources()[m.selL]
				if src.kind == srcModels {
					if len(m.model) > 0 {
						m.pane = 1
					} else if m.prov != nil {
						if !m.loading {
							m.loading = true
							return m, loadCmd(m.prov)
						}
						m.wantR = true // list on the way — enter it when it lands
					}
					return m, nil
				}
				// wrapper: confirm-gated docker spawn
				for i := range m.fleet {
					if m.fleet[i].Name == src.name {
						m.status = ""
						m.pending = &m.fleet[i]
						return m, nil
					}
				}
				return m, nil
			}
			if len(m.model) > 0 {
				model := m.model[m.selR]
				m.spawned++
				np := chat.NewSession(m.th, m.prov, model, m.st)
				return m, func() tea.Msg { return tabs.SpawnMsg{Tab: np} }
			}
		}
		if m.pane == 0 {
			if n, ok := tabs.MoveSel(m.selL, len(m.sources()), msg); ok {
				m.selL = n
			}
		} else if n, ok := tabs.MoveSel(m.selR, len(m.model), msg); ok {
			m.selR = n
		}
	}
	return m, nil
}

// sources exists so the slice is addressable per-selection.
func (m *Model) sources() []source {
	out := []source{{"ollama", "live models on demo-desktop · enter spawns a chat session", srcModels}}
	for _, w := range m.fleet {
		d := w.Desc
		if d == "" {
			d = "docker wrapper"
		}
		if w.Image == "" {
			d += " · image not set"
		}
		out = append(out, source{w.Name, d, srcWrapper})
	}
	return out
}
func (m *Model) Resize(w, h int) { m.w, m.h = w, h }
func (m *Model) box(focused bool) lipgloss.Style {
	b := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.th.Colors.Muted)).
		Padding(0, 1)
	if focused {
		b = b.BorderForeground(m.th.AccentColor())
	}
	return b
}
func (m *Model) leftCol() string {
	var rows []string
	for i, s := range m.sources() {
		marker := "  "
		label := s.name
		switch {
		case i == m.selL && m.pane == 0:
			rows = append(rows, m.th.Accent().Render("▶ "+label))
		case i == m.selL:
			rows = append(rows, m.th.Accent().Render(marker+label))
		default:
			rows = append(rows, m.th.Muted().Render(marker+label))
		}
	}
	return m.box(m.pane == 0).Render(strings.Join(rows, "\n"))
}
func (m *Model) rightCol() string {
	sel := m.sources()[m.selL]
	var body string
	switch sel.kind {
	case srcWrapper:
		w, ok := wrappers.Get(sel.name)
		cmd := wrappers.Summary(w)
		if !ok {
			cmd = "spec not found"
		}
		body = m.th.Accent().Render(sel.name) + "\n" +
			m.th.Secondary().Render(sel.desc) + "\n\n" +
			m.th.Muted().Render("runs in docker · confirm-gated") + "\n" +
			m.th.Muted().Render(tabs.Trunc(cmd, m.w/2))
	default:
		switch {
		case m.loading:
			body = m.th.Muted().Render("loading models…")
		case m.loadErr != "":
			body = m.th.Tertiary().Render(m.loadErr) + "\n" +
				m.th.Muted().Render("r retry")
		case len(m.model) == 0:
			body = m.th.Muted().Render("no models · r retry")
		default:
			vis := m.h - 8
			if vis < 3 {
				vis = 3
			}
			if vis > len(m.model) {
				vis = len(m.model)
			}
			if m.selR < m.top {
				m.top = m.selR
			}
			if m.selR >= m.top+vis {
				m.top = m.selR - vis + 1
			}
			var rows []string
			for i := m.top; i < m.top+vis && i < len(m.model); i++ {
				name := tabs.Trunc(m.model[i], 44)
				if i == m.selR && m.pane == 1 {
					rows = append(rows, m.th.Accent().Render("▶ "+name))
				} else if i == m.selR {
					rows = append(rows, m.th.Accent().Render("  "+name))
				} else {
					rows = append(rows, m.th.Muted().Render("  "+name))
				}
			}
			count := fmt.Sprintf("%d/%d", m.selR+1, len(m.model))
			body = strings.Join(rows, "\n") + "\n" + m.th.Secondary().Render(count)
		}
	}
	head := m.th.Secondary().Render(sel.name)
	return m.box(m.pane == 1).Render(head + "\n" + body)
}
func (m *Model) View() string {
	hint := "h/l pane · j/k move · enter spawn session · r reload"
	if m.spawned > 0 {
		hint = fmt.Sprintf("%s · %d spawned this visit", hint, m.spawned)
	}
	if m.status != "" {
		hint += " · " + m.status
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.leftCol(), "  ", m.rightCol())
	out := m.th.Secondary().Render("launcher") + "  " + m.th.Muted().Render(hint) + "\n" + body
	if m.pending != nil {
		out = m.confirmOverlay(out)
	}
	return out
}

// confirmOverlay draws the y/n docker gate centered over the launcher.
func (m *Model) confirmOverlay(body string) string {
	rows := []string{
		m.th.TabActive().Render(" RUN IN DOCKER? "),
		"",
		m.th.Accent().Render(m.pending.Name),
		m.th.Muted().Render(tabs.Trunc(wrappers.Summary(*m.pending), 56)),
		"",
		m.th.Muted().Render("y run · n cancel"),
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.th.AccentColor()).
		Padding(1, 3).
		Render(strings.Join(rows, "\n"))

	bw := lipgloss.Width(box)
	bh := lipgloss.Height(box)
	lines := strings.Split(body, "\n")
	top := max(0, (m.h-bh)/2)
	left := max(0, (m.w-bw)/2)
	for y := 0; y < bh && top+y < len(lines); y++ {
		row := lines[top+y]
		pad := left
		if pad > len(row) {
			pad = len(row)
		}
		lines[top+y] = row[:pad] + strings.SplitN(box, "\n", bh)[y]
	}
	return strings.Join(lines, "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
