# CHARMULET

The charmulet agentic development environment — a Go TUI built on the Charm
bracelet (bubbletea v2 / ultraviolet / lipgloss v2). Blueprint: `BLUEPRINT.md`.

One binary, nine tabs: chat with local models on demo-desktop, browse files with a
markdown reader, fan prompts out to several models at once, watch the homelab
live, and spawn/resume parallel sessions.

## Run

```sh
go build ./cmd/charmulet && ./charmulet
```

## CLI

```sh
charmulet                    # launch the TUI
charmulet run                # same thing (alias)
charmulet -v                 # version
charmulet -h                 # help
charmulet -T lolcat          # start with a theme from themes/<name>.toml
charmulet -R wat2do          # resume the session matching "wat2do"
charmulet -R last -T dracula # resume newest session, themed
```

`-R` matches session title or model by substring (case-insensitive); `last`
resumes the newest session. No match → notice + fresh chat.

## Tabs

| # | Tab | What it does |
| --- | --- | --- |
| 1 | CHAT | demo-desktop ollama chat · `enter` send · `esc` cancel |
| 2 | FILES | browser + markdown viewer · `enter` open · `esc` back |
| 3 | RUNS | fan one prompt to many models · `p` pick longest · `f` fuse |
| 4 | SERVICES | live homelab cards from dashbored (cpu/mem/disk/GPU/services) |
| 5 | FRONTS | up/down checks for public fronts + local services |
| 6 | LAUNCH | pick a model → spawns a parallel chat tab |
| 7 | SESSIONS | list + resume persisted conversations |
| 8 | AGENTS | edit agent TOML + toggle lazyskills (`.md` / `.md.disabled`) |
| 9 | SETTINGS | live theme switch, zero restart |

Global keys: `tab`/`shift+tab` cycle · `1`-`9` jump · `ctrl+w` close spawned
tab · `?` help · `esc` backs out (cancel/close), then quits · `ctrl+c` quits.

## Configuration

Env vars:

- `CHARMULET_OLLAMA_URL` — ollama base URL (default `http://192.0.2.24:11434`)
- `CHARMULET_DASHBORED_URL` — dashbored feed (default `http://localhost:6900`)
- `CHARMULET_CONFIG_DIR` — config/agents/skills root (default `~/.config/charmulet`)
- `CHARMULET_DB` — session database (default `~/.local/share/charmulet/charmulet.db`)

Files:

- `~/.config/charmulet/charmulet.toml` — theme, ollama url, chat model
- `~/.config/charmulet/agents/*.toml` — agent definitions
- `~/.config/charmulet/skills/*.md` — lazyskills (rename to `.md.disabled` to disable)
- `themes/*.toml` — palettes (default: `pride`)

Sessions persist in SQLite (pure-Go driver, no CGO) and survive restarts.

## Development

```sh
go build ./...   # compile everything
go vet ./...     # static checks
go test ./...    # unit tests (shell routing, stores, providers, tabs)
gofmt -l .       # formatting
```

Architecture rule: `internal/core` never imports `internal/tui` (enforced by
test). The TUI is a face; the core (sessions, providers, agents, config) stays
interface-shaped for a future server mode.
