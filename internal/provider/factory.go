package provider

import (
	"fmt"
	"os"

	"github.com/openclaw/gclaw/internal/model"
	"github.com/openclaw/gclaw/internal/model/claude"
	"github.com/openclaw/gclaw/internal/model/ollama"
	"github.com/openclaw/gclaw/internal/model/openai"
)

// Config holds connection details for a single provider.
type Config struct {
	Type             string            // "claude", "openai", "ollama", "openai-compatible", "mock"
	Model            string            // e.g. "claude-sonnet-4-6", "deepseek-chat", "glm-4"
	APIKey           string            // API key
	BaseURL          string            // Override base URL
	Endpoint         string            // Override chat completions endpoint (default /v1/chat/completions)
	SupportsThinking bool              // Set true for reasoning models (DeepSeek-R1, etc.)
	Headers          map[string]string // Extra HTTP headers (for Zhipu etc.)
}

// Factory creates model instances from provider configs.
type Factory struct {
	providers map[string]Config
}

// NewFactory creates a model factory.
func NewFactory() *Factory {
	return &Factory{
		providers: make(map[string]Config),
	}
}

// Register adds a provider configuration.
func (f *Factory) Register(name string, cfg Config) {
	f.providers[name] = cfg
}

// Build creates a Model instance from a named provider config.
func (f *Factory) Build(name string) (model.Model, error) {
	cfg, ok := f.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}

	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = resolveAPIKey(cfg.Type)
	}

	switch cfg.Type {
	case "claude":
		if apiKey == "" {
			return nil, fmt.Errorf("claude: set ANTHROPIC_API_KEY or CLAUDE_API_KEY")
		}
		c := claude.New(cfg.Model, apiKey)
		if cfg.BaseURL != "" {
			c.SetBaseURL(cfg.BaseURL)
		}
		return c, nil

	case "openai", "openai-compatible":
		if apiKey == "" && cfg.BaseURL == "" {
			return nil, fmt.Errorf("openai: set OPENAI_API_KEY")
		}
		oa := openai.New(cfg.Model, apiKey, cfg.BaseURL)
		if cfg.Endpoint != "" {
			oa.SetEndpoint(cfg.Endpoint)
		}
		if cfg.SupportsThinking {
			oa.SetThinking(true)
		}
		for k, v := range cfg.Headers {
			oa.SetHeader(k, v)
		}
		return oa, nil

	case "ollama":
		return ollama.New(cfg.Model, cfg.BaseURL), nil

	case "mock":
		return model.NewMock(cfg.Model), nil

	default:
		return nil, fmt.Errorf("unknown provider type: %s (valid: claude, openai, ollama, openai-compatible, mock)", cfg.Type)
	}
}

// BuildAll creates all registered providers.
func (f *Factory) BuildAll() (map[string]model.Model, error) {
	models := make(map[string]model.Model)
	for name := range f.providers {
		m, err := f.Build(name)
		if err != nil {
			return nil, fmt.Errorf("build %s: %w", name, err)
		}
		models[name] = m
	}
	return models, nil
}

// Names returns all registered provider names.
func (f *Factory) Names() []string {
	names := make([]string, 0, len(f.providers))
	for n := range f.providers {
		names = append(names, n)
	}
	return names
}

// resolveAPIKey looks up the API key for a provider type.
func resolveAPIKey(providerType string) string {
	switch providerType {
	case "claude":
		return firstEnv("ANTHROPIC_API_KEY", "CLAUDE_API_KEY")
	case "deepseek":
		return os.Getenv("DEEPSEEK_API_KEY")
	case "openai", "openai-compatible":
		return os.Getenv("OPENAI_API_KEY")
	default:
		return ""
	}
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// DefaultFactory auto-detects available providers from environment.
func DefaultFactory() *Factory {
	f := NewFactory()

	// Always register mock and ollama
	f.Register("mock", Config{Type: "mock", Model: "mock-dev"})
	f.Register("ollama", Config{
		Type:    "ollama",
		Model:   envOrDefault("OLLAMA_MODEL", "llama3.2"),
		BaseURL: envOrDefault("OLLAMA_BASE_URL", "http://localhost:11434"),
	})

	// Claude
	if key := firstEnv("ANTHROPIC_API_KEY", "CLAUDE_API_KEY"); key != "" {
		f.Register("claude", Config{
			Type:   "claude",
			Model:  envOrDefault("CLAUDE_MODEL", "claude-sonnet-4-6"),
			APIKey: key,
		})
	}

	// OpenAI
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		f.Register("openai", Config{
			Type:   "openai",
			Model:  envOrDefault("OPENAI_MODEL", "gpt-4o"),
			APIKey: key,
		})
	}

	// DeepSeek (OpenAI-compatible API)
	if key := os.Getenv("DEEPSEEK_API_KEY"); key != "" {
		f.Register("deepseek", Config{
			Type:             "openai-compatible",
			Model:            envOrDefault("DEEPSEEK_MODEL", "deepseek-chat"),
			APIKey:           key,
			BaseURL:          "https://api.deepseek.com",
			SupportsThinking: true,
		})
	}

	// Zhipu (GLM)
	if key := os.Getenv("ZHIPU_API_KEY"); key != "" {
		f.Register("zhipu", Config{
			Type:     "openai-compatible",
			Model:    envOrDefault("ZHIPU_MODEL", "glm-4"),
			APIKey:   key,
			BaseURL:  "https://open.bigmodel.cn/api/paas/v4",
			Endpoint: "/chat/completions",
		})
	}

	// Baidu Qianfan
	if key := firstEnv("QIANFAN_API_KEY", "BAIDU_API_KEY"); key != "" {
		f.Register("qianfan", Config{
			Type:     "openai-compatible",
			Model:    envOrDefault("QIANFAN_MODEL", "ernie-bot-4"),
			APIKey:   key,
			BaseURL:  "https://qianfan.baidubce.com",
			Endpoint: "/v2/chat/completions",
		})
	}

	// Moonshot (Kimi)
	if key := os.Getenv("MOONSHOT_API_KEY"); key != "" {
		f.Register("moonshot", Config{
			Type:    "openai-compatible",
			Model:   envOrDefault("MOONSHOT_MODEL", "moonshot-v1-8k"),
			APIKey:  key,
			BaseURL: "https://api.moonshot.cn",
		})
	}

	return f
}

// Available returns a human-readable summary of available providers.
func (f *Factory) Available() string {
	names := f.Names()
	if len(names) == 0 {
		return "no providers registered"
	}
	result := ""
	for _, n := range names {
		if cfg, ok := f.providers[n]; ok {
			result += fmt.Sprintf("  %s (%s/%s)\n", n, cfg.Type, cfg.Model)
		}
	}
	return result
}
