// Package settings is the SETTINGS tab: live theme switching across every
// tab (zero restart) plus a read-only view of the resolved config.
package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/nvii/charmulet/internal/core/config"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
)

type Model struct {
	th      theme.Theme
	w, h    int
	themes  []theme.Theme
	sel     int
	cfg     *config.Config
	version string

	saved string // transient "theme saved" confirmation
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme, cfg *config.Config, themesDir, version string) *Model {
	return &Model{
		th:      th,
		themes:  theme.Available(themesDir),
		cfg:     cfg,
		version: version,
	}
}

func (m *Model) Title() string { return "SETTINGS" }
func (m *Model) ID() string    { return "settings" }
func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Activate() tea.Cmd {
	if m.cfg == nil {
		if cfg, err := config.Load(); err == nil {
			m.cfg = cfg
		} else {
			m.cfg = &config.Config{}
		}
	}
	return nil
}
func (m *Model) Deactivate() {}

// applyTheme fans the new theme out shell-wide and persists the choice.
func (m *Model) applyTheme(th theme.Theme) tea.Cmd {
	m.th = th
	m.saved = ""
	cfg := m.cfg
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.Theme = th.Name
	if err := config.Save(cfg); err != nil {
		m.saved = "live switch applied · save failed: " + err.Error()
	} else {
		m.saved = "theme saved · " + th.Name
	}
	return func() tea.Msg { return tabs.SetThemeMsg{Theme: th} }
}

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tabs.SetThemeMsg:
		m.th = msg.Theme
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "j", "down":
			if m.sel < len(m.themes)-1 {
				m.sel++
			}
		case "k", "up":
			if m.sel > 0 {
				m.sel--
			}
		case "enter":
			if len(m.themes) > 0 {
				return m, m.applyTheme(m.themes[m.sel])
			}
		case "r", "R":
			dir := "themes"
			if p, err := os.Getwd(); err == nil {
				dir = filepath.Join(p, "themes")
			}
			m.themes = theme.Available(dir)
			if m.sel >= len(m.themes) {
				m.sel = len(m.themes) - 1
			}
			if m.sel < 0 {
				m.sel = 0
			}
		}
	}
	return m, nil
}

func (m *Model) Resize(w, h int) { m.w, m.h = w, h }

func (m *Model) View() string {
	var b strings.Builder
	b.WriteString(m.th.Secondary().Render("theme") + "  " +
		m.th.Muted().Render("j/k move · enter apply live + save · r rescan") + "\n")

	for i, th := range m.themes {
		marker := "  "
		if th.Name == m.th.Name {
			marker = "● "
		}
		row := marker + th.Name
		if i == m.sel {
			b.WriteString(m.th.Accent().Render("▶ "+row) + "\n")
		} else {
			b.WriteString(m.th.Muted().Render(row) + "\n")
		}
	}

	if m.saved != "" {
		b.WriteString("\n" + m.th.Secondary().Render(m.saved) + "\n")
	}

	b.WriteString("\n" + m.th.Secondary().Render("resolved config") + "\n")
	cfg := m.cfg
	if cfg == nil {
		cfg = &config.Config{}
	}
	rows := [][2]string{
		{"ollama", cfg.OllamaURL},
		{"model", cfg.ChatModel},
		{"theme", cfg.Theme},
		{"version", m.version},
	}
	for _, r := range rows {
		v := r[1]
		if v == "" {
			v = "—"
		}
		b.WriteString("  " + m.th.Muted().Render(fmt.Sprintf("%-8s %s", r[0], v)) + "\n")
	}

	b.WriteString("\n" + m.th.Muted().Render("config lives in charmulet.toml · theme files in themes/"))
	return b.String()
}
