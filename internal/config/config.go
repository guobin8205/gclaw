package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	Version    int              `yaml:"version"`
	Model      ModelConfig      `yaml:"model"`
	Agent      AgentConfig      `yaml:"agent"`
	Context    ContextConfig    `yaml:"context"`
	Permission PermissionConfig `yaml:"permission"`
	Triggers   TriggersConfig   `yaml:"triggers"`
	Cron       CronConfig       `yaml:"cron"`
	Channels   ChannelsConfig   `yaml:"channels"`
	Plugins    PluginsConfig    `yaml:"plugins"`
	Logging    LoggingConfig    `yaml:"logging"`
}

// CronConfig holds scheduled job configuration.
type CronConfig struct {
	Enabled bool      `yaml:"enabled"`
	Model   string    `yaml:"model"`
	Jobs    []CronJob `yaml:"jobs"`
}

// CronJob is a single scheduled task.
type CronJob struct {
	Name          string `yaml:"name"`
	Schedule      string `yaml:"schedule"` // 5-field cron expression
	Prompt        string `yaml:"prompt"`
	Enabled       bool   `yaml:"enabled"`
	NotifyWeixin  bool   `yaml:"notify_weixin"`
}

// ChannelsConfig holds messaging channel configuration.
type ChannelsConfig struct {
	Weixin WeixinChannelConfig `yaml:"weixin"`
}

// WeixinChannelConfig holds WeChat channel settings.
type WeixinChannelConfig struct {
	Enabled bool `yaml:"enabled"`
	Verbose bool `yaml:"verbose"`
}

// ModelConfig holds model provider configuration.
type ModelConfig struct {
	Default   string            `yaml:"default"`
	Fallback  []string          `yaml:"fallback"`
	Providers map[string]ProviderConfig `yaml:"providers"`
}

// ProviderConfig holds a single provider's configuration.
type ProviderConfig struct {
	Keys     []string `yaml:"keys"`
	BaseURL  string   `yaml:"base_url"`
	Endpoint string   `yaml:"endpoint"`
	Thinking bool     `yaml:"thinking"`
	Model    string   `yaml:"model"`  // single model (backwards compat)
	Models   []string `yaml:"models"` // multiple models (preferred)
}

// AgentConfig holds agent behavior configuration.
type AgentConfig struct {
	Autonomy     string `yaml:"autonomy"`      // interactive|semi|full
	TickInterval string `yaml:"tick_interval"` // 30s, 1m
	IdleSleep    string `yaml:"idle_sleep"`    // 5m
	MaxTurns     int    `yaml:"max_turns"`
}

// ContextConfig holds context window management.
type ContextConfig struct {
	MaxTokens    int     `yaml:"max_tokens"`
	CompactAt    float64 `yaml:"compact_at"`
	ReserveRatio float64  `yaml:"reserve_ratio"`
}

// PermissionConfig holds permission rules.
type PermissionConfig struct {
	Mode           string   `yaml:"mode"` // default|auto|strict|plan
	Rules          []Rule   `yaml:"rules"`
	AdditionalDirs []string `yaml:"additional_dirs"`
}

// Rule is a permission rule.
type Rule struct {
	Allow string `yaml:"allow,omitempty"`
	Deny  string `yaml:"deny,omitempty"`
	Ask   string `yaml:"ask,omitempty"`
}

// TriggersConfig holds remote trigger configuration.
type TriggersConfig struct {
	Wechat  WechatConfig  `yaml:"wechat"`
	Webhook WebhookConfig `yaml:"webhook"`
}

// WechatConfig holds WeChat integration config.
type WechatConfig struct {
	Token     string   `yaml:"token"`
	AppID     string   `yaml:"app_id"`
	Whitelist []string `yaml:"whitelist"`
}

// WebhookConfig holds webhook listener config.
type WebhookConfig struct {
	Listen    string `yaml:"listen"`
	AuthToken string `yaml:"auth_token"`
}

// PluginsConfig holds plugin system configuration.
type PluginsConfig struct {
	Enabled     []string     `yaml:"enabled"`
	MCPServers  []MCPServer  `yaml:"mcp_servers"`
}

// MCPServer is an MCP server connection config.
type MCPServer struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
}

// LoggingConfig holds logging and observability configuration.
type LoggingConfig struct {
	Level        string `yaml:"level"` // debug|info|warn|error
	Audit        bool   `yaml:"audit"`
	OtelEndpoint string `yaml:"otel_endpoint"`
}

