// Package agents is the AGENTS tab: create/edit agent definitions and
// toggle lazyskills on the charmulet skills dir. All writes are local file
// ops (fast); no destructive actions — no delete, only create/overwrite/toggle.
package agents

import (
	"os"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	coreagents "github.com/nvii/charmulet/internal/core/agents"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
)

type mode int

const (
	modeList      mode = iota // pane browsing
	modeForm                  // agent editor form
	modeField                 // textarea overlay editing one agent field
	modeSkillBody             // textarea overlay editing a skill body
	modeSkillName             // single-line entry for a new skill name
)

type field struct {
	label     string
	multiline bool
	value     string
}

type Model struct {
	th   theme.Theme
	w, h int

	mode mode
	pane int // 0 agents · 1 skills

	agents     []coreagents.Agent
	skills     []coreagents.Skill
	selA, selS int

	form     []field
	selF     int
	origName string // agent file being edited ("" = new)

	newSkillName string

	skillIdx int // skill being body-edited

	ta     textarea.Model
	status string
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme) *Model {
	ta := textarea.New()
	ta.CharLimit = 0
	m := &Model{th: th, ta: ta}
	m.reload()
	return m
}

func (m *Model) reload() {
	if a, err := coreagents.List(); err == nil {
		m.agents = a
	} else {
		m.status = err.Error()
	}
	if s, err := coreagents.ListSkills(); err == nil {
		m.skills = s
	} else {
		m.status = err.Error()
	}
	if m.selA >= len(m.agents) {
		m.selA = len(m.agents) - 1
	}
	if m.selA < 0 {
		m.selA = 0
	}
	if m.selS >= len(m.skills) {
		m.selS = len(m.skills) - 1
	}
	if m.selS < 0 {
		m.selS = 0
	}
}

func (m *Model) Title() string { return "AGENTS" }
func (m *Model) ID() string    { return "agents" }
func (m *Model) Init() tea.Cmd { return nil }
func (m *Model) Activate() tea.Cmd {
	m.reload()
	return nil
}
func (m *Model) Deactivate() {}

func (m *Model) Resize(w, h int) {
	m.w, m.h = w, h
	ew := min(w-6, 90)
	m.ta.SetWidth(ew)
	m.ta.SetHeight(min(h-8, 18))
}

func (m *Model) openForm(a coreagents.Agent, isNew bool) {
	m.mode = modeForm
	m.selF = 0
	if isNew {
		m.origName = ""
	} else {
		m.origName = a.Name
	}
	m.form = []field{
		{label: "name", value: a.Name},
		{label: "model", value: a.Model},
		{label: "system_prompt", value: a.SystemPrompt, multiline: true},
		{label: "wrapper", value: a.Wrapper},
	}
}

func (m *Model) saveForm() {
	name := strings.TrimSpace(m.form[0].value)
	if name == "" {
		m.status = "agent needs a name"
		return
	}
	a := coreagents.Agent{
		Name:         name,
		Model:        strings.TrimSpace(m.form[1].value),
		SystemPrompt: m.form[2].value,
		Wrapper:      strings.TrimSpace(m.form[3].value),
	}
	if err := coreagents.Save(a); err != nil {
		m.status = err.Error()
		return
	}
	// Rename case: remove the old file so names stay unique. No delete for
	// same-name saves (it is an overwrite).
	if m.origName != "" && m.origName != name {
		if err := coreagents.Remove(m.origName); err != nil {
			m.status = "saved, but old file stuck: " + err.Error()
		}
	}
	m.status = "saved " + name
	m.mode = modeList
	m.reload()
}

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch m.mode {
		case modeList:
			return m.updateList(msg)
		case modeForm:
			return m.updateForm(msg)
		case modeField, modeSkillBody:
			return m.updateOverlay(msg)
		case modeSkillName:
			return m.updateSkillName(msg)
		}
	}
	return m, nil
}

