package websearch

// NewDefaultFactory creates a Factory with all built-in backends.
// Backends are auto-configured from environment variables.
func NewDefaultFactory(defaultBackend string) *Factory {
	backends := []Backend{
		initTavily(),
		initExa(),
	}
	return NewFactory(backends, defaultBackend)
}
