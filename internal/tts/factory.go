package tts

import "os"

// NewDefaultFactory creates a Factory with all built-in backends.
// Backends are auto-configured from environment variables.
func NewDefaultFactory(defaultBackend string) *Factory {
	backends := []Backend{
		&OpenAIBackend{
			APIKey: os.Getenv("OPENAI_API_KEY"),
		},
	}
	return NewFactory(backends, defaultBackend)
}
