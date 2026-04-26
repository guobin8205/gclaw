package perm

import (
	"fmt"
	"strings"
)

// Mode defines the permission enforcement level.
type Mode string

const (
	ModeDefault Mode = "default"
	ModeAuto    Mode = "auto"
	ModeStrict  Mode = "strict"
	ModePlan    Mode = "plan"
)

// Rule represents an allow/deny/ask rule with glob matching.
type Rule struct {
	Action  string // "allow", "deny", "ask"
	Pattern string // glob pattern, e.g. "Bash(git:*)"
}

// Checker implements the onion-model permission chain.
type Checker struct {
	mode  Mode
	rules []Rule
}

// NewChecker creates a permission checker.
func NewChecker(mode Mode, rules []Rule) *Checker {
	return &Checker{mode: mode, rules: rules}
}

// Check verifies if a tool call is permitted.
func (c *Checker) Check(toolName string, params map[string]any) error {
	switch c.mode {
	case ModeAuto:
		return nil
	case ModePlan:
		return fmt.Errorf("plan mode: only read operations allowed, got %s", toolName)
	case ModeDefault, ModeStrict:
		return c.matchRules(toolName, params)
	}
	return nil
}

func (c *Checker) matchRules(toolName string, params map[string]any) error {
	// Build the full tool call expression for pattern matching.
	// E.g., "Bash(git:status)" so that rule "Bash(git:*)" can match.
	expr := toolName
	if len(params) > 0 {
		for _, v := range params {
			if s, ok := v.(string); ok && s != "" {
				expr = toolName + "(" + s + ")"
				break
			}
		}
	}

	// Check explicit rules against both the expression and bare tool name
	for _, r := range c.rules {
		if match(r.Pattern, expr) || match(r.Pattern, toolName) {
			switch r.Action {
			case "allow":
				return nil
			case "deny":
				return fmt.Errorf("denied by rule: %s", r.Pattern)
			case "ask":
				if c.mode == ModeStrict {
					return fmt.Errorf("requires approval: %s", r.Pattern)
				}
				return nil
			}
		}
	}

	return nil
}

// match implements simple glob matching for tool patterns.
// Supported patterns:
//   "**" — match everything
//   "Bash" — exact tool name match (no params)
//   "Bash(*)" — tool name + any single param
//   "Bash(**)" — tool name + any param prefix
//   "Bash(git:*)" — tool name + param with prefix glob
func match(pattern, value string) bool {
	if pattern == "**" {
		return true
	}
	if pattern == value {
		return true
	}
	// Patterns with parentheses: tool name + param expression
	if strings.Contains(pattern, "(") && strings.HasSuffix(pattern, ")") {
		parenIdx := strings.Index(pattern, "(")
		toolPart := pattern[:parenIdx]
		paramPart := pattern[parenIdx+1 : len(pattern)-1]

		// Greedy wildcard: Bash(**) matches any Bash(...)
		if paramPart == "**" {
			return strings.HasPrefix(value, toolPart+"(") && strings.HasSuffix(value, ")")
		}

		if !strings.HasPrefix(value, toolPart+"(") || !strings.HasSuffix(value, ")") {
			return false
		}
		valueParam := value[len(toolPart)+1 : len(value)-1]

		// Single param wildcard
		if paramPart == "*" {
			return valueParam != ""
		}
		// Suffix wildcard: git:* matches git:status, rm -rf * matches rm -rf /
		if strings.HasSuffix(paramPart, "*") && !strings.Contains(paramPart[:len(paramPart)-1], "*") {
			prefix := paramPart[:len(paramPart)-1]
			return strings.HasPrefix(valueParam, prefix)
		}
		// Exact param match
		if paramPart == valueParam {
			return true
		}
	}
	return false
}
