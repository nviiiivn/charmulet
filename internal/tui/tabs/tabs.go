// Package tab defines the CHARMULET tab contract. Every surface in the shell
// implements this interface; the shell routes messages to the owning tab
// (TabMsg) or the focused tab, and background tabs stay alive but idle.
package tabs

import tea "charm.land/bubbletea/v2"

import "github.com/nviiiivn/charmulet/internal/tui/theme"

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
	// Esc offers the tab the esc key: cancel a stream, close an overlay or
	// viewer. Returns true when consumed; the shell only quits on esc when
	// no tab consumes it.
	Esc() bool
}

// Typer is implemented by tabs that hold a text box. While Typing() is true
// the shell hands printable keys (digits, "?") to the tab as text instead of
// reading them as tab-jump / help shortcuts; tab, esc and ctrl keys still work.
type Typer interface {
	Typing() bool
}

// TabMsg is a message that belongs to one tab; the shell routes it to the
// owning tab regardless of focus, so background streams keep flowing.
type TabMsg interface {
	// TabID returns the ID() of the tab that owns this message.
	TabID() string
}

// SpawnMsg asks the shell to append a new tab and focus it. Any tab can
// emit it (e.g. the launcher spawning a chat session).
type SpawnMsg struct{ Tab Tab }

// SetThemeMsg re-styles every tab live (zero restart). The shell fans it
// out to all tabs after updating its own chrome.
type SetThemeMsg struct{ Theme theme.Theme }

// MoveSel applies j/k/up/down list navigation, clamped to [0, n-1].
// Returns the new selection and whether the key was a move key.
func MoveSel(sel, n int, msg tea.KeyPressMsg) (int, bool) {
	switch msg.String() {
	case "j", "down":
		if sel < n-1 {
			return sel + 1, true
		}
		return sel, true
	case "k", "up":
		if sel > 0 {
			return sel - 1, true
		}
		return sel, true
	}
	return sel, false
}

// Pad right-pads s with spaces to width n (no-op when already ≥ n).
func Pad(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}

// Trunc shortens s to n runes with a trailing ellipsis when it exceeds n.
func Trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
