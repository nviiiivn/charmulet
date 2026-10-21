// Package cartridge is the per-project RAG engine ("game paks"). Intake,
// ollama embeddings, and the sqlite-vec index arrive in P3. P0 defines the
// manifest shape only — no tui imports allowed here, ever.
package cartridge

import "time"

// Manifest describes one inserted cartridge for a project.
type Manifest struct {
	Name       string    `toml:"name"`
	Sources    []string  `toml:"sources"`
	EmbedModel string    `toml:"embed_model"`
	IndexPath  string    `toml:"index_path"`
	CreatedAt  time.Time `toml:"created_at"`
}

// Cartridge is an inserted pak bound to its manifest on disk.
type Cartridge struct {
	Manifest
	Inserted bool
}
