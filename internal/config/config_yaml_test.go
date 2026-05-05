package config

import (
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
