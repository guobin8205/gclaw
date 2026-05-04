package mcp

import (
	"context"
	"strings"
	"testing"

	mcpclient "github.com/openclaw/gclaw/internal/mcp"
	"github.com/openclaw/gclaw/internal/tool"
)

// TestInterfaceCompliance verifies all three MCP tools implement tool.Tool.
func TestInterfaceCompliance(t *testing.T) {
	var _ tool.Tool = (*mcpListServersTool)(nil)
	var _ tool.Tool = (*mcpDiscoverTool)(nil)
	var _ tool.Tool = (*mcpCallTool)(nil)
}

// TestCheckNil verifies that Check() returns false when ManagerRef is nil.
func TestCheckNil(t *testing.T) {
	orig := ManagerRef
	defer func() { ManagerRef = orig }()

	ManagerRef = nil

	listTool := &mcpListServersTool{}
	discoverTool := &mcpDiscoverTool{}
	callTool := &mcpCallTool{}

	if listTool.Check() {
		t.Error("mcpListServersTool.Check() should return false when ManagerRef is nil")
	}
	if discoverTool.Check() {
		t.Error("mcpDiscoverTool.Check() should return false when ManagerRef is nil")
	}
	if callTool.Check() {
		t.Error("mcpCallTool.Check() should return false when ManagerRef is nil")
	}
}

// TestCheckSet verifies that Check() returns true when ManagerRef is set.
func TestCheckSet(t *testing.T) {
	orig := ManagerRef
	defer func() { ManagerRef = orig }()

	ManagerRef = mcpclient.NewManager()

	listTool := &mcpListServersTool{}
	discoverTool := &mcpDiscoverTool{}
	callTool := &mcpCallTool{}

	if !listTool.Check() {
		t.Error("mcpListServersTool.Check() should return true when ManagerRef is set")
	}
	if !discoverTool.Check() {
		t.Error("mcpDiscoverTool.Check() should return true when ManagerRef is set")
	}
	if !callTool.Check() {
		t.Error("mcpCallTool.Check() should return true when ManagerRef is set")
	}
}

// TestMetadata validates Name, Toolset, Description, ConcurrencySafe, RequiresApproval.
func TestMetadata(t *testing.T) {
	tests := []struct {
		name              string
		t                 tool.Tool
		wantName          string
		wantToolset       string
		wantConcurrent    bool
		wantApproval      bool
	}{
		{
			name:           "mcp_list_servers",
			t:              &mcpListServersTool{},
			wantName:       "mcp_list_servers",
			wantToolset:    "mcp",
			wantConcurrent: false,
			wantApproval:   true,
		},
		{
			name:           "mcp_discover",
			t:              &mcpDiscoverTool{},
			wantName:       "mcp_discover",
			wantToolset:    "mcp",
			wantConcurrent: false,
			wantApproval:   true,
		},
		{
			name:           "mcp_call",
			t:              &mcpCallTool{},
			wantName:       "mcp_call",
			wantToolset:    "mcp",
			wantConcurrent: false,
			wantApproval:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.t.Name() != tt.wantName {
				t.Errorf("Name() = %q, want %q", tt.t.Name(), tt.wantName)
			}
			if tt.t.Toolset() != tt.wantToolset {
				t.Errorf("Toolset() = %q, want %q", tt.t.Toolset(), tt.wantToolset)
			}
			if tt.t.ConcurrencySafe() != tt.wantConcurrent {
				t.Errorf("ConcurrencySafe() = %v, want %v", tt.t.ConcurrencySafe(), tt.wantConcurrent)
			}
			if tt.t.RequiresApproval(nil) != tt.wantApproval {
				t.Errorf("RequiresApproval() = %v, want %v", tt.t.RequiresApproval(nil), tt.wantApproval)
			}
			if tt.t.Description() == "" {
				t.Error("Description() should not be empty")
			}
		})
	}
}

