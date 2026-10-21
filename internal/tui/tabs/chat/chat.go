// Package chat is the main communication window: real streaming conversation
// with the configured provider, esc cancels in-flight generations.
package chat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nviiiivn/charmulet/internal/core/provider"
	"github.com/nviiiivn/charmulet/internal/core/session"
	"github.com/nviiiivn/charmulet/internal/tui/tabs"
	"github.com/nviiiivn/charmulet/internal/tui/theme"
)

// flushInterval coalesces streamed tokens into one repaint per tick so a
// long transcript is not re-wrapped on every chunk.
const flushInterval = 30 * time.Millisecond

var sessionSeq atomic.Int64

// nextID hands out unique tab IDs ("chat", "chat-2", "chat-3", …) so
// parallel sessions each own their stream messages.
func nextID() string {
	n := sessionSeq.Add(1)
	if n == 1 {
		return "chat"
	}
	return "chat-" + strconv.FormatInt(n, 10)
}

type errMsg struct {
	id  string
	err error
}

func (m errMsg) TabID() string { return m.id }

type tokenMsg struct {
	id   string
	text string
}

func (m tokenMsg) TabID() string { return m.id }

type streamDoneMsg struct {
	id  string
	err error
}

func (m streamDoneMsg) TabID() string { return m.id }

// flushMsg triggers the deferred repaint that batches streamed tokens.
type flushMsg struct{ id string }

func (m flushMsg) TabID() string { return m.id }

func flushCmd(id string) tea.Cmd {
	return tea.Tick(flushInterval, func(time.Time) tea.Msg { return flushMsg{id: id} })
}

type Model struct {
	id   string
	ta   textarea.Model
	vp   viewport.Model
	log  []string
	w, h int
	th   theme.Theme

	prov  provider.Provider
	model string
	hist  []provider.Message

	streaming bool
	cancel    context.CancelFunc
	cur       provider.Stream
	buf       strings.Builder
	pending   string // user text of the in-flight turn
	flushing  bool   // a flush tick is armed; skip repaints until it lands

	title string // tab-bar label; "CHAT" for the default session

	renaming  bool // 'i' mode: typing edits the title, enter commits
	renameBuf string

	st  *session.Store // nil = ephemeral, nothing persisted
	sid int64          // 0 = not backed by a store row
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme, prov provider.Provider, model string) *Model {
	return newModel(th, prov, model, "CHAT")
}

// NewSession creates a spawned chat tab with a model-derived title
// (used by the launcher for parallel sessions). A non-nil store backs it
// with a persisted session row; on store failure the title is marked '*'
// so the ephemeral tab is visibly unpersisted.
func NewSession(th theme.Theme, prov provider.Provider, model string, st *session.Store) *Model {
	title := sessionTitle(model)
	m := newModel(th, prov, model, title)
	if st != nil {
		if meta, err := st.Create(m.title, model); err == nil {
			m.st, m.sid = st, meta.ID
		} else {
			m.title = title + "*"
		}
	}
	return m
}

// NewResume reopens a persisted session with its transcript preloaded;
// further turns keep appending to the same row.
func NewResume(th theme.Theme, prov provider.Provider, st *session.Store, meta session.SessionMeta, msgs []session.Message) *Model {
	m := newModel(th, prov, meta.Model, meta.Title)
	if st != nil && meta.ID != 0 {
		m.st, m.sid = st, meta.ID
	}
	for _, msg := range msgs {
		m.hist = append(m.hist, provider.Message{Role: msg.Role, Content: msg.Content})
		switch msg.Role {
		case "user":
			m.log = append(m.log, th.TabActive().Render(" you ")+" "+msg.Content)
		case "assistant":
			m.log = append(m.log, th.Accent().Render(" charm ")+" "+msg.Content, "")
		}
	}
	if len(m.log) > 0 && m.log[len(m.log)-1] == "" {
		m.log = m.log[:len(m.log)-1]
	}
	return m
}

func newModel(th theme.Theme, prov provider.Provider, model, title string) *Model {
	m := &Model{
		id:    nextID(),
		th:    th,
		prov:  prov,
		model: model,
		title: title,
	}
	m.ta = textarea.New()
	m.ta.Placeholder = "say something… (enter to send, esc to cancel)"
	m.ta.Focus() // an unfocused bubbles textarea drops every key
	if prov != nil {
		m.log = []string{m.th.Secondary().Render("charmulet chat — " + prov.Name() + " · " + model), ""}
	} else {
		m.log = []string{m.th.Secondary().Render("charmulet chat — no provider configured"), ""}
	}
	return m
}

// sessionTitle shortens "hf.co/NousResearch/Hermes-4.3-36B-GGUF:q4_K_M"
// to "HERMES-4.3-36B-G…" style labels for the tab bar.
func sessionTitle(model string) string {
	base := model
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.Index(base, ":"); i > 0 {
		base = base[:i]
	}
	base = strings.ToUpper(base)
	if len(base) > 14 {
		base = base[:13] + "…"
	}
	return base
}

