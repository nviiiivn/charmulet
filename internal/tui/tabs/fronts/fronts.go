// Package fronts is the web-fronts tab: reachability + latency checks for
// every public front and notable local service. Read-only — GET only, never
// controls anything.
package fronts

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nvii/charmulet/internal/tui/tabs"
	"github.com/nvii/charmulet/internal/tui/theme"
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

type tickMsg struct{}
type resultsMsg struct{ results []result }

type Model struct {
	th      theme.Theme
	w, h    int
	fronts  []Front
	results []result
	sel     int
	last    time.Time
	loading bool
}

var _ tabs.Tab = (*Model)(nil)

func New(th theme.Theme) *Model {
	fs := append([]Front(nil), defaults...)
	sort.Slice(fs, func(i, j int) bool { return fs[i].Name < fs[j].Name })
	return &Model{th: th, fronts: fs}
}

func (m *Model) Title() string { return "FRONTS" }
func (m *Model) ID() string    { return "fronts" }

func (m *Model) Init() tea.Cmd { return checkCmd(m.fronts) }

func (m *Model) Activate() tea.Cmd {
	m.loading = true
	return checkCmd(m.fronts)
}

func (m *Model) Deactivate() {}

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

func checkCmd(fronts []Front) tea.Cmd {
	return func() tea.Msg {
		out := make([]result, len(fronts))
		var wg sync.WaitGroup
		for i, f := range fronts {
			wg.Add(1)
			go checkOne(f, &wg, out, i)
		}
		wg.Wait()
		return resultsMsg{results: out}
	}
}

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *Model) Update(msg tea.Msg) (tabs.Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tabs.SetThemeMsg:
		m.th = msg.Theme
		return m, nil

	case tickMsg:
		return m, checkCmd(m.fronts)
	case resultsMsg:
		m.loading = false
		m.results = msg.results
		m.last = time.Now()
		if m.sel >= len(m.results) {
			m.sel = len(m.results) - 1
		}
		return m, tick()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "r", "R":
			m.loading = true
			return m, checkCmd(m.fronts)
		case "j", "down":
			if m.sel < len(m.results)-1 {
				m.sel++
			}
		case "k", "up":
			if m.sel > 0 {
				m.sel--
			}
		}
	}
	return m, nil
}

func (m *Model) Resize(w, h int) { m.w, m.h = w, h }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

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
		pad("FRONT", nameW+1) + pad("URL", urlW+1) + pad("ST", 5) + "MS")}
	for i, r := range m.results {
		marker := m.th.Tertiary().Render("●")
		if r.Up {
			marker = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Render("●")
		}
		name := truncate(r.Name, nameW)
		url := truncate(r.URL, urlW)
		status := "—"
		if r.Up {
			status = fmt.Sprintf("%d", r.Status)
		}
		ms := fmt.Sprintf("%3.0f", float64(r.MS.Milliseconds()))
		line := pad(name, nameW+1) + pad(url, urlW+1) + pad(status, 5) + ms
		if i == m.sel {
			line = marker + " " + m.th.Accent().Render(line)
		} else {
			line = marker + " " + line
		}
		rows = append(rows, line)
	}

	sel := m.results[m.sel]
	detail := m.th.Muted().Render(truncate(sel.URL+"  "+sel.Err, max(m.w-2, 10)))

	return head + "  " + sum + "\n" + lipgloss.JoinVertical(lipgloss.Left, rows...) + "\n" + detail
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
