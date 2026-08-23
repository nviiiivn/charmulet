// Package quota meters per-provider usage. Feeds the Monitor tab via
// quota.tick events in P2.
package quota

import (
	"sync"
	"time"
)

type Usage struct {
	Provider string
	Model    string
	Tokens   int
	Requests int
	At       time.Time
}

type Meter struct {
	mu         sync.Mutex
	byProvider map[string]*Usage
}

func NewMeter() *Meter {
	return &Meter{byProvider: map[string]*Usage{}}
}

func (m *Meter) Record(provider, model string, tokens int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byProvider[provider]
	if !ok {
		u = &Usage{Provider: provider}
		m.byProvider[provider] = u
	}
	u.Model = model
	u.Tokens += tokens
	u.Requests++
	u.At = time.Now()
}

func (m *Meter) Snapshot() map[string]Usage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]Usage, len(m.byProvider))
	for k, v := range m.byProvider {
		out[k] = *v
	}
	return out
}