// TestSchemaValidation validates InputSchema has correct required fields.
func TestSchemaValidation(t *testing.T) {
	t.Run("mcp_list_servers", func(t *testing.T) {
		schema := (&mcpListServersTool{}).InputSchema()
		if schema.Type != "object" {
			t.Errorf("schema type = %q, want 'object'", schema.Type)
		}
	})

	t.Run("mcp_discover", func(t *testing.T) {
		schema := (&mcpDiscoverTool{}).InputSchema()
		if schema.Type != "object" {
			t.Errorf("schema type = %q, want 'object'", schema.Type)
		}
		if _, ok := schema.Properties["server"]; !ok {
			t.Error("missing 'server' property")
		}
		// server is optional
		if len(schema.Required) != 0 {
			t.Errorf("expected no required fields, got %v", schema.Required)
		}
	})

	t.Run("mcp_call", func(t *testing.T) {
		schema := (&mcpCallTool{}).InputSchema()
		if schema.Type != "object" {
			t.Errorf("schema type = %q, want 'object'", schema.Type)
		}
		if _, ok := schema.Properties["server"]; !ok {
			t.Error("missing 'server' property")
		}
		if _, ok := schema.Properties["tool"]; !ok {
			t.Error("missing 'tool' property")
		}
		if _, ok := schema.Properties["arguments"]; !ok {
			t.Error("missing 'arguments' property")
		}
		// server and tool are required
		if len(schema.Required) != 2 {
			t.Errorf("expected 2 required fields, got %d", len(schema.Required))
		}
		requiredMap := map[string]bool{}
		for _, r := range schema.Required {
			requiredMap[r] = true
		}
		if !requiredMap["server"] {
			t.Error("'server' should be required")
		}
		if !requiredMap["tool"] {
			t.Error("'tool' should be required")
		}
	})
}

// TestExecuteNoManager verifies Execute returns error when ManagerRef is nil.
func TestExecuteNoManager(t *testing.T) {
	orig := ManagerRef
	defer func() { ManagerRef = orig }()

	ManagerRef = nil
	ctx := context.Background()

	tests := []struct {
		name   string
		t      tool.Tool
		params map[string]any
	}{
		{"mcp_list_servers", &mcpListServersTool{}, map[string]any{}},
		{"mcp_discover", &mcpDiscoverTool{}, map[string]any{}},
		{"mcp_call", &mcpCallTool{}, map[string]any{"server": "s", "tool": "t"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.t.Execute(ctx, tt.params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Error("expected error result when ManagerRef is nil")
			}
			if !strings.Contains(result.Content, "not available") {
				t.Errorf("expected 'not available' message, got: %s", result.Content)
			}
		})
	}
}

// TestExecuteListServersEmpty verifies list output with empty manager.
func TestExecuteListServersEmpty(t *testing.T) {
	orig := ManagerRef
	defer func() { ManagerRef = orig }()

	ManagerRef = mcpclient.NewManager()

	result, err := (&mcpListServersTool{}).Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "No MCP servers") {
		t.Errorf("expected 'No MCP servers' message, got: %s", result.Content)
	}
}

// TestExecuteCallMissingParams verifies mcp_call with missing required params.
func TestExecuteCallMissingParams(t *testing.T) {
	orig := ManagerRef
	defer func() { ManagerRef = orig }()

	ManagerRef = mcpclient.NewManager()
	ctx := context.Background()

	// Missing server
	result, err := (&mcpCallTool{}).Execute(ctx, map[string]any{"tool": "t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing server")
	}
	if !strings.Contains(result.Content, "server") {
		t.Errorf("expected 'server' in error, got: %s", result.Content)
	}

	// Missing tool
	result, err = (&mcpCallTool{}).Execute(ctx, map[string]any{"server": "s"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing tool")
	}
	if !strings.Contains(result.Content, "tool") {
		t.Errorf("expected 'tool' in error, got: %s", result.Content)
	}
}

// TestExecuteDiscoverNotFound verifies discover with unknown server name.
func TestExecuteDiscoverNotFound(t *testing.T) {
	orig := ManagerRef
	defer func() { ManagerRef = orig }()

	ManagerRef = mcpclient.NewManager()

	result, err := (&mcpDiscoverTool{}).Execute(context.Background(), map[string]any{
		"server": "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for unknown server")
	}
	if !strings.Contains(result.Content, "not found") {
		t.Errorf("expected 'not found' in error, got: %s", result.Content)
	}
}
