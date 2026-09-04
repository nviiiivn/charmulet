// Package files is the file browser/viewer: j/k navigation, enter to open
// directories and files, glamour-rendered markdown reading.
package files

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/glamour"

	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
)

type entry struct {
	name  string
	isDir bool
}

type Model struct {
	entries []entry
	cursor  int
	cwd     string
	w, h    int
	th      theme.Theme

	viewing  bool
	vp       viewport.Model
	viewPath string
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme) *Model {
	return &Model{th: th}
}

func (m *Model) Title() string { return "FILES" }
func (m *Model) ID() string    { return "files" }

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Activate() tea.Cmd {
	if m.cwd == "" {
		if err := m.load("."); err != nil {
			return func() tea.Msg { return errMsg{err} }
		}
	}
	return nil
}

func (m *Model) Deactivate() {}

// Esc closes the file viewer; otherwise the shell may quit.
func (m *Model) Esc() bool {
	if m.viewing {
		m.viewing = false
		return true
	}
	return false
}

func (m *Model) load(dir string) error {
	des, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	m.entries = m.entries[:0]
	for _, de := range des {
		if strings.HasPrefix(de.Name(), ".") {
			continue
		}
		m.entries = append(m.entries, entry{name: de.Name(), isDir: de.IsDir()})
	}
	sortEntries(m.entries)
	abs, err := filepath.Abs(dir)
	if err == nil {
		m.cwd = abs
	}
	parent := filepath.Dir(m.cwd)
	if parent != m.cwd {
		m.entries = append([]entry{{name: "..", isDir: true}}, m.entries...)
	}
	m.cursor = 0
	return nil
}

func sortEntries(es []entry) {
	for i := range es {
		for j := i + 1; j < len(es); j++ {
			a, b := es[i], es[j]
			if b.isDir && !a.isDir || (a.isDir == b.isDir && a.name > b.name) {
				es[i], es[j] = es[j], es[i]
			}
		}
	}
}

type errMsg struct{ err error }

func (m *Model) Resize(w, h int) {
	m.w, m.h = w, h
	if m.viewing {
		m.layoutViewer()
	}
}

// layoutViewer sizes the viewport and re-renders content at current width.
func (m *Model) layoutViewer() {
	m.vp.SetWidth(m.w)
	m.vp.SetHeight(m.h - 1)
	if raw, err := os.ReadFile(m.viewPath); err == nil {
		m.vp.SetContent(m.render(string(raw)))
	}
}

// render pipes content through glamour for markdown, passthrough otherwise.
func (m *Model) render(raw string) string {
	if strings.HasSuffix(strings.ToLower(m.viewPath), ".md") || strings.HasSuffix(strings.ToLower(m.viewPath), ".markdown") {
		w := m.w
		if w <= 0 {
			w = 80
		}
		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(min(w-2, 100)),
		)
		if err != nil {
			return raw
		}
		out, err := r.Render(raw)
		if err != nil {
			return raw
		}
		return out
	}
	return raw
}

func (m *Model) open(path string, isDir bool) tea.Cmd {
	if isDir {
		if err := m.load(path); err != nil {
			return func() tea.Msg { return errMsg{err} }
		}
		return nil
	}
	m.viewing = true
	m.viewPath = path
	m.layoutViewer()
	return nil
}

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	if tm, ok := msg.(tabs.SetThemeMsg); ok {
		m.th = tm.Theme
		return m, nil
	}

	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	if m.viewing {
		switch key.String() {
		case "q":
			m.viewing = false
			return m, nil
		default:
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		}
	}

	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
		}
	case "enter":
		e := m.entries[m.cursor]
		target := e.name
		if target == ".." {
			target = filepath.Dir(m.cwd)
		} else if !filepath.IsAbs(target) {
			target = filepath.Join(m.cwd, target)
		}
		return m, m.open(target, e.isDir)
	case "r":
		if err := m.load("."); err != nil {
			return m, func() tea.Msg { return errMsg{err} }
		}
	}
	return m, nil
}

func (m *Model) View() string {
	if m.viewing {
		header := m.th.Muted().Render(m.viewPath) + "\n"
		return header + m.vp.View()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", m.th.Muted().Render(m.cwd))
	maxRows := m.h - 4
	if maxRows <= 0 {
		maxRows = 10
	}
	for i, e := range m.entries {
		if i >= maxRows {
			break
		}
		label := e.name
		if e.isDir && e.name != ".." {
			label += "/"
		}
		switch i {
		case m.cursor:
			b.WriteString(m.th.TabActive().Render("> "+label) + "\n")
		default:
			if e.isDir {
				b.WriteString(m.th.Secondary().Render("  "+label) + "\n")
			} else {
				b.WriteString("  " + label + "\n")
			}
		}
	}
	fmt.Fprintf(&b, "\n%s", m.th.Muted().Render(fmt.Sprintf("%d entries · j/k move · enter open · r refresh · esc back", len(m.entries))))
	return b.String()
}