func (m *Model) updateList(msg tea.KeyPressMsg) (tabs.Tab, tea.Cmd) {
	switch msg.String() {
	case "h", "left":
		if m.pane > 0 {
			m.pane = 0
		}
	case "l", "right":
		if m.pane < 1 {
			m.pane = 1
		}
	case "j", "down":
		if m.pane == 0 && m.selA < len(m.agents)-1 {
			m.selA++
		}
		if m.pane == 1 && m.selS < len(m.skills)-1 {
			m.selS++
		}
	case "k", "up":
		if m.pane == 0 && m.selA > 0 {
			m.selA--
		}
		if m.pane == 1 && m.selS > 0 {
			m.selS--
		}
	case "r", "R":
		m.reload()
		m.status = "refreshed"
	case "n", "N":
		if m.pane == 0 {
			m.openForm(coreagents.Agent{Model: "qwen3.5:9b"}, true)
		} else {
			m.mode, m.newSkillName = modeSkillName, ""
		}
	case " ":
		if m.pane == 1 && len(m.skills) > 0 {
			s := m.skills[m.selS]
			ns, err := coreagents.ToggleSkill(s)
			if err != nil {
				m.status = err.Error()
			} else {
				m.skills[m.selS] = ns
				verb := "disabled"
				if ns.Enabled {
					verb = "enabled"
				}
				m.status = verb + " " + ns.Name
			}
		}
	case "enter":
		if m.pane == 0 {
			if len(m.agents) > 0 {
				m.openForm(m.agents[m.selA], false)
			}
		} else if len(m.skills) > 0 {
			m.skillIdx = m.selS
			m.openOverlay(m.skills[m.selS].Name, m.readSkillBody(m.skills[m.selS]), modeSkillBody)
		}
	}
	return m, nil
}

func (m *Model) readSkillBody(s coreagents.Skill) string {
	return readFile(s.Path)
}

func (m *Model) updateForm(msg tea.KeyPressMsg) (tabs.Tab, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.selF < len(m.form)-1 {
			m.selF++
		}
	case "k", "up":
		if m.selF > 0 {
			m.selF--
		}
	case "enter":
		f := m.form[m.selF]
		m.openOverlay(f.label, f.value, modeField)
	case "ctrl+s", "s":
		m.saveForm()
	case "esc":
		m.mode = modeList
		m.status = "discarded"
	}
	return m, nil
}

func (m *Model) openOverlay(title, body string, md mode) {
	m.mode = md
	m.ta.Reset()
	m.ta.SetValue(body)
	m.status = "editing " + title + " · esc to commit"
}

