// Package fronts is the web-fronts tab: reachability + latency checks for
// every public front and notable local service. Read-only — GET only, never
// controls anything.
package fronts

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"fmt"
	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
	"net/http"
	"sort"
	"sync"
	"time"
)

const refreshEvery = 10 * time.Second

// Front is one URL to watch. Any HTTP response counts as reachable; only
// network errors (timeout, refused, dns) read as down.
type Front struct {
	Name string
	URL  string
}

var defaults = []Front{
	// public fronts (caddy)
	{"iamprettyfamous.online", "https://iamprettyfamous.online"},
	{"umb.alola.lol", "https://umb.alola.lol"},
	{"umb wiki", "https://umb.alola.lol/wiki"},
	{"git.alola.lol", "https://git.alola.lol"},
	{"comfyui.alola.lol", "https://comfyui.alola.lol"},
	{"auth.alola.lol", "https://auth.alola.lol"},
	// demo-server locals
	{"dashbored", "http://localhost:6900"},
	{"gitea", "http://localhost:3100"},
	{"glance", "http://localhost:6908"},
	{"searxng", "http://localhost:8888"},
	{"valhalla", "http://localhost:8002"},
	{"tradesight", "http://localhost:8915"},
	{"mkt-dash", "http://localhost:8502"},
	{"openclaw gw", "http://localhost:18789"},
	// demo-desktop
	{"comfy direct", "http://192.0.2.24:8188"},
	{"gallery", "http://192.0.2.24:8189"},
	{"umb direct", "http://192.0.2.24:8910"},
	{"sparkbook", "http://192.0.2.24:8911"},
	{"ollama api", "http://192.0.2.24:11434"},
}

type result struct {
	Name   string
	URL    string
	Up     bool
	Status int
	MS     time.Duration
	Err    string
}

// tickMsg/resultsMsg carry the poll-chain generation; stale msgs are dropped.
type tickMsg struct{ gen int }
type resultsMsg struct {
	results []result
	gen     int
}
type Model struct {
	th      theme.Theme
	w, h    int
	fronts  []Front
	results []result
	sel     int
	last    time.Time
	loading bool
	gen     int // poll-chain generation; bumps on every new arm
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme) *Model {
	fs := append([]Front(nil), defaults...)
	sort.Slice(fs, func(i, j int) bool { return fs[i].Name < fs[j].Name })
	return &Model{th: th, fronts: fs}
}
func (m *Model) Title() string { return "FRONTS" }
func (m *Model) ID() string    { return "fronts" }
func (m *Model) Init() tea.Cmd { return checkCmd(m.fronts, m.gen) }
func (m *Model) Activate() tea.Cmd {
	m.loading = true
	m.gen++
	return checkCmd(m.fronts, m.gen)
}
func (m *Model) Deactivate() {}

// Esc has no in-tab meaning here; the shell may quit.
func (m *Model) Esc() bool { return false }

func checkOne(f Front, wg *sync.WaitGroup, out []result, i int) {
	defer wg.Done()
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't follow; any response = reachable
		},
	}
	start := time.Now()
	resp, err := client.Get(f.URL)
	ms := time.Since(start)
	r := result{Name: f.Name, URL: f.URL, MS: ms}
	if err != nil {
		r.Err = err.Error()
		out[i] = r
		return
	}
	defer resp.Body.Close()
	r.Up = true
	r.Status = resp.StatusCode
	out[i] = r
}
func checkCmd(fronts []Front, gen int) tea.Cmd {
	return func() tea.Msg {
		out := make([]result, len(fronts))
		var wg sync.WaitGroup
		for i, f := range fronts {
			wg.Add(1)
			go checkOne(f, &wg, out, i)
		}
		wg.Wait()
		return resultsMsg{results: out, gen: gen}
	}
}
func tick(gen int) tea.Cmd {
	return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{gen: gen} })
}
func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tabs.SetThemeMsg:
		m.th = msg.Theme
		return m, nil
	case tickMsg:
		if msg.gen != m.gen {
			return m, nil // stale chain; let it die
		}
		return m, checkCmd(m.fronts, m.gen)
	case resultsMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.loading = false
		m.results = msg.results
		m.last = time.Now()
		if m.sel >= len(m.results) {
			m.sel = len(m.results) - 1
		}
		return m, tick(msg.gen)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "r", "R":
			m.loading = true
			m.gen++
			return m, checkCmd(m.fronts, m.gen)
		}
		if n, ok := tabs.MoveSel(m.sel, len(m.results), msg); ok {
			m.sel = n
		}
	}
	return m, nil
}
func (m *Model) Resize(w, h int) { m.w, m.h = w, h }
func (m *Model) View() string {
	stamp := "loading…"
	if !m.last.IsZero() {
		stamp = m.last.Format("15:04:05")
	}
	if m.loading {
		stamp += " · refreshing"
	}
	head := m.th.Secondary().Render("fronts") + m.th.Muted().Render(" · "+stamp+" · any HTTP response = up")
	if m.results == nil {
		return head + "\n" + m.th.Muted().Render("checking…")
	}
	up := 0
	for _, r := range m.results {
		if r.Up {
			up++
		}
	}
	sum := fmt.Sprintf("%d/%d up", up, len(m.results))
	if up == len(m.results) {
		sum = m.th.Secondary().Render(sum)
	} else {
		sum = m.th.Tertiary().Render(sum)
	}
	nameW, urlW := 22, 42
	rows := []string{m.th.Muted().Render(
		tabs.Pad("FRONT", nameW+1) + tabs.Pad("URL", urlW+1) + tabs.Pad("ST", 5) + "MS")}
	for i, r := range m.results {
		marker := m.th.Tertiary().Render("●")
		if r.Up {
			marker = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Render("●")
		}
		name := tabs.Trunc(r.Name, nameW)
		url := tabs.Trunc(r.URL, urlW)
		status := "—"
		if r.Up {
			status = fmt.Sprintf("%d", r.Status)
		}
		ms := fmt.Sprintf("%3.0f", float64(r.MS.Milliseconds()))
		line := tabs.Pad(name, nameW+1) + tabs.Pad(url, urlW+1) + tabs.Pad(status, 5) + ms
		if i == m.sel {
			line = marker + " " + m.th.Accent().Render(line)
		} else {
			line = marker + " " + line
		}
		rows = append(rows, line)
	}
	sel := m.results[m.sel]
	detail := m.th.Muted().Render(tabs.Trunc(sel.URL+"  "+sel.Err, max(m.w-2, 10)))
	return head + "  " + sum + "\n" + lipgloss.JoinVertical(lipgloss.Left, rows...) + "\n" + detail
}
