// Package files is the file browser/viewer. P0 lists the working directory;
// the viewer pane and git-status awareness arrive in P2.
package files

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
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
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme) *Model {
	return &Model{th: th}
}

func (m *Model) Title() string { return "FILES" }
func (m *Model) ID() string    { return "files" }

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Activate() tea.Cmd {
	if err := m.load("."); err != nil {
		return func() tea.Msg { return errMsg{err} }
	}
	return nil
}

func (m *Model) Deactivate() {}

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
	// directories first, then names
	for i := range m.entries {
		for j := i + 1; j < len(m.entries); j++ {
			a, b := m.entries[i], m.entries[j]
			if b.isDir && !a.isDir || (a.isDir == b.isDir && a.name > b.name) {
				m.entries[i], m.entries[j] = m.entries[j], m.entries[i]
			}
		}
	}
	m.cwd, _ = os.Getwd()
	m.cursor = 0
	return nil
}

type errMsg struct{ err error }

func (m *Model) Resize(w, h int) { m.w, m.h = w, h }

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
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
	case "r":
		if err := m.load("."); err != nil {
			return m, func() tea.Msg { return errMsg{err} }
		}
	}
	return m, nil
}

func (m *Model) View() string {
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
		if e.isDir {
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
	fmt.Fprintf(&b, "\n%s", m.th.Muted().Render(fmt.Sprintf("%d entries · j/k move · r refresh", len(m.entries))))
	return b.String()
}