func (m *Model) updateOverlay(msg tea.KeyPressMsg) (tabs.Tab, tea.Cmd) {
	if msg.String() == "esc" {
		body := m.ta.Value()
		switch m.mode {
		case modeField:
			m.form[m.selF].value = body
			m.mode = modeForm
		case modeSkillBody:
			s := m.skills[m.skillIdx]
			if err := writeFile(s.Path, body); err != nil {
				m.status = err.Error()
			} else {
				m.status = "saved " + s.Name
			}
			m.mode = modeList
			m.reload()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	return m, cmd
}

func (m *Model) updateSkillName(msg tea.KeyPressMsg) (tabs.Tab, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(m.newSkillName)
		if name == "" {
			m.status = "skill needs a name"
			m.mode = modeList
			return m, nil
		}
		if err := coreagents.WriteSkill(name, "# "+name+"\n\n"); err != nil {
			m.status = err.Error()
			m.mode = modeList
			return m, nil
		}
		m.reload()
		for i, s := range m.skills {
			if s.Name == name && s.Enabled {
				m.selS, m.skillIdx = i, i
			}
		}
		m.openOverlay(name, m.readSkillBody(m.skills[m.skillIdx]), modeSkillBody)
	case "backspace":
		if n := len(m.newSkillName); n > 0 {
			m.newSkillName = m.newSkillName[:n-1]
		}
	case "esc":
		m.mode = modeList
	default:
		if len(msg.Text) > 0 && msg.Mod == 0 {
			m.newSkillName += msg.Text
		}
	}
	return m, nil
}

func (m *Model) View() string {
	var b strings.Builder
	switch m.mode {
	case modeList:
		b.WriteString(m.headerLine())
		if m.pane == 0 {
			b.WriteString(m.agentsView())
		} else {
			b.WriteString(m.skillsView())
		}
		if m.status != "" {
			b.WriteString("\n" + m.th.Secondary().Render(m.status))
		}
	case modeForm:
		b.WriteString(m.th.Secondary().Render("agent editor") + "  " +
			m.th.Muted().Render("j/k field · enter edit · s save · esc discard") + "\n\n")
		for i, f := range m.form {
			marker := "  "
			style := m.th.Muted()
			if i == m.selF {
				marker, style = "▶ ", m.th.Accent()
			}
			val := f.value
			if !f.multiline {
				if val == "" {
					val = "·"
				}
			} else {
				val = oneLine(val)
			}
			b.WriteString(style.Render(marker+f.label) + "  " + m.th.Body().Render(val) + "\n")
		}
	case modeField, modeSkillBody:
		b.WriteString(m.renderOverlay())
	case modeSkillName:
		b.WriteString(m.th.Secondary().Render("new skill name") + "  " +
			m.th.Accent().Render(m.newSkillName+"▏") + "\n" +
			m.th.Muted().Render("enter continue · esc cancel"))
	}
	return b.String()
}

func (m *Model) headerLine() string {
	left, right := "AGENTS", "SKILLS"
	var l, r string
	if m.pane == 0 {
		l, r = m.th.Accent().Render("▶ "+left), m.th.Muted().Render("  "+right)
	} else {
		l, r = m.th.Muted().Render("  "+left), m.th.Accent().Render("▶ "+right)
	}
	return l + " " + r + "  " + m.th.Muted().Render("h/l pane · j/k move · enter edit · n new · space toggle (skills) · r refresh") + "\n\n"
}

func (m *Model) agentsView() string {
	if len(m.agents) == 0 {
		return m.th.Muted().Render("no agents — press n to create one")
	}
	var b strings.Builder
	b.WriteString(m.th.Accent().Render(pad("NAME", 16)+pad("MODEL", 22)) + "PROMPT\n")
	for i, a := range m.agents {
		row := pad(trunc(a.Name, 15), 16) + pad(trunc(a.Model, 21), 22) + trunc(oneLine(a.SystemPrompt), 44)
		if i == m.selA {
			b.WriteString(m.th.Accent().Render("▶ "+row) + "\n")
		} else {
			b.WriteString(m.th.Muted().Render("  "+row) + "\n")
		}
	}
	return b.String()
}

func (m *Model) skillsView() string {
	if len(m.skills) == 0 {
		return m.th.Muted().Render("no skills — press n to add one (~/.config/charmulet/skills)")
	}
	var b strings.Builder
	b.WriteString(m.th.Accent().Render(pad("SKILL", 20)+pad("STATE", 10)) + "DESC\n")
	for i, s := range m.skills {
		st := m.th.Muted().Render(pad("off", 10))
		if s.Enabled {
			st = m.th.Secondary().Render(pad("on", 10))
		}
		row := pad(trunc(s.Name, 19), 20) + st + trunc(s.Desc, 48)
		if i == m.selS {
			b.WriteString(m.th.Accent().Render("▶ "+row) + "\n")
		} else {
			b.WriteString(m.th.Muted().Render("  "+row) + "\n")
		}
	}
	return b.String()
}

func (m *Model) renderOverlay() string {
	var title string
	if m.mode == modeField {
		title = m.form[m.selF].label
	} else {
		title = m.skills[m.skillIdx].Name
	}
	return m.th.Secondary().Render("editing "+title) + "  " +
		m.th.Muted().Render("esc commit") + "\n" + m.ta.View()
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ⏎ ")
	if len(s) > 60 {
		s = s[:59] + "…"
	}
	if s == "" {
		s = "·"
	}
	return s
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

func writeFile(p, body string) error {
	return os.WriteFile(p, []byte(body), 0o644)
}
