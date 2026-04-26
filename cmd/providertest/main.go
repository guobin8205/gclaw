package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/openclaw/gclaw/internal/config"
	"github.com/openclaw/gclaw/internal/model"
	"github.com/openclaw/gclaw/internal/provider"
)

func main() {
	fmt.Println("=== Config-Driven Provider Test ===")
	fmt.Println()

	cwd, _ := os.Getwd()
	projectPath := config.FindProjectConfig(cwd)
	userPath, _ := config.UserConfigPath()

	fmt.Printf("Project config: %s\n", projectPath)

	cfg, err := config.Load(projectPath, userPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Default model: %s\n", cfg.Model.Default)
	fmt.Printf("Fallback: %v\n", cfg.Model.Fallback)
	fmt.Printf("Autonomy: %s\n", cfg.Agent.Autonomy)
	fmt.Println()

	f := setupProviders(cfg)
	fmt.Printf("Registered providers: %v\n", f.Names())
	fmt.Println()

	for _, name := range f.Names() {
		if name == "mock" || name == "ollama" {
			continue
		}
		fmt.Printf("--- %s ---\n", name)
		m, err := f.Build(name)
		if err != nil {
			fmt.Printf("  SKIP: %v\n\n", err)
			continue
		}
		testModel(m)
	}

	fmt.Println("--- Resolve default model ---")
	defaultModel := resolveModel(f, cfg)
	if defaultModel != nil {
		fmt.Printf("  Resolved: %s\n", defaultModel.ID())
	}

	fmt.Println()
	fmt.Println("=== Done ===")
}

func setupProviders(cfg *config.Config) *provider.Factory {
	f := provider.DefaultFactory()

	for name, p := range cfg.Model.Providers {
		keys := config.GetProviderKeys(p)
		apiKey := ""
		if len(keys) > 0 {
			apiKey = keys[0]
		}
		models := p.Models
		if len(models) == 0 {
			if p.Model != "" {
				models = []string{p.Model}
			} else {
				models = []string{cfg.Model.Default}
			}
		}
		for _, modelName := range models {
			f.Register(modelName, provider.Config{
				Type:             resolveProviderType(name),
				Model:            modelName,
				APIKey:           apiKey,
				BaseURL:          p.BaseURL,
				Endpoint:         p.Endpoint,
				SupportsThinking: p.Thinking,
			})
		}
	}

	return f
}

func resolveProviderType(name string) string {
	switch name {
	case "anthropic", "claude":
		return "claude"
	case "openai":
		return "openai"
	case "deepseek", "zhipu", "qianfan", "moonshot":
		return "openai-compatible"
	case "ollama":
		return "ollama"
	default:
		return name
	}
}

func resolveModel(f *provider.Factory, cfg *config.Config) model.Model {
	names := f.Names()
	if len(names) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no model providers configured\n")
		os.Exit(1)
	}

	if cfg.Model.Default != "" {
		m, err := f.Build(cfg.Model.Default)
		if err == nil {
			return m
		}
		fmt.Printf("  Cannot build default '%s': %v\n", cfg.Model.Default, err)
	}

	for _, name := range cfg.Model.Fallback {
		m, err := f.Build(name)
		if err == nil {
			fmt.Printf("  Using fallback: %s\n", name)
			return m
		}
	}

	for _, name := range names {
		m, err := f.Build(name)
		if err == nil {
			fmt.Printf("  Using auto-selected: %s\n", name)
			return m
		}
	}

	fmt.Fprintf(os.Stderr, "Error: no available model provider\n")
	os.Exit(1)
	return nil
}

func testModel(m model.Model) {
	fmt.Printf("  ID: %s\n", m.ID())
	fmt.Printf("  MaxTokens: %d\n", m.MaxTokens())
	fmt.Printf("  Streaming: %v\n", m.SupportsStreaming())
	fmt.Printf("  Thinking: %v\n", m.SupportsThinking())

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := m.Call(ctx, model.CallParams{
		SystemPrompt: "Only reply 'OK' and nothing else.",
		Messages: []model.Message{
			{Role: "user", Content: "Hello"},
		},
		MaxTokens: 50,
	})
	if err != nil {
		fmt.Printf("  FAIL: %v\n\n", err)
		return
	}
	fmt.Printf("  Response: %s\n", truncate(resp.Text, 200))
	fmt.Printf("  Usage: in=%d out=%d\n", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	fmt.Println("  Status: OK")
	fmt.Println()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
