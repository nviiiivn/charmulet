// Package tab defines the CHARMULET tab contract. Every surface in the shell
// implements this interface; the shell routes messages to the active tab and
// keeps background tabs alive but idle.
package tabs

import tea "charm.land/bubbletea/v2"

import "github.com/nvii/charmulet/internal/tui/theme"

type Tab interface {
	// Title returns the label shown in the tab bar.
	Title() string
	// ID is a stable identifier used by keybinds and persistence.
	ID() string
	Init() tea.Cmd
	Update(msg tea.Msg) (Tab, tea.Cmd)
	View() string
	// Activate resumes polling/streams when the tab gains focus.
	Activate() tea.Cmd
	// Deactivate pauses work; state must be retained.
	Deactivate()
	Resize(w, h int)
}

// SpawnMsg asks the shell to append a new tab and focus it. Any tab can
// emit it (e.g. the launcher spawning a chat session).
type SpawnMsg struct{ Tab Tab }

// SetThemeMsg re-styles every tab live (zero restart). The shell fans it
// out to all tabs after updating its own chrome.
type SetThemeMsg struct{ Theme theme.Theme }
