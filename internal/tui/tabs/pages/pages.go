// Package pages renders user-defined dashboard pages (TOML grids of
// registered components) from the dashbored feed. Read-only.
package pages

import (
	"context"
	"image/color"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nvii/charmulet/internal/homelab"
	dashpages "github.com/nvii/charmulet/internal/pages"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
)

const pollEvery = 10 * time.Second

type tickMsg struct{ gen int }
type fetchedMsg struct {
	gen   int
	data  map[string]homelab.Machine
	err   error
	pages []dashpages.Page
}

type Model struct {
	th      theme.Theme
	dashURL string
	w, h    int

	pages  []dashpages.Page
	cur    int
	data   map[string]homelab.Machine
	err    string
	loaded bool
	gen    int
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme, dashURL string) *Model {
	return &Model{th: th, dashURL: dashURL}
}

func (m *Model) Title() string { return "PAGES" }
func (m *Model) ID() string    { return "pages" }
func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Activate() tea.Cmd {
	m.gen++
	dashpages.EnsureDefault() // seed home page on first use
	return m.fetch(m.gen)
}

func (m *Model) Deactivate() {}

func (m *Model) fetch(gen int) tea.Cmd {
	url := m.dashURL
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		data, err := homelab.Fetch(ctx, url)
		return fetchedMsg{gen: gen, data: data, err: err, pages: dashpages.List()}
	}
}

func tick(gen int) tea.Cmd {
	return tea.Tick(pollEvery, func(time.Time) tea.Msg { return tickMsg{gen: gen} })
}

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tabs.SetThemeMsg:
		m.th = msg.Theme
		return m, nil

	case tickMsg:
		if msg.gen != m.gen {
			return m, nil // stale chain
		}
		return m, m.fetch(m.gen)

	case fetchedMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.loaded = true
		m.err = ""
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, tick(m.gen)
		}
		m.data = msg.data
		if len(msg.pages) > 0 {
			m.pages = msg.pages
			if m.cur >= len(m.pages) {
				m.cur = len(m.pages) - 1
			}
		}
		return m, tick(m.gen)

	case tea.KeyPressMsg:
		switch msg.String() {
		case "left", "h":
			if m.cur > 0 {
				m.cur--
			}
		case "right", "l":
			if m.cur < len(m.pages)-1 {
				m.cur++
			}
		case "r", "R":
			m.gen++
			return m, m.fetch(m.gen)
		}
		if n, ok := tabs.MoveSel(m.cur, len(m.pages), msg); ok {
			m.cur = n
		}
	}
	return m, nil
}

func (m *Model) Resize(w, h int) { m.w, m.h = w, h }

func (m *Model) Esc() bool { return false }

func (m *Model) View() string {
	var b strings.Builder
	b.WriteString(m.th.Secondary().Render("pages") + "  " +
		m.th.Muted().Render("h/l switch page · j/k move · r refresh") + "\n")

	switch {
	case m.err != "":
		b.WriteString(m.th.Tertiary().Render("⚠ " + m.err))
	case !m.loaded:
		b.WriteString(m.th.Muted().Render("loading…"))
	case len(m.pages) == 0:
		b.WriteString(m.th.Muted().Render("no pages — drop a TOML in " + dashpages.Dir()))
	default:
		p := m.pages[m.cur]
		b.WriteString(m.renderPage(p))
	}
	out := b.String()

	// page indicator
	if len(m.pages) > 0 {
		ind := m.th.Secondary().Render(
			strings.TrimSpace(m.pages[m.cur].Name) + " · " +
				strconv.Itoa(m.cur+1) + "/" + strconv.Itoa(len(m.pages)))
		out += "\n" + ind
	}
	return out
}

func (m *Model) renderPage(p dashpages.Page) string {
	cols := p.Columns
	if cols < 1 || cols > 3 {
		cols = 2
	}
	gap := 2
	colW := (m.w - (cols-1)*gap - 2) / cols
	if colW < 10 {
		colW = 10
	}

	colors := []func() lipgloss.Style{m.th.Accent, m.th.Secondary, m.th.Tertiary}
	deps := dashpages.Deps{Width: colW - 4, Now: time.Now(), Machines: m.data}

	boxes := make([]string, 0, len(p.Widgets))
	for i, w := range p.Widgets {
		style := colors[i%len(colors)]()
		title := w.Title
		if title == "" {
			title = w.Kind
			if w.Machine != "" {
				title += " · " + w.Machine
			}
		}
		body := dashpages.Render(w, deps)
		box := style.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.colorFor(i)).
			Padding(0, 1).
			Width(colW).
			Render(m.th.TabActive().Render(" "+title+" ") + "\n" + body)
		boxes = append(boxes, box)
	}

	// row-major fill
	var rows []string
	for i := 0; i < len(boxes); i += cols {
		end := i + cols
		if end > len(boxes) {
			end = len(boxes)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, boxes[i:end]...))
	}
	grid := lipgloss.JoinVertical(lipgloss.Left, rows...)

	// clamp to body height
	maxH := m.h - 3
	if maxH < 1 {
		maxH = 1
	}
	lines := strings.Split(grid, "\n")
	if len(lines) > maxH {
		lines = lines[:maxH]
	}
	return strings.Join(lines, "\n")
}

// colorFor rotates the pride accents across widget boxes.
func (m *Model) colorFor(i int) color.Color {
	switch i % 3 {
	case 0:
		return m.th.AccentColor()
	case 1:
		return m.th.SecondaryColor()
	default:
		return m.th.TertiaryColor()
	}
}
