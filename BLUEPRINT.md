# CHARMULET — Architecture Blueprint v0

> Status: DRAFT 2026-08-23 by Architect (ox-alpha). Not implemented yet.
> NAME LOCKED: CHARMULET (verified clear as a software name 2026-08-23 — only collision is a jewelry site).
> Hierarchy: TOTEMIC = overhead umbrella entity · totem = the CLI you run today · CHARMULET = the new derivative experience under TOTEMIC but its own thing — a philosophical + literal + graphical rewrite of the stack.
> Inherits ideas from the TOT3MIC lineage (machinery-not-prose, enforcement hooks), never its code.

## 1. Thesis

OpenChamber's power is its shape: one server core exposing sessions/tools/events/quota, with thin interchangeable faces on top (web/desktop/mobile/VS Code). CHARMULET adopts that shape natively in Go:

```
CHARMULET SHELL (bubbletea v2 TUI)
  [Chat][Runs][Files][Changes][Monitor][Cartridges][Settings]
        | tea.Msg bus |
CORE (Go packages): session · provider · events · quota · cartridge · config
        |
SQLite
```

Rules inherited from Crush's proven split:
1. Core never imports the TUI. The TUI is a client of core.
2. Monolithic in-process by default; every core call is interface-shaped so `serve` mode (HTTP/SSE) drops in later without redesign.

## 2. Stack

| Concern | Choice |
|---|---|
| Language | Go, single static binary |
| UI | Bubble Tea v2 (runs on ultraviolet — cell diffing renderer, universal input — free underneath) |
| Layout splits | lipgloss v2 joins + ultraviolet `layout` constraints (Cassowary solver) |
| Theming ("NES cartridge" skins) | lipgloss style sets loaded from TOML data files, hot-swappable at runtime |
| Widgets | bubbles + evertras/bubble-table (sortable tables) |
| Markdown render | glow embedded |
| CLI entry | fang |
| Regression tests | vhs tapes in CI (render tabs, diff frames) |
| Persistence | SQLite |
| Embeddings | demo-desktop ollama nomic-embed-* over HTTP (hardware exists: demo-desktop 192.0.2.24; no new infra) |
| Config | layered: global -> project -> runtime overrides (crush.json-style) |

NOT in v1: web/mobile/desktop faces (interfaces allow later), auto-fusion beyond manual selection, VS Code extension.

## 3. Repo layout

```
charmulet/
├── cmd/charmulet/main.go          fang-wrapped entrypoint
├── internal/
│   ├── core/                   NO tui imports allowed (CI lint enforces)
│   │   ├── session/ provider/ events/ quota/ cartridge/ config/
│   ├── tui/
│   │   ├── shell/              tabbar, statusbar, router, msg routing
│   │   ├── tabs/               one package per tab
│   │   ├── theme/              cartridge skins (pure data)
│   │   └── widgets/            diff viewer, sparkline, toast
│   └── server/                 optional HTTP+SSE wrapper over core
├── themes/*.toml
└── tapes/*.tape                vhs regression recordings
```

Lint rule: any `internal/core/**` file importing `internal/tui/**` fails the build. Machinery, not prose.

## 4. Tab contract

```go
type Tab interface {
    Title() string
    ID() string                       // stable id: keybinds 1-9, persistence
    Init() tea.Cmd
    Update(msg tea.Msg) (Tab, tea.Cmd)
    View() string
    Activate() tea.Cmd                // resume polling/streams when focused
    Deactivate()                      // pause work, retain state
    Resize(w, h int)
}
// Optional capability interfaces (shell type-switches; tabs opt in):
type CommandPalettable interface{ Commands() []Command }
type StatusReporter    interface{ StatusLine() string }
type CartridgeAware    interface{ SetCartridge(c *cartridge.Cartridge) }
```

Shell owns: global keybinds (1-7 jump-to-tab, tab/shift-tab cycle, ctrl+p palette), message fan-out (global msgs broadcast to all tabs, tab msgs scoped), chrome frame, last-active-tab persistence across restarts.

## 5. Message flow

