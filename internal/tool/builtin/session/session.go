package session

import (
	"context"
	"fmt"

	"github.com/openclaw/gclaw/internal/session"
	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface compliance check.
var _ tool.Tool = (*SessionSearchTool)(nil)

// StoreRef holds a reference to the session store, set by main.go.
var StoreRef session.Store

// SessionSearchTool provides full-text search across conversation history.
type SessionSearchTool struct{}

func (t *SessionSearchTool) Name() string                    { return "SessionSearch" }
func (t *SessionSearchTool) Toolset() string                 { return "session" }
func (t *SessionSearchTool) ConcurrencySafe() bool           { return true }
func (t *SessionSearchTool) RequiresApproval(map[string]any) bool { return false }

func (t *SessionSearchTool) Description() string {
	return "Search conversation history for relevant past discussions and context using full-text search."
}

func (t *SessionSearchTool) Check() bool {
	return StoreRef != nil
}

func (t *SessionSearchTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"query": {
				Type:        "string",
				Description: "Search query to find matching conversations",
			},
			"limit": {
				Type:        "integer",
				Description: "Maximum number of results to return (default: 5)",
			},
		},
		Required: []string{"query"},
	}
}

func (t *SessionSearchTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	if StoreRef == nil {
		return tool.ToolResult{
			Content: "Error: session store not available",
			IsError: true,
		}, nil
	}

	query, ok := params["query"].(string)
	if !ok || query == "" {
		return tool.ToolResult{
			Content: "Error: query parameter is required",
			IsError: true,
		}, nil
	}

	limit := 5
	if l, ok := params["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
	}

	results, err := StoreRef.Search(query, limit)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error searching sessions: %v", err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{
		Content: formatResults(results),
	}, nil
}

// formatResults renders search results into a human-readable string.
func formatResults(results []session.SearchResult) string {
	if len(results) == 0 {
		return "Found 0 matching sessions."
	}

	out := fmt.Sprintf("Found %d matching sessions:\n\n", len(results))
	for i, r := range results {
		out += fmt.Sprintf("[%d] %s (%s, %s)\n", i+1, r.SessionID, r.Role, r.Timestamp.Format("2006-01-02 15:04:05"))
		snippet := r.Content
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		out += snippet + "\n\n"
	}
	return out
}

func init() {
	tool.GlobalRegistry.Register(&SessionSearchTool{})
}
