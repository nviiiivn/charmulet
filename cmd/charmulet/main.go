package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/nviiiivn/charmulet/internal/core/config"
	"github.com/nviiiivn/charmulet/internal/core/provider"
	"github.com/nviiiivn/charmulet/internal/core/session"
	"github.com/nviiiivn/charmulet/internal/providers/ollama"
	"github.com/nviiiivn/charmulet/internal/tui/shell"
	"github.com/nviiiivn/charmulet/internal/tui/tabs"
	"github.com/nviiiivn/charmulet/internal/tui/tabs/agents"
	changestabs "github.com/nviiiivn/charmulet/internal/tui/tabs/changes"
	"github.com/nviiiivn/charmulet/internal/tui/tabs/chat"
	filestabs "github.com/nviiiivn/charmulet/internal/tui/tabs/files"
	frontstabs "github.com/nviiiivn/charmulet/internal/tui/tabs/fronts"
	launchtabs "github.com/nviiiivn/charmulet/internal/tui/tabs/launch"
	pagestabs "github.com/nviiiivn/charmulet/internal/tui/tabs/pages"
	"github.com/nviiiivn/charmulet/internal/tui/tabs/runs"
	servicestabs "github.com/nviiiivn/charmulet/internal/tui/tabs/services"
	sessionstabs "github.com/nviiiivn/charmulet/internal/tui/tabs/sessions"
	settingstabs "github.com/nviiiivn/charmulet/internal/tui/tabs/settings"
	"github.com/nviiiivn/charmulet/internal/tui/theme"
)

func loadTheme(name string) theme.Theme {
	if name == "" || name == "pride" {
		if th, err := theme.Load(filepath.Join("themes", "pride.toml")); err == nil {
			return th
		}
		return theme.Default()
	}
	th, err := theme.Load(filepath.Join("themes", name+".toml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "theme %q not found — falling back to pride\n", name)
		return theme.Default()
	}
	return th
}

func dashURL() string {
	if v := os.Getenv("CHARMULET_DASHBORED_URL"); v != "" {
		return v
	}
	return "http://localhost:6900"
}

// resumeTab finds a persisted session by title/model substring ("last" or
// "" = most recent) and materializes a live chat tab for it. nil = no match.
func resumeTab(st *session.Store, th theme.Theme, prov provider.Provider, q string) tabs.Tab {
	if st == nil {
		return nil
	}
	metas, err := st.List()
	if err != nil || len(metas) == 0 {
		return nil
	}
	var meta *session.SessionMeta
	if q == "" || strings.EqualFold(q, "last") {
		meta = &metas[0]
	} else {
		lq := strings.ToLower(q)
		for i := range metas {
			if strings.Contains(strings.ToLower(metas[i].Title), lq) ||
				strings.Contains(strings.ToLower(metas[i].Model), lq) {
				meta = &metas[i]
				break
			}
		}
	}
	if meta == nil {
		return nil
	}
	msgs, err := st.Messages(meta.ID)
	if err != nil {
		return nil
	}
	return chat.NewResume(th, prov, st, *meta, msgs)
}

// configExists reports whether any charmulet.toml is already on disk.
func configExists() bool {
	for _, p := range config.Paths() {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func runTUI(themeName, resumeQuery, version string) error {
	// First run with no config anywhere: walk her through setup before
	// the shell takes over the screen. A -T override skips the wizard.
	if themeName == "" && !configExists() {
		cfg, err := config.Load()
		if err != nil {
			cfg = &config.Config{OllamaURL: "http://localhost:11434", ChatModel: "qwen3.5:9b"}
		}
		if cfg, err = runSetup(cfg, "themes"); err == nil {
			_ = config.Save(cfg)
		} else {
			fmt.Fprintf(os.Stderr, "setup skipped: %v\n", err)
		}
	}

	th := loadTheme(themeName)
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{OllamaURL: "http://localhost:11434", ChatModel: "qwen3.5:9b"}
	}
	prov := ollama.New(cfg.OllamaURL)

	st, err := session.Open()
	if err != nil {
		st = nil // dashboard tabs work fine without persistence
	}

	var initial []tabs.Tab
	if rt := resumeTab(st, th, prov, resumeQuery); rt != nil {
		initial = append(initial, rt)
	} else {
		if resumeQuery != "" {
			fmt.Fprintf(os.Stderr, "no session matches %q — starting fresh\n", resumeQuery)
		}
		initial = append(initial, chat.New(th, prov, cfg.ChatModel))
	}
	initial = append(initial,
		filestabs.New(th),
		runs.New(th, prov),
		servicestabs.New(th, dashURL()),
		frontstabs.New(th),
		launchtabs.New(th, prov, st),
		sessionstabs.New(th, prov, st),
		agents.New(th),
		settingstabs.New(th, cfg, "themes", version),
		pagestabs.New(th, dashURL()),
		changestabs.New(th, "."),
	)

	p := tea.NewProgram(shell.New(th, initial...))
	_, err = p.Run()
	return err
}

const appVersion = "0.1.0"

func main() {
	var themeName, resumeQuery string

	root := &cobra.Command{
		Use:     "charmulet",
		Short:   "The charmulet agentic development environment",
		Version: appVersion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(themeName, resumeQuery, appVersion)
		},
	}

	// `charmulet run` stays as an alias for muscle memory.
	run := &cobra.Command{
		Use:   "run",
		Short: "Launch the CHARMULET TUI (same as bare charmulet)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(themeName, resumeQuery, appVersion)
		},
	}

	// `charmulet setup` re-runs the first-run wizard and saves.
	setup := &cobra.Command{
		Use:   "setup",
		Short: "Re-run the setup wizard (ollama URL, model, theme)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				cfg = &config.Config{OllamaURL: "http://localhost:11434", ChatModel: "qwen3.5:9b"}
			}
			if cfg, err = runSetup(cfg, "themes"); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Println("saved —", config.Paths()[len(config.Paths())-1])
			return nil
		},
	}

	root.Flags().BoolP("version", "v", false, "print version")
	root.PersistentFlags().StringVarP(&themeName, "theme", "T", "", "theme name (themes/<name>.toml, default pride)")
	root.PersistentFlags().StringVarP(&resumeQuery, "resume", "R", "", "resume a session by title/model substring, or \"last\"")

	root.AddCommand(run, setup)
	ctx := context.Background()
	if err := fang.Execute(ctx, root, fang.WithVersion(appVersion)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
