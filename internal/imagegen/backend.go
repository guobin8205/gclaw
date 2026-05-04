package imagegen

import (
	"context"
	"fmt"
	"sync"
)

// GenOptions controls image generation behavior.
type GenOptions struct {
	Model string
	Size  string // "landscape", "square", "portrait"
}

// GenResult is the result of an image generation request.
type GenResult struct {
	URL       string
	LocalPath string
}

// Backend is the interface each image generation provider must implement.
type Backend interface {
	Name() string
	Check() bool // returns true if env vars/config are available
	Generate(ctx context.Context, prompt string, opts GenOptions) (*GenResult, error)
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

// Generate uses the specified backend (or the default) to generate an image.
func (f *Factory) Generate(ctx context.Context, prompt string, opts GenOptions, backendName string) (*GenResult, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Try specified backend
	if backendName != "" {
		for _, b := range f.backends {
			if b.Name() == backendName && b.Check() {
				return b.Generate(ctx, prompt, opts)
			}
		}
	}

	// Use default backend
	if f.default_ != "" {
		for _, b := range f.backends {
			if b.Name() == f.default_ && b.Check() {
				return b.Generate(ctx, prompt, opts)
			}
		}
	}

	// Auto-detect: use first available backend
	for _, b := range f.backends {
		if b.Check() {
			return b.Generate(ctx, prompt, opts)
		}
	}

	return nil, fmt.Errorf("no image generation backend available: set FAL_API_KEY or OPENAI_API_KEY")
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

// NewDefaultFactory creates a Factory with all built-in backends.
// Backends are auto-configured from environment variables.
func NewDefaultFactory(defaultBackend string) *Factory {
	backends := []Backend{
		initFal(),
		initOpenAI(),
	}
	return NewFactory(backends, defaultBackend)
}
