package main

import (
	"charm.land/huh/v2"

	"github.com/nviiiivn/charmulet/internal/core/config"
	"github.com/nviiiivn/charmulet/internal/tui/theme"
)

// runSetup is the first-run wizard (also `charmulet setup`): ollama URL,
// default chat model, theme. huh runs standalone here — same v2 stack as
// the shell, so it can also embed in a tab later.
func runSetup(cfg *config.Config, themesDir string) (*config.Config, error) {
	names := make([]string, 0, 8)
	for _, th := range theme.Available(themesDir) {
		names = append(names, th.Name)
	}
	if len(names) == 0 {
		names = []string{"pride"}
	}
	if cfg.Theme == "" {
		cfg.Theme = "pride"
	}

	url := cfg.OllamaURL
	model := cfg.ChatModel
	pick := cfg.Theme

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Ollama URL").
				Description("e.g. http://localhost:11434").
				Value(&url),
			huh.NewInput().
				Title("Default chat model").
				Description("any model tag your ollama host serves").
				Value(&model),
			huh.NewSelect[string]().
				Title("Theme").
				Options(huh.NewOptions(names...)...).
				Value(&pick),
		),
	)
	if err := form.Run(); err != nil {
		return nil, err
	}

	cfg.OllamaURL = url
	cfg.ChatModel = model
	cfg.Theme = pick
	return cfg, nil
}
