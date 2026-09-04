// Package shell is the CHARMULET app shell: tab bar, status bar, and the
// router that owns the active tab. Global keybinds are handled here; all
// other messages route to the focused tab.
package shell

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
)

const (
	tabBarHeight = 1
	statusHeight = 1
)

type Model struct {
	tabs        []tabs.Tab
	core        map[string]bool // tab IDs that cannot be closed (ctrl+w)
	active      int
	w, h        int
	th          theme.Theme
	status      string // transient message line (errors etc.)
	help        bool   // help overlay screen
	splash      bool   // intro splash screen (dismiss: any key or timer)
	confirmQuit bool   // ctrl+q / ctrl+c gate: y quits, anything else stays
}

var _ tea.Model = (*Model)(nil)

func New(th theme.Theme, ts ...tabs.Tab) *Model {
	core := make(map[string]bool, len(ts))
	for _, t := range ts {
		core[t.ID()] = true
	}
	return &Model{tabs: ts, core: core, th: th, splash: true}
}

type splashTickMsg struct{}

func splashCmd() tea.Cmd {
	return tea.Tick(1600*time.Millisecond, func(time.Time) tea.Msg { return splashTickMsg{} })
}

func (m *Model) Active() tabs.Tab { return m.tabs[m.active] }

// spawn appends a tab, focuses it, and returns the cmds to run.
func (m *Model) spawn(t tabs.Tab) tea.Cmd {
	m.Active().Deactivate()
	m.tabs = append(m.tabs, t)
	t.Resize(m.w, bodyHeight(m))
	m.active = len(m.tabs) - 1
	return tea.Batch(t.Init(), t.Activate())
}

// close removes the active tab unless it is core. Returns focus cmd.
func (m *Model) close() {
	if m.core[m.Active().ID()] {
		m.status = "core tabs can't be closed"
		return
	}
	m.Active().Deactivate()
	m.tabs = append(m.tabs[:m.active], m.tabs[m.active+1:]...)
	if m.active >= len(m.tabs) {
		m.active = len(m.tabs) - 1
	}
	if m.active < 0 {
		m.active = 0
	}
	m.status = ""
}

func bodyHeight(m *Model) int {
	h := m.h - tabBarHeight - statusHeight
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.Active().Activate(), splashCmd())
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		for _, t := range m.tabs {
			t.Resize(m.w, bodyHeight(m))
		}
		return m, nil

	case tea.KeyPressMsg:
		if m.splash {
			m.splash = false
			return m, nil // swallow the dismissing key
		}
		if m.confirmQuit {
			switch msg.String() {
			case "y", "Y", "enter":
				return m, tea.Quit
			default:
				m.confirmQuit = false
				return m, nil // any other key stays
			}
		}
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			m.confirmQuit = true
			return m, nil
		case "esc":
			if m.help {
				m.help = false
				return m, nil
			}
			// esc backs out of the active thing (cancel stream,
			// close viewer/overlay); it NEVER quits the app
			m.Active().Esc()
			return m, nil
		case "?":
			m.help = !m.help
			return m, nil
		}
		if m.help {
			return m, nil // overlay swallows everything else
		}
		switch msg.String() {
		case "tab":
			m.Active().Deactivate()
			m.active = (m.active + 1) % len(m.tabs)
			return m, m.Active().Activate()
		case "shift+tab":
			m.Active().Deactivate()
			m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
			return m, m.Active().Activate()
		case "ctrl+w":
			m.close()
			return m, m.Active().Activate()
		default:
			// number keys 1-9 jump straight to a tab
			if len(msg.String()) == 1 && msg.String()[0] >= '1' && msg.String()[0] <= '9' {
				idx := int(msg.String()[0] - '1')
				if idx < len(m.tabs) && idx != m.active {
					m.Active().Deactivate()
					m.active = idx
					return m, m.Active().Activate()
				}
				return m, nil
			}
		}

	case splashTickMsg:
		m.splash = false
		return m, nil

	case tabs.SpawnMsg:
		return m, m.spawn(msg.Tab)

	case tabs.SetThemeMsg:
		m.th = msg.Theme
		for i, t := range m.tabs {
			nt, _ := t.Update(msg)
			m.tabs[i] = nt
		}
		return m, nil

	case errMsg:
		m.status = "error: " + msg.err.Error()
		return m, nil
	}

	// messages owned by a tab (streams, poll results) reach that tab even
	// when it is not focused, so background work keeps flowing
	if tm, ok := msg.(tabs.TabMsg); ok {
		for i, t := range m.tabs {
			if t.ID() == tm.TabID() {
				nt, cmd := t.Update(msg)
				m.tabs[i] = nt
				return m, cmd
			}
		}
		return m, nil // owner closed; drop
	}

	// everything else goes to the focused tab
	newTab, cmd := m.Active().Update(msg)
	m.tabs[m.active] = newTab
	return m, cmd
}

type errMsg struct{ err error }

