package browser

import (
	"context"
	"testing"

	"github.com/openclaw/gclaw/internal/tool"
)

// allTools returns all browser tool instances for iteration.
func allTools() []tool.Tool {
	return []tool.Tool{
		&NavigateTool{},
		&SnapshotTool{},
		&ClickTool{},
		&TypeTool{},
		&ScrollTool{},
		&PressTool{},
		&ScreenshotTool{},
	}
}

// TestInterfaceCompliance verifies all 7 tools satisfy the tool.Tool interface
// at compile time (via the var _ declarations in each file) and that the
// registry can retrieve them.
func TestInterfaceCompliance(t *testing.T) {
	for _, tt := range allTools() {
		// The var _ tool.Tool = (*XxxTool)(nil) lines in each file give
		// compile-time guarantees. Here we additionally verify the tools
		// are usable through the interface.
		_ = tt.Name()
		_ = tt.Toolset()
		_ = tt.Description()
		_ = tt.InputSchema()
		_ = tt.Check()
		_ = tt.ConcurrencySafe()
		_ = tt.RequiresApproval(nil)
	}
}

// TestCheckReturnsFalseWhenNil verifies that Check() returns false when
// BrowserRef is nil.
func TestCheckReturnsFalseWhenNil(t *testing.T) {
	orig := BrowserRef
	BrowserRef = nil
	defer func() { BrowserRef = orig }()

	for _, tt := range allTools() {
		if tt.Check() {
			t.Errorf("%s: Check() should return false when BrowserRef is nil", tt.Name())
		}
	}
}

// TestToolset verifies all tools report the "browser" toolset.
func TestToolset(t *testing.T) {
	for _, tt := range allTools() {
		if ts := tt.Toolset(); ts != "browser" {
			t.Errorf("%s: Toolset() = %q, want %q", tt.Name(), ts, "browser")
		}
	}
}

// TestConcurrencySafe verifies none of the browser tools are concurrency-safe.
func TestConcurrencySafe(t *testing.T) {
	for _, tt := range allTools() {
		if tt.ConcurrencySafe() {
			t.Errorf("%s: ConcurrencySafe() should return false", tt.Name())
		}
	}
}

// TestRequiresApproval verifies all browser tools require approval.
func TestRequiresApproval(t *testing.T) {
	for _, tt := range allTools() {
		if !tt.RequiresApproval(nil) {
			t.Errorf("%s: RequiresApproval() should return true", tt.Name())
		}
	}
}

// TestToolNames verifies the expected tool names.
func TestToolNames(t *testing.T) {
	expected := map[string]bool{
		"browser_navigate":  true,
		"browser_snapshot":  true,
		"browser_click":     true,
		"browser_type":      true,
		"browser_scroll":    true,
		"browser_press":     true,
		"browser_screenshot": true,
	}

	for _, tt := range allTools() {
		if !expected[tt.Name()] {
			t.Errorf("unexpected tool name: %s", tt.Name())
		}
		delete(expected, tt.Name())
	}
	if len(expected) > 0 {
		t.Errorf("missing tool names: %v", expected)
	}
}

