package perm

import (
	"testing"
)

func TestPermAutoMode(t *testing.T) {
	c := NewChecker(ModeAuto, nil)
	if err := c.Check("Bash", map[string]any{"command": "rm -rf /"}); err != nil {
		t.Errorf("auto mode should allow all: %v", err)
	}
}

func TestPermPlanMode(t *testing.T) {
	c := NewChecker(ModePlan, nil)
	if err := c.Check("Bash", map[string]any{}); err == nil {
		t.Error("plan mode should deny non-read operations")
	}
	if err := c.Check("ReadFile", map[string]any{}); err == nil {
		t.Error("plan mode should deny all write-adjacent ops")
	}
}

func TestPermDefaultRules(t *testing.T) {
	rules := []Rule{
		{Action: "allow", Pattern: "Bash(git:*)"},
		{Action: "deny", Pattern: "Bash(rm -rf *)"},
		{Action: "allow", Pattern: "ReadFile"},
	}
	c := NewChecker(ModeDefault, rules)

	// git commands allowed (uses git: namespace convention)
	if err := c.Check("Bash", map[string]any{"command": "git:status"}); err != nil {
		t.Errorf("git:status should be allowed: %v", err)
	}

	// rm -rf denied
	if err := c.Check("Bash", map[string]any{"command": "rm -rf /"}); err == nil {
		t.Error("rm -rf should be denied")
	}

	// ReadFile allowed (no params — exact match)
	if err := c.Check("ReadFile", map[string]any{}); err != nil {
		t.Errorf("ReadFile should be allowed: %v", err)
	}

	// Unmatched tool should be allowed (default allow)
	if err := c.Check("Glob", map[string]any{}); err != nil {
		t.Errorf("unmatched tool should be allowed: %v", err)
	}
}

func TestPermStrictMode(t *testing.T) {
	rules := []Rule{
		{Action: "ask", Pattern: "WriteFile"},
	}
	c := NewChecker(ModeStrict, rules)

	// ask rule in strict mode should deny
	if err := c.Check("WriteFile", map[string]any{}); err == nil {
		t.Error("ask rule in strict mode should deny")
	}
}

func TestPermDefaultAskMode(t *testing.T) {
	rules := []Rule{
		{Action: "ask", Pattern: "WriteFile"},
	}
	c := NewChecker(ModeDefault, rules)

	// ask rule in default mode should allow (non-blocking prompt)
	if err := c.Check("WriteFile", map[string]any{}); err != nil {
		t.Errorf("ask rule in default mode should allow: %v", err)
	}
}

func TestPermWildcard(t *testing.T) {
	rules := []Rule{
		{Action: "allow", Pattern: "**"},
	}
	c := NewChecker(ModeStrict, rules)

	// ** should match all
	if err := c.Check("Bash", map[string]any{"command": "rm -rf /"}); err != nil {
		t.Errorf("** should allow all: %v", err)
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"**", "anything", true},
		{"**", "Bash(git:push)", true},
		{"Bash", "Bash", true},
		{"Bash", "bash", false},
		{"Bash(*)", "Bash(git status)", true},
		{"Bash(*)", "Bash", false},
		{"Bash(**)", "Bash(git:push:force)", true},
		{"Bash(git:*)", "Bash(git:status)", true},
		{"Bash(git:*)", "Bash(rm -rf)", false},
		{"ReadFile", "ReadFile", true},
		{"ReadFile", "WriteFile", false},
	}

	for _, tt := range tests {
		got := match(tt.pattern, tt.value)
		if got != tt.want {
			t.Errorf("match(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
		}
	}
}
