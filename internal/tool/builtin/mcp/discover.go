package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	mcpclient "github.com/openclaw/gclaw/internal/mcp"
	"github.com/openclaw/gclaw/internal/tool"
)

// ---- mcp_discover ----

type mcpDiscoverTool struct{}

func (t *mcpDiscoverTool) Name() string        { return "mcp_discover" }
func (t *mcpDiscoverTool) Toolset() string     { return "mcp" }
func (t *mcpDiscoverTool) ConcurrencySafe() bool { return false }
func (t *mcpDiscoverTool) RequiresApproval(map[string]any) bool { return true }

func (t *mcpDiscoverTool) Description() string {
	return "Discover tools available on MCP servers. Optionally specify a server name to discover tools from a single server."
}

func (t *mcpDiscoverTool) Check() bool {
	return ManagerRef != nil
}

func (t *mcpDiscoverTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"server": {
				Type:        "string",
				Description: "Optional server name. If empty, discovers tools from all servers.",
			},
		},
	}
}

func (t *mcpDiscoverTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	if ManagerRef == nil {
		return tool.ToolResult{
			Content: "Error: MCP manager not available",
			IsError: true,
		}, nil
	}

	server, _ := params["server"].(string)

	if server != "" {
		// Discover from a single server
		client, ok := ManagerRef.GetClient(server)
		if !ok {
			return tool.ToolResult{
				Content: fmt.Sprintf("Error: MCP server %q not found", server),
				IsError: true,
			}, nil
		}
		if err := client.DiscoverTools(ctx); err != nil {
			return tool.ToolResult{
				Content: fmt.Sprintf("Error discovering tools from %q: %v", server, err),
				IsError: true,
			}, nil
		}
		tools := client.ListTools()
		return tool.ToolResult{
			Content: formatToolList(server, tools),
		}, nil
	}

	// Discover from all servers
	servers := ManagerRef.ListServers()
	if len(servers) == 0 {
		return tool.ToolResult{Content: "No MCP servers configured."}, nil
	}

	var sb strings.Builder
	totalTools := 0

	sorted := make([]string, len(servers))
	copy(sorted, servers)
	sort.Strings(sorted)

	for _, name := range sorted {
		client, ok := ManagerRef.GetClient(name)
		if !ok {
			continue
		}
		if err := client.DiscoverTools(ctx); err != nil {
			sb.WriteString(fmt.Sprintf("Server %q: error - %v\n", name, err))
			continue
		}
		tools := client.ListTools()
		totalTools += len(tools)
		sb.WriteString(formatToolList(name, tools))
		sb.WriteString("\n")
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Discovered %d tools from %d server(s):\n\n%s", totalTools, len(sorted), sb.String()),
	}, nil
}

// formatToolList formats discovered tools for display.
func formatToolList(serverName string, tools []mcpclient.Tool) string {
	if len(tools) == 0 {
		return fmt.Sprintf("Server %q: no tools available.", serverName)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Server %q (%d tools):\n", serverName, len(tools)))
	for _, t := range tools {
		desc := t.Description
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", t.Name, desc))
	}
	return sb.String()
}

func init() {
	tool.GlobalRegistry.Register(&mcpDiscoverTool{})
}
