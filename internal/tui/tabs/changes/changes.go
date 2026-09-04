// Package changes is the CHANGES tab: a git status file list with a diff
// walkthrough viewer — openchamber's changes pane, files-tab mechanics.
package changes

import (
	"context"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/nvii/charmulet/internal/core/git"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
)

type loadedMsg struct {
	files  []git.File
	branch string
	err    error
}

type diffMsg struct {
	path string
	diff string
	err  error
}

type Model struct {
	th   theme.Theme
	dir  string // repo directory
	w, h int

	files  []git.File
	branch string
	sel    int
	err    string
	loaded bool

	viewing bool
	diffVp  viewport.Model
	diffFor string
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme, dir string) *Model {
	vp := viewport.New()
	return &Model{th: th, dir: dir, diffVp: vp}
}

func (m *Model) Title() string { return "CHANGES" }
func (m *Model) ID() string    { return "changes" }
func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Activate() tea.Cmd { return m.loadCmd() }
func (m *Model) Deactivate()       {}

func (m *Model) loadCmd() tea.Cmd {
	dir := m.dir
	return func() tea.Msg {
		files, err := git.Status(context.Background(), dir)
		branch := git.Branch(context.Background(), dir)
		return loadedMsg{files: files, branch: branch, err: err}
	}
}

func (m *Model) diffCmd(f git.File) tea.Cmd {
	dir := m.dir
	return func() tea.Msg {
		diff, err := git.Diff(context.Background(), dir, f)
		return diffMsg{path: f.Path, diff: diff, err: err}
	}
}

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tabs.SetThemeMsg:
		m.th = msg.Theme
		return m, nil

	case loadedMsg:
		m.loaded = true
		m.err = ""
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.files = msg.files
		m.branch = msg.branch
		if m.sel >= len(m.files) {
			m.sel = len(m.files) - 1
		}
		if m.sel < 0 {
			m.sel = 0
		}
		return m, nil

	case diffMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.viewing = true
		m.diffFor = msg.path
		m.diffVp.SetContent(colorDiff(msg.diff, m.th))
		m.diffVp.GotoTop()
		m.resizeViewer()
		return m, nil

	case tea.KeyPressMsg:
		if m.viewing {
			switch msg.String() {
			case "esc", "q":
				m.viewing = false
				return m, nil
			default:
				var cmd tea.Cmd
				m.diffVp, cmd = m.diffVp.Update(msg)
				return m, cmd
			}
		}
		switch msg.String() {
		case "r", "R":
			return m, m.loadCmd()
		case "enter":
			if len(m.files) > 0 {
				return m, m.diffCmd(m.files[m.sel])
			}
		}
		if n, ok := tabs.MoveSel(m.sel, len(m.files), msg); ok {
			m.sel = n
		}
	}
	return m, nil
}

func (m *Model) Resize(w, h int) {
	m.w, m.h = w, h
	m.resizeViewer()
}

func (m *Model) resizeViewer() {
	m.diffVp.SetWidth(m.w)
	m.diffVp.SetHeight(m.h - 2)
}

// Esc closes the diff viewer when open; otherwise declines.
func (m *Model) Esc() bool {
	if m.viewing {
		m.viewing = false
		return true
	}
	return false
}

func (m *Model) View() string {
	var b strings.Builder
	if m.viewing {
		b.WriteString(m.th.Secondary().Render("diff") + "  " +
			m.th.Muted().Render(m.diffFor+" · esc/q back") + "\n")
		b.WriteString(m.diffVp.View())
		return b.String()
	}

	b.WriteString(m.th.Secondary().Render("changes") + "  " +
		m.th.Muted().Render(m.branch+" · j/k move · enter diff · r refresh") + "\n")

	switch {
	case m.err != "":
		b.WriteString(m.th.Tertiary().Render("⚠ " + m.err))
	case !m.loaded:
		b.WriteString(m.th.Muted().Render("loading…"))
	case len(m.files) == 0:
		b.WriteString(m.th.Muted().Render("clean tree — nothing changed"))
	default:
		vis := m.h - 4
		if vis < 3 {
			vis = 3
		}
		if vis > len(m.files) {
			vis = len(m.files)
		}
		if m.sel < vis-1 {
			// simple windowing: clamp sel into view
		}
		top := 0
		if m.sel >= vis {
			top = m.sel - vis + 1
		}
		for i := top; i < top+vis && i < len(m.files); i++ {
			f := m.files[i]
			row := f.Label() + " " + tabs.Trunc(f.Path, m.w-8)
			style := m.th.Muted()
			switch {
			case f.X == '?':
				style = m.th.Muted()
			case f.Staged:
				style = m.th.Secondary()
			case f.Deleted:
				style = m.th.Tertiary()
			default:
				style = m.th.Accent()
			}
			if i == m.sel {
				b.WriteString(m.th.Accent().Render("▶ ") + style.Render(row) + "\n")
			} else {
				b.WriteString("  " + style.Render(row) + "\n")
			}
		}
		b.WriteString(m.th.Secondary().Render(strconv.Itoa(m.sel+1) + "/" + strconv.Itoa(len(m.files))))
	}
	return b.String()
}

// colorDiff tints +/- diff lines with the theme.
func colorDiff(diff string, th theme.Theme) string {
	lines := strings.Split(diff, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "+"):
			out = append(out, th.Secondary().Render(l))
		case strings.HasPrefix(l, "-"):
			out = append(out, th.Tertiary().Render(l))
		case strings.HasPrefix(l, "@@"):
			out = append(out, th.Muted().Render(l))
		default:
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
