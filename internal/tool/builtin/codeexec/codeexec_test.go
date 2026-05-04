package codeexec

import (
	"context"
	"testing"

	"github.com/openclaw/gclaw/internal/tool"
)

func TestInterfaceCompliance(t *testing.T) {
	// Compile-time check that CodeExecuteTool satisfies tool.Tool
	var _ tool.Tool = &CodeExecuteTool{}
}

func TestCheckAlwaysTrue(t *testing.T) {
	ct := &CodeExecuteTool{}
	if !ct.Check() {
		t.Error("expected Check() to always return true")
	}
}

func TestMetadata(t *testing.T) {
	ct := &CodeExecuteTool{}

	if ct.Name() != "code_execute" {
		t.Errorf("expected name 'code_execute', got %q", ct.Name())
	}
	if ct.Toolset() != "codeexec" {
		t.Errorf("expected toolset 'codeexec', got %q", ct.Toolset())
	}
	if ct.ConcurrencySafe() {
		t.Error("expected ConcurrencySafe() to return false")
	}
	if !ct.RequiresApproval(nil) {
		t.Error("expected RequiresApproval(nil) to return true")
	}
}

func TestSchema(t *testing.T) {
	ct := &CodeExecuteTool{}
	schema := ct.InputSchema()

	if schema.Type != "object" {
		t.Errorf("expected schema type 'object', got %q", schema.Type)
	}

	// Check required fields
	if len(schema.Required) != 2 {
		t.Fatalf("expected 2 required fields, got %d", len(schema.Required))
	}
	requiredMap := map[string]bool{}
	for _, r := range schema.Required {
		requiredMap[r] = true
	}
	if !requiredMap["code"] {
		t.Error("expected 'code' to be required")
	}
	if !requiredMap["language"] {
		t.Error("expected 'language' to be required")
	}

	// Check properties exist
	if _, ok := schema.Properties["code"]; !ok {
		t.Error("expected 'code' property")
	}
	if _, ok := schema.Properties["language"]; !ok {
		t.Error("expected 'language' property")
	}
	if _, ok := schema.Properties["timeout"]; !ok {
		t.Error("expected 'timeout' property")
	}

	// Check language enum
	langProp := schema.Properties["language"]
	if len(langProp.Enum) != 4 {
		t.Fatalf("expected 4 enum values for language, got %d", len(langProp.Enum))
	}
	expectedEnums := map[string]bool{
		"go": true, "python": true, "javascript": true, "shell": true,
	}
	for _, e := range langProp.Enum {
		if !expectedEnums[e] {
			t.Errorf("unexpected enum value %q", e)
		}
	}

	// Check timeout property type
	timeoutProp := schema.Properties["timeout"]
	if timeoutProp.Type != "integer" {
		t.Errorf("expected timeout type 'integer', got %q", timeoutProp.Type)
	}
}

func TestRequiresApprovalAnyParams(t *testing.T) {
	ct := &CodeExecuteTool{}

	// Should always return true regardless of params
	if !ct.RequiresApproval(nil) {
		t.Error("expected RequiresApproval(nil) to return true")
	}
	if !ct.RequiresApproval(map[string]any{"code": "print('hi')", "language": "python"}) {
		t.Error("expected RequiresApproval(params) to return true")
	}
	if !ct.RequiresApproval(map[string]any{}) {
		t.Error("expected RequiresApproval(empty) to return true")
	}
}

func TestMissingCode(t *testing.T) {
	ct := &CodeExecuteTool{}
	result, err := ct.Execute(context.Background(), map[string]any{
		"language": "python",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing code")
	}
}

func TestMissingLanguage(t *testing.T) {
	ct := &CodeExecuteTool{}
	result, err := ct.Execute(context.Background(), map[string]any{
		"code": "print('hi')",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing language")
	}
}