// overlay draws the help screen centered over a dimmed body.
func (m *Model) overlay(body string) string {
	rows := []string{
		m.th.TabActive().Render(" CHARMULET "),
		"",
		m.th.Accent().Render("tabs") + "     tab / shift+tab cycle · 1-9 jump · ctrl+w close spawned",
		m.th.Accent().Render("quit") + "     ctrl+q (asks first) · esc backs out, never quits",
		m.th.Accent().Render("chat") + "     enter send · esc cancel stream",
		m.th.Accent().Render("launch") + "   pick model → spawns a parallel chat session",
		m.th.Accent().Render("sessions") + " resume past conversations · enter reopen",
		m.th.Accent().Render("agents") + "   edit agent TOML · skills on/off · n new",
		m.th.Accent().Render("settings") + " live theme switch · enter apply + save",
		m.th.Accent().Render("runs") + "     fan-out prompt to many models",
		m.th.Accent().Render("files") + "    j/k move · enter open · r refresh",
		m.th.Accent().Render("viewer") + "  esc/q close file",
		m.th.Accent().Render("services") + " homelab cards · r refresh · j/k select",
		m.th.Accent().Render("fronts") + "  url checks · r refresh · j/k select",
		"",
		m.th.Muted().Render("? close this screen · ctrl+c quit"),
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
		ln := lines[top+y]
		pad := left
		if short := m.w - lipgloss.Width(ln); short < pad {
			pad = max(0, short)
			lines[top+y] = ln + strings.Repeat(" ", pad) + boxLines(box)[y]
			continue
		}
		lines[top+y] = ln + strings.Repeat(" ", pad) + boxLines(box)[y]
	}
	return strings.Join(lines, "\n")
}

func boxLines(box string) []string { return strings.Split(box, "\n") }

// quitOverlay asks for confirmation before quitting so no session or draft
// is lost to a stray ctrl+q / ctrl+c.
func (m *Model) quitOverlay(body string) string {
	rows := []string{
		m.th.TabActive().Render(" QUIT CHARMULET? "),
		"",
		m.th.Accent().Render("y") + " quit · " + m.th.Secondary().Render("any other key stays"),
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
		ln := lines[top+y]
		pad := left
		if short := m.w - lipgloss.Width(ln); short < pad {
			pad = max(0, short)
		}
		lines[top+y] = ln + strings.Repeat(" ", pad) + boxLines(box)[y]
	}
	return strings.Join(lines, "\n")
}

// splashView renders the intro: the word charmulet in cycling pride colors.
func (m *Model) splashView() string {
	cols := []color.Color{m.th.AccentColor(), m.th.SecondaryColor(), m.th.TertiaryColor()}
	var word strings.Builder
	for i, r := range "charmulet" {
		word.WriteString(lipgloss.NewStyle().Bold(true).Foreground(cols[i%len(cols)]).Render(string(r)))
	}

	rows := []string{
		m.th.Muted().Render("✦ ✧ ✦"),
		"",
		word.String(),
		m.th.Secondary().Render("the agentic development environment"),
		"",
		m.th.Muted().Render("press any key"),
	}
	box := strings.Join(rows, "\n")

	top := max(0, (m.h-lipgloss.Height(box))/2)
	left := max(0, (m.w-lipgloss.Width(box))/2)

	var lines []string
	for i := 0; i < top; i++ {
		lines = append(lines, "")
	}
	for _, r := range strings.Split(box, "\n") {
		lines = append(lines, strings.Repeat(" ", left)+r)
	}
	for len(lines) < m.h {
		lines = append(lines, "")
	}
	return m.th.Body().Render(strings.Join(lines, "\n"))
}

func (m *Model) View() tea.View {
	if m.w == 0 {
		return tea.NewView("loading…")
	}

	if m.splash {
		v := tea.NewView(m.splashView())
		v.AltScreen = true
		return v
	}

	// --- tab bar ---
	var slots []string
	for i, t := range m.tabs {
		label := fmt.Sprintf(" %d %s ", i+1, t.Title())
		if i == m.active {
			slots = append(slots, m.th.TabActive().Render(label))
		} else {
			slots = append(slots, m.th.TabInactive().Render(label))
		}
	}
	bar := strings.Join(slots, "")
	if pad := m.w - lipgloss.Width(bar); pad > 0 {
		bar += strings.Repeat(" ", pad)
	}

	// --- active tab body, padded to full width ---
	body := m.Active().View()
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		if short := m.w - lipgloss.Width(ln); short > 0 {
			lines[i] = ln + strings.Repeat(" ", short)
		}
	}
	for len(lines) < bodyHeight(m) {
		lines = append(lines, strings.Repeat(" ", m.w))
	}
	body = m.th.Body().Render(strings.Join(lines, "\n"))

	if m.help {
		body = m.overlay(body)
	}
	if m.confirmQuit {
		body = m.quitOverlay(body)
	}

	// --- status bar ---
	status := m.status
	if status == "" {
		status = fmt.Sprintf("charmulet · theme:%s · %s", m.th.Name, m.Active().ID())
	}
	sb := m.th.StatusBar().Render(status)
	if pad := m.w - lipgloss.Width(sb); pad > 0 {
		sb += strings.Repeat(" ", pad)
	}

	v := tea.NewView(bar + "\n" + body + "\n" + sb)
	v.AltScreen = true
	return v
}
