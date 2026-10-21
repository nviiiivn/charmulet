package homelab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const happyFeed = `{
  "demo-desktop": {
    "display": "demo-desktop", "color": "#58a6ff",
    "widgets": {
      "system": {"cpu": "i7", "cpu_percent": 12.5, "disk": "1.6T/1.9T (88%)", "disk_percent": 88,
                  "gpu": "NVIDIA GeForce RTX 3080, 5162 MiB, 20480 MiB, 0 %", "gpu_percent": 0,
                  "mem_used": "8000", "mem_total": "64000", "memory_percent": 12.5,
                  "temp": "47C", "uptime": "2 days", "hostname": "demo-desktop"},
      "services": {"services": [{"port": "8188", "name": "comfy", "process": "python", "pid": "42", "active": true}]},
      "ollama": {"models": [{"name": "qwen3.5:9b", "size": "5.7 GB", "modified": "2025-01-01"}], "running": ["qwen3.5:9b"]},
      "media": {"status": "playing", "source": "navidrome", "now_playing": "track"},
      "network": {"ignored": true}
    }
  },
  "demo-server": {
    "display": "Pi 5", "color": "#3fb950",
    "widgets": {"system": {"cpu": "aarch64", "cpu_percent": 3.1, "memory_percent": 40, "hostname": "demo-server"}}
  },
  "zeta-extra": {
    "display": "Zeta", "color": "#ffffff",
    "widgets": {"system": {"error": "unreachable"}}
  }
}`

func TestFetchHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/dashboard" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(happyFeed))
	}))
	defer srv.Close()

	machines, err := Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(machines) != 3 {
		t.Fatalf("want 3 machines, got %d", len(machines))
	}

	tw := machines["demo-desktop"]
	if tw.Display != "demo-desktop" || tw.Color != "#58a6ff" {
		t.Fatalf("demo-desktop meta wrong: %+v", tw)
	}
	sys := tw.Widgets.System
	if sys == nil || sys.DiskPercent != 88 || sys.GPUPercent != 0 || sys.Temp != "47C" {
		t.Fatalf("demo-desktop system widget wrong: %+v", sys)
	}
	if len(tw.Widgets.Services.Services) != 1 || !tw.Widgets.Services.Services[0].Active {
		t.Fatalf("services widget wrong: %+v", tw.Widgets.Services)
	}
	ol := tw.Widgets.Ollama
	if ol == nil || len(ol.Models) != 1 || ol.Models[0].Name != "qwen3.5:9b" || len(ol.Running) != 1 {
		t.Fatalf("ollama widget wrong: %+v", ol)
	}
	if tw.Widgets.Media.NowPlaying != "track" {
		t.Fatalf("media widget wrong: %+v", tw.Widgets.Media)
	}

	// error-widget machine: Error set, everything else zero
	if z := machines["zeta-extra"].Widgets.System; z == nil || z.Error != "unreachable" {
		t.Fatalf("error widget not surfaced: %+v", z)
	}
}

func TestOrderedCanonicalThenExtras(t *testing.T) {
	machines := map[string]Machine{
		"zeta-extra":  {},
		"demo-laptop": {},
		"demo-server": {},
	}
	got := Ordered(machines)
	want := []string{"demo-server", "demo-laptop", "zeta-extra"}
	if len(got) != len(want) {
		t.Fatalf("order %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want %v got %v", want, got)
		}
	}
}

func TestFetchNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("want HTTP 500 error, got %v", err)
	}
}

func TestFetchUnreachable(t *testing.T) {
	// port closed: use a server we shut down immediately
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if _, err := Fetch(context.Background(), url); err == nil {
		t.Fatal("unreachable server must error")
	}
}
