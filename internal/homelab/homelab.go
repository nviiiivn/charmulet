// Package homelab is a read-only client for the dashbored feed
// (http://localhost:6900). CHARMULET never re-scrapes machines itself;
// dashbored is the single source of truth for homelab state.
package homelab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

// System is the "system" widget. Error != "" means the host was
// unreachable and every other field is empty.
type System struct {
	Error       string  `json:"error"`
	CPU         string  `json:"cpu"`
	CPUPercent  float64 `json:"cpu_percent"`
	Disk        string  `json:"disk"`
	DiskPercent float64 `json:"disk_percent"`
	GPU         string  `json:"gpu"`
	GPUPercent  float64 `json:"gpu_percent"`
	MemUsed     string  `json:"mem_used"`
	MemTotal    string  `json:"mem_total"`
	MemPercent  float64 `json:"memory_percent"`
	Temp        string  `json:"temp"`
	Uptime      string  `json:"uptime"`
	Hostname    string  `json:"hostname"`
}

// Service is one entry of the "services" widget (systemd/ports as dashbored
// sees them — display only, no control from here).
type Service struct {
	Port    string `json:"port"`
	Name    string `json:"name"`
	Process string `json:"process"`
	PID     string `json:"pid"`
	Active  bool   `json:"active"`
}

type servicesWidget struct {
	Services []Service `json:"services"`
}

// OllamaModel is one installed model on the ollama host.
type OllamaModel struct {
	Name     string `json:"name"`
	Size     string `json:"size"`
	Modified string `json:"modified"`
}

type ollamaWidget struct {
	Models  []OllamaModel `json:"models"`
	Running []string      `json:"running"`
}

type mediaWidget struct {
	Status     string `json:"status"`
	Source     string `json:"source"`
	NowPlaying string `json:"now_playing"`
}

// Widgets holds the widget set we render. Unknown widgets are ignored by
// json decoding.
type Widgets struct {
	System   *System         `json:"system"`
	Services *servicesWidget `json:"services"`
	Ollama   *ollamaWidget   `json:"ollama"`
	Media    *mediaWidget    `json:"media"`
}

// Machine is one host in the dashbored inventory.
type Machine struct {
	Display string  `json:"display"`
	Color   string  `json:"color"`
	Widgets Widgets `json:"widgets"`
}

// Order is the canonical machine order (mirrors dashbored-config.json).
var Order = []string{"demo-desktop", "demo-server", "demo-laptop", "demo-mini", "demo-tablet"}

// Ordered returns machine keys in canonical order, unknown machines sorted
// at the end.
func Ordered(machines map[string]Machine) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(machines))
	for _, k := range Order {
		if _, ok := machines[k]; ok {
			out = append(out, k)
			seen[k] = true
		}
	}
	var rest []string
	for k := range machines {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// Fetch GETs <baseURL>/api/dashboard and decodes the machine map.
func Fetch(ctx context.Context, baseURL string) (map[string]Machine, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/dashboard", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dashbored: HTTP %d", resp.StatusCode)
	}
	var machines map[string]Machine
	if err := json.NewDecoder(resp.Body).Decode(&machines); err != nil {
		return nil, err
	}
	return machines, nil
}
