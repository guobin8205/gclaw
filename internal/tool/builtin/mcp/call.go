package mcp

import (
	"context"
	"fmt"

	"github.com/openclaw/gclaw/internal/tool"
)

// ---- mcp_call ----

type mcpCallTool struct{}

func (t *mcpCallTool) Name() string        { return "mcp_call" }
func (t *mcpCallTool) Toolset() string     { return "mcp" }
func (t *mcpCallTool) ConcurrencySafe() bool { return false }
func (t *mcpCallTool) RequiresApproval(map[string]any) bool { return true }

func (t *mcpCallTool) Description() string {
	return "Call a tool on an MCP server. Requires server name, tool name, and optional arguments."
}

func (t *mcpCallTool) Check() bool {
	return ManagerRef != nil
}

func (t *mcpCallTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"server": {
				Type:        "string",
				Description: "Name of the MCP server to call the tool on",
			},
			"tool": {
				Type:        "string",
				Description: "Name of the tool to call",
			},
			"arguments": {
				Type:        "object",
				Description: "Optional arguments to pass to the tool",
			},
		},
		Required: []string{"server", "tool"},
	}
}

func (t *mcpCallTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	if ManagerRef == nil {
		return tool.ToolResult{
			Content: "Error: MCP manager not available",
			IsError: true,
		}, nil
	}

	server, _ := params["server"].(string)
	if server == "" {
		return tool.ToolResult{
			Content: "Error: server parameter is required",
			IsError: true,
		}, nil
	}

	toolName, _ := params["tool"].(string)
	if toolName == "" {
		return tool.ToolResult{
			Content: "Error: tool parameter is required",
			IsError: true,
		}, nil
	}

	// Extract arguments (optional)
	var args map[string]any
	if a, ok := params["arguments"].(map[string]any); ok {
		args = a
	}

	result, err := ManagerRef.CallTool(ctx, server, toolName, args)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error calling %s on %s: %v", toolName, server, err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{
		Content: result,
	}, nil
}

func init() {
	tool.GlobalRegistry.Register(&mcpCallTool{})
}
