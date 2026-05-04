package websearch

import (
	"context"
	"sync"
)

// SearchResult is a single search result from any backend.
type SearchResult struct {
	Title       string
	URL         string
	Description string
	RawContent  string
}

// SearchOptions controls search behavior.
type SearchOptions struct {
	MaxResults int
	TimeRange  string // "day", "week", "month", "year"
}

// Backend is the interface each search provider must implement.
type Backend interface {
	Name() string
	Check() bool // returns true if env vars/config are available
	Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error)
}

// Factory selects and delegates to the appropriate backend.
type Factory struct {
	mu       sync.RWMutex
	backends []Backend
	default_ string
}

// NewFactory creates a Factory with the given backends.
func NewFactory(backends []Backend, defaultBackend string) *Factory {
	return &Factory{
		backends: backends,
		default_: defaultBackend,
	}
}

// Search uses the specified backend (or the default) to perform a search.
func (f *Factory) Search(ctx context.Context, query string, opts SearchOptions, backendName string) ([]SearchResult, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if backendName != "" {
		for _, b := range f.backends {
			if b.Name() == backendName && b.Check() {
				return b.Search(ctx, query, opts)
			}
		}
	}

	// Use default backend
	if f.default_ != "" {
		for _, b := range f.backends {
			if b.Name() == f.default_ && b.Check() {
				return b.Search(ctx, query, opts)
			}
		}
	}

	// Auto-detect: use first available backend
	for _, b := range f.backends {
		if b.Check() {
			return b.Search(ctx, query, opts)
		}
	}

	return nil, ErrNoBackend
}

// Available returns list of available backend names.
func (f *Factory) Available() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var names []string
	for _, b := range f.backends {
		if b.Check() {
			names = append(names, b.Name())
		}
	}
	return names
}

var ErrNoBackend = NewError("no search backend available: set TAVILY_API_KEY, EXA_API_KEY, or another provider")