func (m *Model) Title() string { return m.title }
func (m *Model) ID() string    { return m.id }

func (m *Model) Init() tea.Cmd { return textarea.Blink }

func (m *Model) Activate() tea.Cmd { return nil }
func (m *Model) Deactivate()       {}

// Esc cancels an in-flight generation; consumed only while streaming.
func (m *Model) Esc() bool {
	if m.renaming {
		m.renaming = false
		return true
	}
	if m.streaming && m.cancel != nil {
		m.cancel()
		return true
	}
	// a half-typed draft is precious — esc must not do anything destructive
	if m.ta.Value() != "" {
		return true
	}
	return false
}

func (m *Model) Resize(w, h int) {
	m.w, m.h = w, h
	inputH := 3
	m.vp.SetWidth(w)
	m.vp.SetHeight(h - inputH - 3)
	m.ta.SetWidth(w)
	m.ta.SetHeight(inputH)
}

// pump reads the next chunk off the live stream as a self-rescheduling cmd.
func pump(id string, s provider.Stream) tea.Cmd {
	return func() tea.Msg {
		chunk, ok, err := s()
		switch {
		case err != nil:
			return streamDoneMsg{id: id, err: err}
		case !ok:
			return streamDoneMsg{id: id}
		default:
			return tokenMsg{id: id, text: chunk}
		}
	}
}

// appendCmd persists a turn off the event loop (nil store no-ops).
func (m *Model) appendCmd(role, content string) tea.Cmd {
	st, sid := m.st, m.sid
	return func() tea.Msg {
		_ = st.Append(sid, role, content)
		return nil
	}
}

// exportedMsg flashes the export result.
type exportedMsg struct {
	id   string
	path string
	err  error
}

func (m exportedMsg) TabID() string { return m.id }

// exportCmd writes the transcript (hist) to a markdown file off-loop.
func (m *Model) exportCmd() tea.Cmd {
	id := m.id
	title, hist := m.title, append([]provider.Message(nil), m.hist...)
	return func() tea.Msg {
		path, err := exportMarkdown(title, hist)
		return exportedMsg{id: id, path: path, err: err}
	}
}

// updateRename handles keys while the title editor is open.
func (m *Model) updateRename(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		if t := strings.TrimSpace(m.renameBuf); t != "" {
			m.title = t
			if m.st != nil {
				st, sid := m.st, m.sid
				_ = st.Rename(sid, t)
			}
		}
		m.renaming = false
	case "backspace":
		if r := []rune(m.renameBuf); len(r) > 0 {
			m.renameBuf = string(r[:len(r)-1])
		}
	default:
		if msg.Mod == 0 && msg.Text != "" {
			m.renameBuf += msg.Text
		}
	}
	return nil
}

