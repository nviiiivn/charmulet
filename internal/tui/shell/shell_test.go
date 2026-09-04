package shell

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
)

// fake is a recording tab: every lifecycle call appends to trace.
type fake struct {
	id      string
	esc     bool // value Esc() returns
	trace   []string
	gotMsgs []tea.Msg
}

func (f *fake) Title() string     { return f.id }
func (f *fake) ID() string        { return f.id }
func (f *fake) Init() tea.Cmd     { f.trace = append(f.trace, "init"); return nil }
func (f *fake) Activate() tea.Cmd { f.trace = append(f.trace, "activate"); return nil }
func (f *fake) Deactivate()       { f.trace = append(f.trace, "deactivate") }
func (f *fake) Resize(w, h int)   { f.trace = append(f.trace, "resize") }
func (f *fake) Esc() bool         { f.trace = append(f.trace, "esc"); return f.esc }
func (f *fake) View() string      { return f.id }
func (f *fake) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	f.gotMsgs = append(f.gotMsgs, msg)
	return f, nil
}

func newShell(fs ...*fake) (*Model, []*fake) {
	var ts []tabs.Tab
	for _, f := range fs {
		ts = append(ts, f)
	}
	m := New(theme.Default(), ts...)
	m.splash = false // tests target the main screen
	m.w, m.h = 80, 24
	return m, fs
}

func key(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: mod}
}

func digit(d string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: rune(d[0]), Text: d}
}

// owned is a TabMsg bound to one tab id, like chat token messages.
type owned struct{ owner string }

func (o owned) TabID() string  { return o.owner }
func (o owned) String() string { return "owned" }

func TestSpawnAppendsFocusesActivates(t *testing.T) {
	m, fs := newShell(&fake{id: "a"})
	b := &fake{id: "b"}

	m.Update(tabs.SpawnMsg{Tab: b})

	if len(m.tabs) != 2 || m.active != 1 {
		t.Fatalf("want 2 tabs active=1, got %d active=%d", len(m.tabs), m.active)
	}
	if m.tabs[1] != tabs.Tab(b) {
		t.Fatal("spawned tab not appended")
	}
	if got := fs[0].trace; got[len(got)-1] != "deactivate" {
		t.Fatalf("previous tab not deactivated: %v", got)
	}
	if !contains(b.trace, "init") || !contains(b.trace, "activate") || !contains(b.trace, "resize") {
		t.Fatalf("spawned tab lifecycle missing: %v", b.trace)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestCtrlWCoreGuardAndClose(t *testing.T) {
	m, fs := newShell(&fake{id: "a"}, &fake{id: "b"})
	m.active = 1

	// all initial tabs are core: close refused
	m.Update(key('w', tea.ModCtrl))
	if len(m.tabs) != 2 || m.status != "core tabs can't be closed" {
		t.Fatalf("core close not refused: %d tabs, status %q", len(m.tabs), m.status)
	}

	// spawned tab is closable
	c := &fake{id: "c"}
	m.Update(tabs.SpawnMsg{Tab: c})
	m.Update(key('w', tea.ModCtrl))
	if len(m.tabs) != 2 || m.active != 1 {
		t.Fatalf("spawned tab not closed: %d tabs active=%d", len(m.tabs), m.active)
	}
	if fs[1] != m.Active() {
		t.Fatal("focus did not fall back to previous tab")
	}
	if !contains(c.trace, "deactivate") {
		t.Fatalf("closed tab not deactivated: %v", c.trace)
	}
}

func TestDigitJump(t *testing.T) {
	m, fs := newShell(&fake{id: "a"}, &fake{id: "b"}, &fake{id: "c"})

	m.Update(digit("3"))
	if m.active != 2 {
		t.Fatalf("want active=2, got %d", m.active)
	}
	if !contains(fs[0].trace, "deactivate") || !contains(fs[2].trace, "activate") {
		t.Fatalf("jump lifecycle wrong: %v / %v", fs[0].trace, fs[2].trace)
	}

	// digit beyond tab count is a no-op
	m.Update(digit("9"))
	if m.active != 2 {
		t.Fatalf("out-of-range digit moved focus: %d", m.active)
	}
}

func TestTabMsgRoutesToOwnerNotFocus(t *testing.T) {
	m, fs := newShell(&fake{id: "a"}, &fake{id: "b"})
	m.active = 0 // focus a, message owned by b

	m.Update(owned{owner: "b"})

	if len(fs[0].gotMsgs) != 0 {
		t.Fatalf("focused tab should not receive foreign msg: %v", fs[0].gotMsgs)
	}
	if len(fs[1].gotMsgs) != 1 {
		t.Fatalf("owner tab missing msg: %v", fs[1].gotMsgs)
	}

	// message for a closed tab drops silently
	m2, _ := newShell(&fake{id: "a"})
	m2.Update(owned{owner: "ghost"})
	if len(m2.tabs) != 1 {
		t.Fatal("ghost owner must not mutate tabs")
	}
}

func TestEscConsumeThenQuit(t *testing.T) {
	m, fs := newShell(&fake{id: "a", esc: true})

	// tab consumes esc: no quit
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Fatal("consumed esc must not return a cmd")
	}
	if !contains(fs[0].trace, "esc") {
		t.Fatal("tab never offered esc")
	}

	// tab declines esc: shell quits
	fs[0].esc = false
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("declined esc must quit")
	}
}

func TestSplashSwallowsFirstKey(t *testing.T) {
	m, fs := newShell(&fake{id: "a"})
	m.splash = true

	// first key dismisses the splash and must not reach tabs or quit
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.splash || cmd != nil {
		t.Fatalf("splash not dismissed cleanly: splash=%v cmd=%v", m.splash, cmd)
	}
	if len(fs[0].gotMsgs) != 0 || contains(fs[0].trace, "esc") {
		t.Fatalf("splash dismissal leaked to tab: %v %v", fs[0].gotMsgs, fs[0].trace)
	}

	// next esc follows the normal path (quit when declined)
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc after splash should quit")
	}
}
