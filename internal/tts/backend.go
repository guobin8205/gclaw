package tts

import (
	"context"
	"fmt"
)

// TTSOptions holds options for text-to-speech synthesis.
type TTSOptions struct {
	Voice  string
	Speed  float64
	Format string // "mp3", "wav"
}

// TTSResult holds the result of a TTS operation.
type TTSResult struct {
	FilePath string
}

// Backend is the interface that TTS backends must implement.
type Backend interface {
	Name() string
	Check() bool
	Speak(ctx context.Context, text string, opts TTSOptions) (*TTSResult, error)
}

// Factory manages TTS backends and dispatches Speak calls.
type Factory struct {
	backends []Backend
	default_ string
}

// NewFactory creates a new TTS factory with the given backends.
func NewFactory(backends []Backend, defaultBackend string) *Factory {
	return &Factory{
		backends: backends,
		default_: defaultBackend,
	}
}

// Speak selects a backend and delegates the TTS call.
func (f *Factory) Speak(ctx context.Context, text string, opts TTSOptions, backendName string) (*TTSResult, error) {
	var backend Backend

	if backendName != "" {
		// Use explicitly requested backend
		for _, b := range f.backends {
			if b.Name() == backendName {
				backend = b
				break
			}
		}
		if backend == nil {
			return nil, fmt.Errorf("tts: unknown backend %q", backendName)
		}
	} else {
		// Use default backend
		if f.default_ != "" {
			for _, b := range f.backends {
				if b.Name() == f.default_ {
					backend = b
					break
				}
			}
		}
		if backend == nil && len(f.backends) > 0 {
			// Fallback to first available
			for _, b := range f.backends {
				if b.Check() {
					backend = b
					break
				}
			}
		}
	}

	if backend == nil {
		return nil, fmt.Errorf("tts: no available backend")
	}

	if !backend.Check() {
		return nil, fmt.Errorf("tts: backend %q is not available", backend.Name())
	}

	return backend.Speak(ctx, text, opts)
}

// Available returns the names of all backends that pass their Check.
func (f *Factory) Available() []string {
	var names []string
	for _, b := range f.backends {
		if b.Check() {
			names = append(names, b.Name())
		}
	}
	return names
}
