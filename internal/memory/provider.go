package memory

import (
	"context"
	"strings"
)

// Provider is the interface for memory backends.
type Provider interface {
	Available() bool
	Initialize(sessionID string) error
	Prefetch(ctx context.Context, query string, limit int) ([]Entry, error)
	SyncTurn(ctx context.Context, userMsg, assistantMsg string) error
	SystemPromptBlock() string
	Shutdown() error
}

// Manager orchestrates multiple memory providers.
type Manager struct {
	providers []Provider
}

// NewManager creates a memory manager.
func NewManager(providers ...Provider) *Manager {
	return &Manager{providers: providers}
}

// AddProvider appends a provider (call during setup, before goroutines start).
func (m *Manager) AddProvider(p Provider) {
	m.providers = append(m.providers, p)
}

// Initialize calls Initialize on all providers.
func (m *Manager) Initialize(sessionID string) error {
	for _, p := range m.providers {
		if p.Available() {
			if err := p.Initialize(sessionID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Prefetch queries all providers and returns combined results wrapped in <memory-context>.
func (m *Manager) Prefetch(ctx context.Context, query string, limit int) string {
	var results []string
	for _, p := range m.providers {
		if !p.Available() {
			continue
		}
		entries, err := p.Prefetch(ctx, query, limit)
		if err != nil || len(entries) == 0 {
			continue
		}
		for _, e := range entries {
			results = append(results, "["+e.Type+"] "+e.Title+": "+e.Content)
		}
	}
	if len(results) == 0 {
		return ""
	}
	return "<memory-context>\n" + strings.Join(results, "\n") + "\n</memory-context>"
}

// SyncTurn propagates a turn to all providers.
func (m *Manager) SyncTurn(ctx context.Context, userMsg, assistantMsg string) {
	for _, p := range m.providers {
		if p.Available() {
			_ = p.SyncTurn(ctx, userMsg, assistantMsg)
		}
	}
}

// SystemPromptBlock returns the combined static blocks from all providers.
func (m *Manager) SystemPromptBlock() string {
	var blocks []string
	for _, p := range m.providers {
		if p.Available() {
			if b := p.SystemPromptBlock(); b != "" {
				blocks = append(blocks, b)
			}
		}
	}
	return strings.Join(blocks, "\n")
}

// Shutdown shuts down all providers.
func (m *Manager) Shutdown() {
	for _, p := range m.providers {
		if p.Available() {
			_ = p.Shutdown()
		}
	}
}
