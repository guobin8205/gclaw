package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestYAMLProviderParsing(t *testing.T) {
	// Test parsing just a ModelConfig (without the "model:" wrapper)
	modelYAML := `default: deepseek-chat
providers:
  deepseek:
    keys:
      - sk-test123
    base_url: https://api.deepseek.com
    models:
      - deepseek-chat
`
	var mc ModelConfig
	if err := yaml.Unmarshal([]byte(modelYAML), &mc); err != nil {
		t.Fatalf("ModelConfig parse error: %v", err)
	}
	t.Logf("ModelConfig: default=%s providers_count=%d", mc.Default, len(mc.Providers))
	for k, v := range mc.Providers {
		t.Logf("  provider %s: keys=%v base_url=%s", k, v.Keys, v.BaseURL)
	}
	if len(mc.Providers) == 0 {
		t.Fatal("ModelConfig: no providers parsed")
	}

	// Test full Config
	fullYAML := `model:
  default: deepseek-chat
  providers:
    deepseek:
      keys:
        - sk-test123
      base_url: https://api.deepseek.com
      models:
        - deepseek-chat
agent:
  autonomy: interactive
context:
  max_tokens: 128000
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(fullYAML), &cfg); err != nil {
		t.Fatalf("Config parse error: %v", err)
	}
	t.Logf("Config: default=%s providers_count=%d", cfg.Model.Default, len(cfg.Model.Providers))
	for k, v := range cfg.Model.Providers {
		t.Logf("  provider %s: keys=%v base_url=%s", k, v.Keys, v.BaseURL)
	}
	if len(cfg.Model.Providers) == 0 {
		t.Fatal("Config: no providers parsed")
	}
}

func TestSavePreference(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yaml")

	// Write initial config
	initial := "model:\n    default: deepseek-v4-flash\n"
	if err := os.WriteFile(tmpFile, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	// Save a nested preference
	err := SavePreference(tmpFile, "tui.theme", "catppuccin-mocha")
	if err != nil {
		t.Fatalf("SavePreference error: %v", err)
	}

	// Read back and verify
	raw, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Written YAML:\n%s", string(raw))

	var data map[string]any
	if err := yaml.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}

	tui, ok := data["tui"].(map[string]any)
	if !ok {
		t.Fatal("tui key not found or not a map")
	}
	theme, ok := tui["theme"].(string)
	if !ok || theme != "catppuccin-mocha" {
		t.Fatalf("tui.theme = %v, want catppuccin-mocha", tui["theme"])
	}

	// Also verify Load picks it up
	cfg2, err := Load("", tmpFile)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg2.TUI.Theme != "catppuccin-mocha" {
		t.Fatalf("cfg.TUI.Theme = %q, want catppuccin-mocha", cfg2.TUI.Theme)
	}
}

func TestSavePreferenceWithRealConfig(t *testing.T) {
	tmpDir := t.TempDir()
	projectFile := filepath.Join(tmpDir, "config.yaml")

	realConfig := `agent:
    autonomy: interactive
channels:
    weixin:
        enabled: true
context:
    compact_at: 0.85
    compressor_enabled: true
    compressor_model: glm-4-flash
    max_tokens: 128000
    reserve_ratio: 0.15
cron:
    enabled: true
    jobs:
        - enabled: true
          name: github-trending
          schedule: "0 9 * * *"
    model: deepseek-v4-flash
model:
    default: deepseek-v4-flash
    fallback:
        - deepseek-v4-pro
    providers:
        deepseek:
            base_url: https://api.deepseek.com
            keys:
                - sk-testkey123
            models:
                - deepseek-v4-flash
permission:
    mode: default
skills:
    enabled: true
version: 1
`
	if err := os.WriteFile(projectFile, []byte(realConfig), 0644); err != nil {
		t.Fatal(err)
	}

	err := SavePreference(projectFile, "tui.theme", "catppuccin-mocha")
	if err != nil {
		t.Fatalf("SavePreference error: %v", err)
	}

	raw, err := os.ReadFile(projectFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("After SavePreference:\n%s", string(raw))

	cfg, err := Load(projectFile, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	t.Logf("Loaded TUI.Theme = %q", cfg.TUI.Theme)
	if cfg.TUI.Theme != "catppuccin-mocha" {
		t.Fatalf("TUI.Theme = %q, want catppuccin-mocha", cfg.TUI.Theme)
	}
}