// Duration parses a duration string, supporting both Go and human formats.
func Duration(s string) (time.Duration, error) {
	mapping := map[string]time.Duration{
		"30s": 30 * time.Second,
		"1m":  time.Minute,
		"5m":  5 * time.Minute,
		"10m": 10 * time.Minute,
		"30m": 30 * time.Minute,
		"1h":  time.Hour,
	}
	if d, ok := mapping[s]; ok {
		return d, nil
	}
	return time.ParseDuration(s)
}

// Defaults returns a Config with sensible defaults.
func Defaults() Config {
	return Config{
		Version: 1,
		Model: ModelConfig{
			Default:  "claude-sonnet-4-6",
			Fallback: []string{"claude-sonnet-4-6", "gpt-4o"},
			Providers: map[string]ProviderConfig{
				"ollama": {BaseURL: "http://localhost:11434"},
			},
		},
		Agent: AgentConfig{
			Autonomy:     "interactive",
			TickInterval: "30s",
			IdleSleep:    "5m",
			MaxTurns:     100,
		},
		Context: ContextConfig{
			MaxTokens:    200000,
			CompactAt:    0.85,
			ReserveRatio: 0.15,
		},
		Permission: PermissionConfig{
			Mode: "default",
			Rules: []Rule{
				{Allow: "Bash(git:*)"},
				{Deny: "Bash(rm -rf *)"},
			},
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}
}

// Load loads configuration from standard locations with priority merging.
// Priority: CLI args (not handled here) > env > project > user > defaults
func Load(projectPath, userPath string) (*Config, error) {
	cfg := Defaults()

	// Layer 4: User config
	if userPath != "" {
		if err := mergeFile(&cfg, userPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("user config: %w", err)
		}
	}

	// Layer 3: Project config
	if projectPath != "" {
		if err := mergeFile(&cfg, projectPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("project config: %w", err)
		}
	}

	// Layer 2: Environment variables
	applyEnvOverrides(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// mergeFile merges a YAML config file into cfg.
func mergeFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	expanded := expandEnvVars(string(data))
	var fileCfg Config
	if err := yaml.Unmarshal([]byte(expanded), &fileCfg); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	merge(cfg, fileCfg)
	return nil
}

// merge performs a shallow merge of non-zero fields from src into dst.
func merge(dst *Config, src Config) {
	if src.Version != 0 {
		dst.Version = src.Version
	}
	if src.Model.Default != "" {
		dst.Model.Default = src.Model.Default
	}
	if len(src.Model.Fallback) > 0 {
		dst.Model.Fallback = src.Model.Fallback
	}
	if src.Model.Providers != nil {
		if dst.Model.Providers == nil {
			dst.Model.Providers = make(map[string]ProviderConfig)
		}
		for k, v := range src.Model.Providers {
			dst.Model.Providers[k] = v
		}
	}
	if src.Agent.Autonomy != "" {
		dst.Agent.Autonomy = src.Agent.Autonomy
	}
	if src.Agent.TickInterval != "" {
		dst.Agent.TickInterval = src.Agent.TickInterval
	}
	if src.Agent.MaxTurns != 0 {
		dst.Agent.MaxTurns = src.Agent.MaxTurns
	}
	if src.Context.MaxTokens != 0 {
		dst.Context.MaxTokens = src.Context.MaxTokens
	}
	if src.Context.CompactAt != 0 {
		dst.Context.CompactAt = src.Context.CompactAt
	}
	if src.Permission.Mode != "" {
		dst.Permission.Mode = src.Permission.Mode
	}
	if len(src.Permission.Rules) > 0 {
		dst.Permission.Rules = src.Permission.Rules
	}
	if src.Logging.Level != "" {
		dst.Logging.Level = src.Logging.Level
	}
	if src.Logging.OtelEndpoint != "" {
		dst.Logging.OtelEndpoint = src.Logging.OtelEndpoint
	}
	// Triggers merge
	if src.Triggers.Wechat.Token != "" {
		dst.Triggers.Wechat = src.Triggers.Wechat
	}
	if src.Triggers.Webhook.Listen != "" {
		dst.Triggers.Webhook = src.Triggers.Webhook
	}
	// Cron merge
	if src.Cron.Enabled {
		dst.Cron.Enabled = true
	}
	if len(src.Cron.Jobs) > 0 {
		dst.Cron.Jobs = src.Cron.Jobs
	}
	// Channels merge
	if src.Channels.Weixin.Enabled {
		dst.Channels.Weixin = src.Channels.Weixin
	}
	// Plugins merge
	if len(src.Plugins.Enabled) > 0 {
		dst.Plugins.Enabled = src.Plugins.Enabled
	}
	if len(src.Plugins.MCPServers) > 0 {
		dst.Plugins.MCPServers = src.Plugins.MCPServers
	}
}

var envVarRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// expandEnvVars replaces ${VAR_NAME} with environment variable values.
func expandEnvVars(s string) string {
	return envVarRe.ReplaceAllStringFunc(s, func(match string) string {
		key := match[2 : len(match)-1]
		return os.Getenv(key)
	})
}

// applyEnvOverrides applies GCLAW_* environment variable overrides.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("GCLAW_MODEL"); v != "" {
		cfg.Model.Default = v
	}
	if v := os.Getenv("GCLAW_AUTONOMY"); v != "" {
		cfg.Agent.Autonomy = v
	}
	if v := os.Getenv("GCLAW_PERMISSION_MODE"); v != "" {
		cfg.Permission.Mode = v
	}
	if v := os.Getenv("GCLAW_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
}

// validate checks configuration values.
func validate(cfg *Config) error {
	validModes := map[string]bool{
		"interactive": true, "semi": true, "full": true,
	}
	if !validModes[cfg.Agent.Autonomy] {
		return fmt.Errorf("agent.autonomy: must be interactive|semi|full, got %q", cfg.Agent.Autonomy)
	}

	validPerm := map[string]bool{
		"default": true, "auto": true, "strict": true, "plan": true,
	}
	if !validPerm[cfg.Permission.Mode] {
		return fmt.Errorf("permission.mode: must be default|auto|strict|plan, got %q", cfg.Permission.Mode)
	}

	if cfg.Context.MaxTokens < 1000 {
		return fmt.Errorf("context.max_tokens: must be >= 1000, got %d", cfg.Context.MaxTokens)
	}
	if cfg.Context.CompactAt <= 0 || cfg.Context.CompactAt > 1 {
		return fmt.Errorf("context.compact_at: must be between 0-1, got %f", cfg.Context.CompactAt)
	}
	if cfg.Agent.MaxTurns < 1 {
		return fmt.Errorf("agent.max_turns: must be >= 1, got %d", cfg.Agent.MaxTurns)
	}

	return nil
}

// FindProjectConfig locates .gclaw/config.yaml by walking up from dir.
func FindProjectConfig(dir string) string {
	for {
		path := filepath.Join(dir, ".gclaw", "config.yaml")
		if _, err := os.Stat(path); err == nil {
			return path
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// UserConfigPath returns the path to the user-level config file.
func UserConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gclaw", "config.yaml"), nil
}

// SanitizedKey masks an API key for display.
func SanitizedKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "-****-" + key[len(key)-4:]
}

// expandEnvVarsForList expands env vars in a string list.
func expandEnvVarsForList(items []string) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = expandEnvVars(s)
	}
	return out
}