- User input -> shell.Update -> global bind? handle : route to activeTab.Update.
- tea.Cmd async results return as Msgs; all long core calls wrapped in goroutine Cmds — tabs NEVER block the update loop.
- Streaming LLM tokens arrive batched (~30ms TokenMsg groups) to keep the diffing renderer cheap.
- Core event bus (`core/events`) typed topics: session.created, token.stream, tool.call, tool.result, quota.tick, cartridge.indexed, config.changed. TUI subscribes once at startup; the future SSE server subscribes identically — one publisher, N transports.

## 6. Tabs

**Chat** (main comm window) — textarea input + viewport transcript, glow-rendered markdown. Ultraviolet layout split: transcript left, context panel right (active cartridge hits, tool-call trace). Esc = mid-stream cancel through core (also satisfies the R5 lineage requirement).

**Runs** (multi-run / fusion) — fan one prompt to up to 5 models concurrently; each streams into its own pane column. Pick-best marks a winner and promotes it into the main session. Fuse = manually select spans across panes -> composed reply (auto-fusion deferred).

**Files** — browser pane (bubbles list, git-status aware) + viewer pane (syntax-highlighted; glow for .md; scrollable extracted text for PDFs). Keybind sends selected file into Chat context.

**Changes** (diff walkthrough) — large diffs grouped into ordered steps (file-grouped hunks); step list left, unified diff right; per-step agent-written "why this fits together" note generated post-diff, cached.

**Monitor** (quota/live dashboard) — fed purely by quota.tick events, zero polling logic in the tab: per-provider spend/tokens/requests tables + sparklines + provider health pings. Reference vibe: ocmonitor.vercel.app/docs.

**Cartridges** (per-project RAG "game paks") — cartridge = {name, sources[], embed_model, index_path, manifest}. Intake flow (TUI-driven): point at PDFs/text files (use pre-extracted OCR text layers — e.g. the EX500 Haynes OCR set already on the Pi) -> chunk -> embed via demo-desktop nomic-embed -> sqlite-vec index. Query via explicit @reference or chat relevance -> top-k chunks injected with page/chunk citations. Slots render inserted/ejected like NES paks, lipgloss-styled.

**Settings** — live-editable providers/credentials/models/plugins/theme against layered config. Save emits config.changed; everything re-themes/re-resolves with ZERO restarts. Credentials are read-only display, resolved from external stores (env / vault), never echoed or written into configs.

## 7. Phases

- **P0 Walking skeleton**: scaffold, shell + tabbar/statusbar, 2 stub tabs, TOML theme load, vhs smoke test in CI. Exit: switch/resize/live theme swap, tests green.
- **P1 Real Chat**: session store + providers (ollama + one hosted), streaming, cancel, markdown, persistence. Exit: real conversation, kill mid-stream, restart reloads history.
- **P2 Environment tabs**: Files full, Changes walkthrough, Monitor meters, Runs fan-out/pick-best. Exit: all four usable standalone.
- **P3 Cartridges**: ingest pipeline, demo-desktop embeddings, sqlite-vec index, @refs in chat, management UI. Exit: Haynes indexed; "head bolt torque" returns cited answer.
- **P4 Settings/plugins**: live config editing, plugin registry (stdio protocol, MCP-compatible), theme folder. Exit: add an ollama model and swap a theme entirely in-TUI, no restart.
- **P5 Server mode** (optional): serve wrapper HTTP+SSE over core events; unlocks phone/browser faces without redesign.

Each phase ends with vhs evidence + green tests before the next starts.

## 8. Risks

| # | Risk | Mitigation |
|---|---|---|
| Q1 | Bubble Tea v2 / ultraviolet API still moving ("API may change") | pin versions; isolate custom ultraviolet use behind tui/widgets |
| Q2 | On-device vector scale | cartridges small/per-project; sqlite-vec fine until >100MB indexes |
| Q3 | PDF extraction quality varies wildly | standardize pre-extracted text layers as v1 input format |
| Q4 | Secrets in settings UI | read-only display, external store resolution, never echoed |
| Q5 | Fusion quality unknown | manual pick/fuse first; measure before automating |

## 9. Handoff plan

Planning stays with Architect. Execution delegates to golang-pro (TUI heavy) and backend-developer (core/server), starting P0 after blueprint confirmation.
