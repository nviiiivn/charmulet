// Package tab defines the CHARMULET tab contract. Every surface in the shell
// implements this interface; the shell routes messages to the active tab and
// keeps background tabs alive but idle.
package tabs

import tea "charm.land/bubbletea/v2"

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
