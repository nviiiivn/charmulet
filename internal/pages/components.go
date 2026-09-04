// Component registry: page widgets resolve here. Components are pure
// renderers — width, current time, and the homelab feed in, text out. The
// PAGES tab draws the boxes and colors.
package pages

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/nvii/charmulet/internal/homelab"
)

// Deps carries everything a component may render from.
type Deps struct {
	Width     int
	Now       time.Time
	Machines  map[string]homelab.Machine
	FrontsURL string // reserved for the fronts component
}

// render is a component implementation.
type render func(w Widget, d Deps) string

// Registry maps widget kinds to renderers. Unknown kinds render a hint.
var Registry = map[string]render{
	"system": renderSystem,
	"disk":   renderDisk,
	"ollama": renderOllama,
	"clock":  renderClock,
	"note":   renderNote,
}

// Kinds lists registered component types (for docs/editors).
func Kinds() []string {
	ks := make([]string, 0, len(Registry))
	for k := range Registry {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Render draws one widget, falling back to an unknown-kind hint.
func Render(w Widget, d Deps) string {
	if r, ok := Registry[w.Kind]; ok {
		return r(w, d)
	}
	return "unknown component " + quote(w.Kind)
}

func quote(s string) string { return "\"" + s + "\"" }

// pick returns the machine a widget targets (by key, else by display name,
// else the first in canonical order).
func pick(machine string, d Deps) (string, homelab.Machine, bool) {
	if m, ok := d.Machines[machine]; ok {
		return machine, m, true
	}
	for _, k := range homelab.Ordered(d.Machines) {
		m := d.Machines[k]
		if machine != "" && m.Display == machine {
			return k, m, true
		}
	}
	if machine == "" {
		for _, k := range homelab.Ordered(d.Machines) {
			return k, d.Machines[k], true
		}
	}
	return "", homelab.Machine{}, false
}

func renderSystem(w Widget, d Deps) string {
	key, m, ok := pick(w.Machine, d)
	if !ok {
		return "machine " + quote(w.Machine) + " not in feed"
	}
	sys := m.Widgets.System
	if sys == nil {
		return "no system data"
	}
	if sys.Error != "" {
		return "unreachable: " + sys.Error
	}

	var b strings.Builder
	b.WriteString(barRow("cpu", sys.CPUPercent, d.Width))
	b.WriteString("\n" + barRow("mem", sys.MemPercent, d.Width))
	if sys.MemTotal != "" {
		b.WriteString("  " + sys.MemUsed + "/" + sys.MemTotal)
	}
	b.WriteString("\n" + barRow("disk", sys.DiskPercent, d.Width))
	if sys.Disk != "" {
		b.WriteString("  " + sys.Disk)
	}
	if sys.DiskPercent >= 85 {
		b.WriteString(" !")
	}
	if sys.GPU != "" {
		parts := strings.Split(sys.GPU, ",")
		gpu := strings.TrimSpace(parts[0])
		if len(parts) >= 4 {
			gpu += " · " + strings.TrimSpace(parts[3])
		}
		if len(parts) >= 3 {
			gpu += " · " + strings.TrimSpace(parts[2])
		}
		b.WriteString("\ngpu  " + gpu)
	}
	if sys.Uptime != "" || sys.Temp != "" {
		b.WriteString("\nup   " + sys.Uptime + " · " + sys.Temp)
	}
	_ = key
	return b.String()
}

func renderDisk(w Widget, d Deps) string {
	_, m, ok := pick(w.Machine, d)
	if !ok {
		return "machine " + quote(w.Machine) + " not in feed"
	}
	sys := m.Widgets.System
	if sys == nil {
		return "no system data"
	}
	if sys.Error != "" {
		return "unreachable"
	}

	title := orDefault(w.Title, m.Display)
	var b strings.Builder
	b.WriteString(title + "\n")
	b.WriteString(barRow("disk", sys.DiskPercent, d.Width))
	if sys.Disk != "" {
		b.WriteString("  " + sys.Disk)
	}
	if sys.DiskPercent >= 85 {
		b.WriteString("  ! FULL")
	}
	b.WriteString("\n" + barRow("mem", sys.MemPercent, d.Width))
	return b.String()
}

func renderOllama(w Widget, d Deps) string {
	_, m, ok := pick(w.Machine, d)
	if !ok {
		return "machine " + quote(w.Machine) + " not in feed"
	}
	oll := m.Widgets.Ollama
	if oll == nil {
		return "no ollama on " + m.Display
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d models · %d running\n", len(oll.Models), len(oll.Running))
	for i, r := range oll.Running {
		if i >= 4 {
			b.WriteString("…\n")
			break
		}
		b.WriteString("♪ " + r + "\n")
	}
	s := b.String()
	return strings.TrimSuffix(s, "\n")
}

func renderClock(w Widget, d Deps) string {
	var b strings.Builder
	b.WriteString(d.Now.Format("15:04:05"))
	b.WriteString("\n" + d.Now.Format("Mon 02 Jan 2006"))
	return b.String()
}

func renderNote(w Widget, d Deps) string {
	return lipgloss.NewStyle().Width(d.Width).Render(strings.TrimSpace(w.Text))
}

// barRow renders "label ▕███░░▏ pct%" sized to width.
func barRow(label string, pct float64, width int) string {
	barW := width - len(label) - 12
	if barW < 4 {
		barW = 4
	}
	filled := int(pct / 100 * float64(barW))
	if filled > barW {
		filled = barW
	}
	return fmt.Sprintf("%s ▕%s▏ %3.0f%%", label,
		strings.Repeat("█", filled)+strings.Repeat("░", barW-filled), pct)
}

func orDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}
