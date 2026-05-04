package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	mcpclient "github.com/openclaw/gclaw/internal/mcp"
	"github.com/openclaw/gclaw/internal/tool"
)

// ManagerRef holds a reference to the MCP manager, set by main.
var ManagerRef *mcpclient.Manager

// Compile-time interface compliance checks.
var (
	_ tool.Tool = (*mcpListServersTool)(nil)
	_ tool.Tool = (*mcpDiscoverTool)(nil)
	_ tool.Tool = (*mcpCallTool)(nil)
)

// ---- mcp_list_servers ----

type mcpListServersTool struct{}

func (t *mcpListServersTool) Name() string        { return "mcp_list_servers" }
func (t *mcpListServersTool) Toolset() string     { return "mcp" }
func (t *mcpListServersTool) ConcurrencySafe() bool { return false }
func (t *mcpListServersTool) RequiresApproval(map[string]any) bool { return true }

func (t *mcpListServersTool) Description() string {
	return "List all configured MCP servers and their connection status."
}

func (t *mcpListServersTool) Check() bool {
	return ManagerRef != nil
}

func (t *mcpListServersTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type:       "object",
		Properties: map[string]tool.Property{},
	}
}

func (t *mcpListServersTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	if ManagerRef == nil {
		return tool.ToolResult{
			Content: "Error: MCP manager not available",
			IsError: true,
		}, nil
	}

	servers := ManagerRef.ListServers()
	status := ManagerRef.ServerStatus()

	if len(servers) == 0 {
		return tool.ToolResult{Content: "No MCP servers configured."}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d MCP server(s) configured:\n\n", len(servers)))

	// Sort for consistent output
	sorted := make([]string, len(servers))
	copy(sorted, servers)
	sort.Strings(sorted)

	for _, name := range sorted {
		connected := status[name]
		state := "disconnected"
		if connected {
			state = "connected"
		}
		tools, _ := ManagerRef.ListTools(name)
		toolCount := len(tools)
		sb.WriteString(fmt.Sprintf("- %s [%s] (%d tools discovered)\n", name, state, toolCount))
	}

	return tool.ToolResult{Content: sb.String()}, nil
}

func init() {
	tool.GlobalRegistry.Register(&mcpListServersTool{})
}
