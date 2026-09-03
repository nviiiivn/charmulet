// Package theme loads skin definitions (TOML) into lipgloss styles.
// Themes are pure data: swapping one re-styles the whole shell.
package theme

import (
	"fmt"
	"os"

	"image/color"

	"charm.land/lipgloss/v2"
	toml "github.com/pelletier/go-toml/v2"
)

type colors struct {
	Background    string `toml:"background"`
	Foreground    string `toml:"foreground"`
	Accent        string `toml:"accent"`
	Secondary     string `toml:"secondary"`
	Tertiary      string `toml:"tertiary"`
	Muted         string `toml:"muted"`
	TabActiveBG   string `toml:"tab_active_bg"`
	TabActiveFG   string `toml:"tab_active_fg"`
	TabInactiveBG string `toml:"tab_inactive_bg"`
	TabInactiveFG string `toml:"tab_inactive_fg"`
	StatusBG      string `toml:"status_bg"`
	StatusFG      string `toml:"status_fg"`
}

type Theme struct {
	Name   string `toml:"name"`
	Colors colors `toml:"colors"`
}

func Default() Theme {
	return Theme{
		Name: "pride",
		Colors: colors{
			Background: "#0d0b10", Foreground: "#f6eef7", Accent: "#ff4fa3",
			Secondary: "#b57bff", Tertiary: "#ff8a5c", Muted: "#8d7fa8",
			TabActiveBG: "#ff2e88", TabActiveFG: "#ffffff",
			TabInactiveBG: "#241631", TabInactiveFG: "#b9a6d6",
			StatusBG: "#3a1c52", StatusFG: "#f2d9e6",
		},
	}
}

// Load reads a TOML theme file. Missing fields fall back to defaults via
// zero-value merge on Colors.
func Load(path string) (Theme, error) {
	t := Default()
	raw, err := os.ReadFile(path)
	if err != nil {
		return t, fmt.Errorf("theme: %w", err)
	}
	if err := toml.Unmarshal(raw, &t); err != nil {
		return t, fmt.Errorf("theme: %w", err)
	}
	return t, nil
}

func (t Theme) base(fg, bg string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Background(lipgloss.Color(bg))
}

func (t Theme) TabActive() lipgloss.Style {
	return t.base(t.Colors.TabActiveFG, t.Colors.TabActiveBG).Bold(true)
}
func (t Theme) TabInactive() lipgloss.Style {
	return t.base(t.Colors.TabInactiveFG, t.Colors.TabInactiveBG)
}
func (t Theme) StatusBar() lipgloss.Style { return t.base(t.Colors.StatusFG, t.Colors.StatusBG) }
func (t Theme) Accent() lipgloss.Style    { return t.base("#ffffff", t.Colors.Accent) }
func (t Theme) Secondary() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Colors.Secondary))
}
func (t Theme) Tertiary() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Colors.Tertiary))
}
func (t Theme) Muted() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Colors.Muted))
}
func (t Theme) Body() lipgloss.Style { return t.base(t.Colors.Foreground, t.Colors.Background) }

// AccentColor exposes the raw accent color for style attributes that take a
// color.Color (borders, underlines) instead of a full Style.
func (t Theme) AccentColor() color.Color { return lipgloss.Color(t.Colors.Accent) }