// MaskKeys returns provider configs with API keys redacted.
func MaskKeys(providers map[string]ProviderConfig) map[string]ProviderConfig {
	masked := make(map[string]ProviderConfig, len(providers))
	for k, v := range providers {
		keys := make([]string, len(v.Keys))
		for i, key := range v.Keys {
			keys[i] = SanitizedKey(key)
		}
		masked[k] = ProviderConfig{
			Keys:     keys,
			BaseURL:  v.BaseURL,
			Endpoint: v.Endpoint,
			Thinking: v.Thinking,
			Model:    v.Model,
			Models:   v.Models,
		}
	}
	return masked
}

// ValidateRuleSet ensures no contradictory rules.
func ValidateRuleSet(rules []Rule) error {
	for _, r := range rules {
		count := 0
		if r.Allow != "" {
			count++
		}
		if r.Deny != "" {
			count++
		}
		if r.Ask != "" {
			count++
		}
		if count != 1 {
			return fmt.Errorf("each rule must have exactly one of: allow, deny, ask")
		}
	}
	return nil
}

// GetProviderKeys expands env vars in provider API keys.
func GetProviderKeys(p ProviderConfig) []string {
	return expandEnvVarsForList(p.Keys)
}

// FindStringInSlice checks if a string exists in a slice.
func FindStringInSlice(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// TrimPathSeparator normalizes path separators for the current OS.
func TrimPathSeparator(path string) string {
	return strings.TrimRight(path, string(filepath.Separator))
}