func (m *Model) start(userText string) tea.Cmd {
	msgs := make([]provider.Message, 0, len(m.hist)+1)
	msgs = append(msgs, m.hist...)
	msgs = append(msgs, provider.Message{Role: "user", Content: userText})

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.streaming = true
	m.buf.Reset()
	m.pending = userText
	persist := m.appendCmd("user", userText)

	s, err := m.prov.Complete(ctx, m.model, msgs)
	if err != nil {
		cancel()
		m.streaming = false
		return tea.Batch(persist, func() tea.Msg { return errMsg{id: m.id, err: err} })
	}
	m.cur = s
	return tea.Batch(persist, pump(m.id, s))
}

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	var cmds []tea.Cmd
	dirty := false    // transcript state changed → needs a re-render
	consumed := false // key fully handled; keep it out of the textarea

	switch msg := msg.(type) {
	case tabs.SetThemeMsg:
		m.th = msg.Theme
		m.repaint() // restyle transcript immediately
		return m, nil

	case tea.KeyPressMsg:
		if m.renaming {
			return m, m.updateRename(msg)
		}
		switch msg.String() {
		case "enter":
			consumed = true // enter sends; it never adds a line to the box
			val := strings.TrimSpace(m.ta.Value())
			if val != "" && !m.streaming && m.prov != nil {
				m.log = append(m.log,
					m.th.TabActive().Render(" you ")+" "+val,
				)
				m.ta.Reset()
				cmds = append(cmds, m.start(val))
				dirty = true
			}
		case "ctrl+r":
			if !m.streaming {
				m.renaming = true
				m.renameBuf = m.title
			}
		case "ctrl+e":
			cmds = append(cmds, m.exportCmd())
		}
		// Plain typing only moves the textarea cursor — View() renders
		// it live, so no transcript repaint needed (perf: O(transcript)
		// re-wrap per keystroke avoided).

	case tokenMsg:
		if msg.id != m.id {
			return m, nil // another session's stream
		}
		m.buf.WriteString(msg.text)
		cmds = append(cmds, pump(m.id, m.cur))
		if !m.flushing {
			m.flushing = true
			cmds = append(cmds, flushCmd(m.id))
		}
		// Deferred repaint: the flush tick below materializes the buffer,
		// so a burst of tokens costs one re-wrap, not one per chunk.
		return m, tea.Batch(cmds...)

	case flushMsg:
		if msg.id != m.id {
			return m, nil
		}
		m.flushing = false
		dirty = true // materialize the streamed tail

	case streamDoneMsg:
		if msg.id != m.id {
			return m, nil
		}
		text := strings.TrimSpace(m.buf.String())
		if text != "" {
			m.log = append(m.log,
				m.th.Accent().Render(" charm ")+" "+text, "",
			)
		} else if msg.err != nil {
			m.log = append(m.log, m.th.TabActive().Render(" err ")+fmtErr(msg.err), "")
		}
		m.hist = append(m.hist, provider.Message{Role: "user", Content: m.pending})
		if text != "" {
			m.hist = append(m.hist, provider.Message{Role: "assistant", Content: text})
			cmds = append(cmds, m.appendCmd("assistant", text))
		}
		m.pending = ""
		m.streaming = false
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		dirty = true

	case errMsg:
		if msg.id != m.id {
			return m, nil
		}
		m.log = append(m.log, m.th.TabActive().Render(" err ")+msg.err.Error(), "")
		m.streaming = false
		dirty = true

	case exportedMsg:
		if msg.id != m.id {
			return m, nil
		}
		if msg.err != nil {
			m.log = append(m.log, m.th.TabActive().Render(" err ")+"export: "+msg.err.Error(), "")
		} else {
			m.log = append(m.log, m.th.Secondary().Render(" saved ")+"→ "+msg.path, "")
		}
		dirty = true
	}

	if !consumed {
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		cmds = append(cmds, cmd)
	}

	if dirty {
		m.repaint()
	}
	return m, tea.Batch(cmds...)
}

// repaint pushes the rendered transcript into the viewport.
func (m *Model) repaint() {
	content := lipgloss.NewStyle().Width(m.w).Render(strings.Join(m.renderLog(), "\n"))
	m.vp.SetContent(content)
	m.vp.GotoBottom()
}

// renderLog materializes log lines plus any in-flight buffer tail.
func (m *Model) renderLog() []string {
	out := make([]string, len(m.log))
	copy(out, m.log)
	if m.streaming && m.buf.Len() > 0 {
		out = append(out, m.th.Accent().Render(" charm ")+" "+m.buf.String()+"▌")
	}
	return out
}

func fmtErr(err error) string { return "⚠ " + err.Error() }

// exportMarkdown writes the transcript to
// ~/.local/share/charmulet/exports/<title>-<timestamp>.md and returns the
// path written.
func exportMarkdown(title string, hist []provider.Message) (string, error) {
	dir := os.Getenv("CHARMULET_EXPORT_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "share", "charmulet", "exports")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := sanitize(title) + "-" + time.Now().Format("20060102-150405") + ".md"
	path := filepath.Join(dir, name)

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	for _, msg := range hist {
		switch msg.Role {
		case "user":
			fmt.Fprintf(&b, "## you\n\n%s\n\n", msg.Content)
		case "assistant":
			fmt.Fprintf(&b, "## charm\n\n%s\n\n", msg.Content)
		}
	}
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

// sanitize strips path-unsafe runes from a filename seed.
func sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', ' ':
			return '-'
		}
		return r
	}, s)
	if s == "" {
		s = "session"
	}
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

func (m *Model) View() string {
	input := m.ta.View()
	if pad := m.w - lipgloss.Width(input); pad > 0 {
		lines := strings.Split(input, "\n")
		last := len(lines) - 1
		lines[last] += strings.Repeat(" ", pad)
		input = strings.Join(lines, "\n")
	}
	if m.renaming {
		bar := m.th.Accent().Render(" title: ") + m.renameBuf + "▮" +
			m.th.Muted().Render("  enter save · esc cancel")
		if pad := m.w - lipgloss.Width(bar); pad > 0 {
			bar += strings.Repeat(" ", pad)
		}
		return m.vp.View() + "\n" + bar + "\n" + input
	}
	hint := m.th.Muted().Render(" enter send · esc cancel · ctrl+r rename · ctrl+e export")
	if pad := m.w - lipgloss.Width(hint); pad > 0 {
		hint += strings.Repeat(" ", pad)
	}
	return m.vp.View() + "\n" + input + "\n" + hint
}

// Typing is true while there is text in the box or the title editor is open,
// so digits and "?" land in the message. With an empty box, 1-9 still jump
// tabs and "?" still opens help.
func (m *Model) Typing() bool {
	return m.renaming || strings.TrimSpace(m.ta.Value()) != ""
}
