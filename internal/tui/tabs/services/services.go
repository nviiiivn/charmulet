// Package services is the homelab dashboard tab: live read-only cards for
// every machine, fed by the dashbored /api/dashboard endpoint.
package services

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nvii/charmulet/internal/homelab"
	"github.com/nvii/charmulet/internal/tui/layout"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
)

const (
	refreshEvery = 5 * time.Second
	innerWidth   = 38
)

type tickMsg struct{}
type fetchedMsg struct {
	data map[string]homelab.Machine
	err  error
}

type Model struct {
	th   theme.Theme
	url  string
	w, h int

	data    map[string]homelab.Machine
	order   []string
	sel     int
	last    time.Time
	lastErr string
	loading bool
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme, dashURL string) *Model {
	return &Model{th: th, url: dashURL}
}

func (m *Model) Title() string { return "SERVICES" }
func (m *Model) ID() string    { return "services" }

func (m *Model) Init() tea.Cmd { return fetchCmd(m.url) }

func (m *Model) Activate() tea.Cmd {
	m.loading = true
	return fetchCmd(m.url)
}

func (m *Model) Deactivate() {}

func fetchCmd(url string) tea.Cmd {
	return func() tea.Msg {
		data, err := homelab.Fetch(context.Background(), url)
		return fetchedMsg{data: data, err: err}
	}
}

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		return m, fetchCmd(m.url)
	case fetchedMsg:
		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			return m, tick()
		}
		m.data, m.lastErr = msg.data, ""
		m.order = homelab.Ordered(msg.data)
		if m.sel >= len(m.order) {
			m.sel = len(m.order) - 1
		}
		if m.sel < 0 {
			m.sel = 0
		}
		m.last = time.Now()
		return m, tick()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "r", "R":
			m.loading = true
			return m, fetchCmd(m.url)
		case "j", "down":
			if m.sel < len(m.order)-1 {
				m.sel++
			}
		case "k", "up":
			if m.sel > 0 {
				m.sel--
			}
		}
	}
	return m, nil
}

func (m *Model) Resize(w, h int) { m.w, m.h = w, h }

// bar renders a text meter: filled blocks in the fill style, dots as track.
func bar(th theme.Theme, pct float64, width int, fill lipgloss.Style) string {
	if width < 4 {
		width = 4
	}
	filled := int(math.Round(pct / 100 * float64(width)))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return fill.Render(strings.Repeat("█", filled)) + th.Muted().Render(strings.Repeat("·", width-filled))
}

func pct(p float64) string { return fmt.Sprintf("%.0f%%", p) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

// gpuParts splits the dashbored gpu string "NAME, USED MiB, TOTAL MiB, P %".
func gpuParts(g string) (name, vram string) {
	parts := strings.Split(g, ",")
	name = strings.TrimSpace(parts[0])
	if len(parts) >= 3 {
		vram = strings.TrimSpace(parts[1]) + "/" + strings.TrimSpace(parts[2]) + " MiB"
	}
	return name, vram
}

func (m *Model) card(key string, mach homelab.Machine, selected bool) string {
	dot := lipgloss.NewStyle().Foreground(lipgloss.Color(mach.Color)).Render("●")
	name := mach.Display
	if name == "" {
		name = key
	}
	head := dot + " " + lipgloss.NewStyle().Bold(true).Render(name)
	if selected {
		head = m.th.Accent().Render("▶ ") + head
	}

	var lines []string
	sys := mach.Widgets.System
	switch {
	case sys == nil || sys.Error != "":
		lines = append(lines, m.th.Muted().Render("unreachable"))
	default:
		cpuL := "cpu  " + bar(m.th, sys.CPUPercent, 12, m.th.Accent()) + " " + pad(pct(sys.CPUPercent), 4)
		memL := "mem  " + bar(m.th, sys.MemPercent, 12, m.th.Secondary()) + " " + pad(pct(sys.MemPercent), 4)
		diskL := "disk " + bar(m.th, sys.DiskPercent, 12, m.th.Tertiary()) + " " + pad(pct(sys.DiskPercent), 4)
		if sys.DiskPercent >= 85 {
			diskL += m.th.Tertiary().Render(" !")
		}
		lines = append(lines, cpuL, memL, diskL)

		info := "up " + sys.Uptime
		if sys.Temp != "" {
			info += " · " + sys.Temp
		}
		lines = append(lines, m.th.Muted().Render(truncate(info, innerWidth)))

		if sys.GPU != "" {
			gname, vram := gpuParts(sys.GPU)
			g := "gpu  " + truncate(gname, innerWidth-14) + " " + pct(sys.GPUPercent)
			if vram != "" {
				g += " " + vram
			}
			lines = append(lines, m.th.Secondary().Render(truncate(g, innerWidth)))
		}
	}

	if sw := mach.Widgets.Services; sw != nil && len(sw.Services) > 0 {
		up := 0
		for _, s := range sw.Services {
			if s.Active {
				up++
			}
		}
		s := fmt.Sprintf("svcs %d/%d up", up, len(sw.Services))
		if up < len(sw.Services) {
			s += fmt.Sprintf(" · %d down", len(sw.Services)-up)
			lines = append(lines, m.th.Tertiary().Render(truncate(s, innerWidth)))
		} else {
			lines = append(lines, m.th.Secondary().Render(truncate(s, innerWidth)))
		}
	}

	if ow := mach.Widgets.Ollama; ow != nil {
		o := fmt.Sprintf("ollama %d models", len(ow.Models))
		if n := len(ow.Running); n > 0 {
			o += fmt.Sprintf(" · %d running", n)
		}
		lines = append(lines, m.th.Accent().Render(truncate(o, innerWidth)))
	}

	if mw := mach.Widgets.Media; mw != nil && mw.NowPlaying != "" {
		lines = append(lines, m.th.Muted().Render(truncate("♪ "+mw.NowPlaying, innerWidth)))
	}

	body := lipgloss.NewStyle().Width(innerWidth).Render(strings.Join(lines, "\n"))

	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(mach.Color)).
		Padding(0, 1)
	if selected {
		border = border.BorderForeground(m.th.AccentColor())
	}
	return border.Render(head + "\n" + body)
}

func (m *Model) View() string {
	var head string
	stamp := "loading…"
	if !m.last.IsZero() {
		stamp = m.last.Format("15:04:05")
	}
	if m.loading {
		stamp += " · refreshing"
	}
	head = m.th.Secondary().Render("dashbored") + m.th.Muted().Render(" · "+stamp)
	if m.lastErr != "" {
		head += " " + m.th.Tertiary().Render(truncate(m.lastErr, m.w-lipgloss.Width(head)-2))
	}

	if len(m.order) == 0 {
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.th.AccentColor()).
			Padding(1, 2)
		msg := "waiting for feed…"
		if m.lastErr != "" {
			msg = "feed error: " + m.lastErr
		}
		return head + "\n\n" + box.Render(m.th.Muted().Render(msg))
	}

	cols := 1
	cardTotal := innerWidth + 4
	if m.w >= cardTotal*2+2 {
		cols = 2
	}

	cards := make([]string, 0, len(m.order))
	for i, key := range m.order {
		cards = append(cards, m.card(key, m.data[key], i == m.sel))
	}

	body := layout.Grid(cols, cards...)
	foot := m.th.Muted().Render("r refresh · j/k select")
	return head + "\n" + body + "\n" + foot
}
