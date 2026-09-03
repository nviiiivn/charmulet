package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"charm.land/bubbletea/v2"
	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/nvii/charmulet/internal/core/config"
	"github.com/nvii/charmulet/internal/providers/ollama"
	"github.com/nvii/charmulet/internal/tui/shell"
	"github.com/nvii/charmulet/internal/tui/tabs/chat"
	filestabs "github.com/nvii/charmulet/internal/tui/tabs/files"
	frontstabs "github.com/nvii/charmulet/internal/tui/tabs/fronts"
	launchtabs "github.com/nvii/charmulet/internal/tui/tabs/launch"
	"github.com/nvii/charmulet/internal/tui/tabs/runs"
	servicestabs "github.com/nvii/charmulet/internal/tui/tabs/services"
	"github.com/nvii/charmulet/internal/tui/theme"
)

func loadTheme() theme.Theme {
	th, err := theme.Load(filepath.Join("themes", "pride.toml"))
	if err != nil {
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

func main() {
	root := &cobra.Command{
		Use:     "charmulet",
		Short:   "The charmulet agentic development environment",
		Version: "0.0.1-p0",
	}

	run := &cobra.Command{
		Use:   "run",
		Short: "Launch the CHARMULET TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			th := loadTheme()
			cfg, err := config.Load()
			if err != nil {
				cfg = &config.Config{OllamaURL: "http://192.0.2.24:11434", ChatModel: "qwen3.5:9b"}
			}
			prov := ollama.New(cfg.OllamaURL)
			m := shell.New(th,
				chat.New(th, prov, cfg.ChatModel),
				filestabs.New(th),
				runs.New(th, prov),
				servicestabs.New(th, dashURL()),
				frontstabs.New(th),
				launchtabs.New(th, prov),
			)
			p := tea.NewProgram(m)
			_, err = p.Run()
			return err
		},
	}

	root.AddCommand(run)
	if err := fang.Execute(context.Background(), root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