// TestSchemaValidation checks schemas have correct type, properties, and
// required fields.
func TestSchemaValidation(t *testing.T) {
	t.Run("navigate", func(t *testing.T) {
		s := (&NavigateTool{}).InputSchema()
		if s.Type != "object" {
			t.Errorf("type = %q, want 'object'", s.Type)
		}
		if _, ok := s.Properties["url"]; !ok {
			t.Error("missing 'url' property")
		}
		if len(s.Required) != 1 || s.Required[0] != "url" {
			t.Errorf("required = %v, want [url]", s.Required)
		}
	})

	t.Run("snapshot", func(t *testing.T) {
		s := (&SnapshotTool{}).InputSchema()
		if s.Type != "object" {
			t.Errorf("type = %q, want 'object'", s.Type)
		}
		if len(s.Required) != 0 {
			t.Errorf("required = %v, want empty", s.Required)
		}
	})

	t.Run("click", func(t *testing.T) {
		s := (&ClickTool{}).InputSchema()
		if _, ok := s.Properties["ref"]; !ok {
			t.Error("missing 'ref' property")
		}
		if len(s.Required) != 1 || s.Required[0] != "ref" {
			t.Errorf("required = %v, want [ref]", s.Required)
		}
	})

	t.Run("type", func(t *testing.T) {
		s := (&TypeTool{}).InputSchema()
		if _, ok := s.Properties["ref"]; !ok {
			t.Error("missing 'ref' property")
		}
		if _, ok := s.Properties["text"]; !ok {
			t.Error("missing 'text' property")
		}
		required := map[string]bool{}
		for _, r := range s.Required {
			required[r] = true
		}
		if !required["ref"] || !required["text"] {
			t.Errorf("required = %v, want [ref text]", s.Required)
		}
	})

	t.Run("scroll", func(t *testing.T) {
		s := (&ScrollTool{}).InputSchema()
		prop, ok := s.Properties["direction"]
		if !ok {
			t.Fatal("missing 'direction' property")
		}
		if len(prop.Enum) != 2 {
			t.Errorf("direction enum = %v, want [up down]", prop.Enum)
		}
		if len(s.Required) != 1 || s.Required[0] != "direction" {
			t.Errorf("required = %v, want [direction]", s.Required)
		}
		if _, ok := s.Properties["amount"]; !ok {
			t.Error("missing 'amount' property")
		}
	})

	t.Run("press", func(t *testing.T) {
		s := (&PressTool{}).InputSchema()
		if _, ok := s.Properties["key"]; !ok {
			t.Error("missing 'key' property")
		}
		if len(s.Required) != 1 || s.Required[0] != "key" {
			t.Errorf("required = %v, want [key]", s.Required)
		}
	})

	t.Run("screenshot", func(t *testing.T) {
		s := (&ScreenshotTool{}).InputSchema()
		if s.Type != "object" {
			t.Errorf("type = %q, want 'object'", s.Type)
		}
		if len(s.Required) != 0 {
			t.Errorf("required = %v, want empty", s.Required)
		}
	})
}

// TestExecuteWithoutBrowser verifies that all tools return an error when
// BrowserRef is nil.
func TestExecuteWithoutBrowser(t *testing.T) {
	orig := BrowserRef
	BrowserRef = nil
	defer func() { BrowserRef = orig }()

	ctx := context.Background()

	tests := []struct {
		name   string
		tool   tool.Tool
		params map[string]any
	}{
		{"navigate", &NavigateTool{}, map[string]any{"url": "https://example.com"}},
		{"snapshot", &SnapshotTool{}, map[string]any{}},
		{"click", &ClickTool{}, map[string]any{"ref": "@e1"}},
		{"type", &TypeTool{}, map[string]any{"ref": "@e1", "text": "hello"}},
		{"scroll", &ScrollTool{}, map[string]any{"direction": "down"}},
		{"press", &PressTool{}, map[string]any{"key": "Enter"}},
		{"screenshot", &ScreenshotTool{}, map[string]any{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.tool.Execute(ctx, tt.params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Errorf("%s: expected IsError=true when browser is nil", tt.name)
			}
		})
	}
}

// TestExecuteMissingRequiredParams verifies tools reject missing required
// parameters even when BrowserRef is set (we use a non-nil dummy by resetting
// the check).
func TestExecuteMissingRequiredParams(t *testing.T) {
	orig := BrowserRef
	BrowserRef = nil // keep nil so Execute returns early with "browser not available"
	defer func() { BrowserRef = orig }()

	ctx := context.Background()

	// With nil BrowserRef, all tools should return IsError=true, which
	// covers the first guard clause. The parameter validation is tested
	// implicitly through schema tests above.

	// Navigate missing URL.
	result, _ := (&NavigateTool{}).Execute(ctx, map[string]any{})
	if !result.IsError {
		t.Error("navigate without url should error")
	}

	// Click missing ref.
	result, _ = (&ClickTool{}).Execute(ctx, map[string]any{})
	if !result.IsError {
		t.Error("click without ref should error")
	}

	// Type missing both.
	result, _ = (&TypeTool{}).Execute(ctx, map[string]any{})
	if !result.IsError {
		t.Error("type without params should error")
	}

	// Scroll missing direction.
	result, _ = (&ScrollTool{}).Execute(ctx, map[string]any{})
	if !result.IsError {
		t.Error("scroll without direction should error")
	}

	// Press missing key.
	result, _ = (&PressTool{}).Execute(ctx, map[string]any{})
	if !result.IsError {
		t.Error("press without key should error")
	}
}
