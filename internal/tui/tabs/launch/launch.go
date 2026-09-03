// Package launch is the launcher tab: pick a model (or wrapper) and spawn
// parallel chat sessions. Wrappers run in docker — the worker layer lands
// with P4; until then they list read-only.
package launch

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nvii/charmulet/internal/core/provider"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/tabs/chat"
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

// wrappers is the locked fleet list (her decision): main CLIs/agents/tools.
var sources = []source{
	{"ollama", "live models on demo-desktop · enter spawns a chat session", srcModels},
	{"hax", "agent cli", srcWrapper},
	{"crush", "charm-stack agent (Go)", srcWrapper},
	{"qwen", "qwen cli", srcWrapper},
	{"claude", "claude code", srcWrapper},
	{"codex", "openai codex cli", srcWrapper},
	{"opencode", "opencode", srcWrapper},
	{"hermes", "hermes agent · server on demo-desktop", srcWrapper},
	{"openclaw", "openclaw gateway", srcWrapper},
	{"agent-zero", "a0 · demo-desktop (sleeping)", srcWrapper},
	{"totem", "her current cli", srcWrapper},
}

type modelsMsg struct {
	models []string
	err    error
}

type Model struct {
	th   theme.Theme
	prov provider.Provider
	w, h int

	pane  int // 0 = sources, 1 = detail
	selL  int
	selR  int
	top   int // model list scroll window
	model []string

	loading bool
	loadErr string
	spawned int
	wantR   bool // enter pressed while loading — hop to model pane when it arrives
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme, prov provider.Provider) *Model {
	return &Model{th: th, prov: prov}
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
		switch msg.String() {
		case "j", "down":
			if m.pane == 0 && m.selL < len(sources)-1 {
				m.selL++
			}
			if m.pane == 1 && m.selR < len(m.model)-1 {
				m.selR++
			}
		case "k", "up":
			if m.pane == 0 && m.selL > 0 {
				m.selL--
			}
			if m.pane == 1 && m.selR > 0 {
				m.selR--
			}
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
		case "enter":
			if m.pane == 0 {
				if m.sources()[m.selL].kind == srcModels {
					if len(m.model) > 0 {
						m.pane = 1
					} else if m.prov != nil {
						if !m.loading {
							m.loading = true
							return m, loadCmd(m.prov)
						}
						m.wantR = true // list on the way — enter it when it lands
					}
				}
				return m, nil
			}
			if len(m.model) > 0 {
				model := m.model[m.selR]
				m.spawned++
				np := chat.NewSession(m.th, m.prov, model)
				return m, func() tea.Msg { return tabs.SpawnMsg{Tab: np} }
			}
		}
	}
	return m, nil
}

// sources exists so the slice is addressable per-selection.
func (m *Model) sources() []source { return sources }

func (m *Model) Resize(w, h int) { m.w, m.h = w, h }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

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
	for i, s := range sources {
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
	sel := sources[m.selL]
	var body string
	switch sel.kind {
	case srcWrapper:
		body = m.th.Accent().Render(sel.name) + "\n" +
			m.th.Secondary().Render(sel.desc) + "\n\n" +
			m.th.Muted().Render("runs in docker · worker layer lands in P4\n(you'll launch it here — confirm-gated)")
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
				name := truncate(m.model[i], 44)
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
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.leftCol(), "  ", m.rightCol())
	return m.th.Secondary().Render("launcher") + "  " + m.th.Muted().Render(hint) + "\n" + body
}
