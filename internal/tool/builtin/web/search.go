package web

import (
	"context"
	"fmt"
	"strings"

	"github.com/openclaw/gclaw/internal/tool"
	"github.com/openclaw/gclaw/internal/websearch"
)

// SearchFactory is the websearch factory, set by main.go.
var SearchFactory *websearch.Factory

// WebSearchTool performs web searches via configured backends.
type WebSearchTool struct{}

func (t *WebSearchTool) Name() string        { return "WebSearch" }
func (t *WebSearchTool) Toolset() string      { return "web" }
func (t *WebSearchTool) ConcurrencySafe() bool { return true }
func (t *WebSearchTool) RequiresApproval(params map[string]any) bool { return false }

func (t *WebSearchTool) Description() string {
	return "Search the web using configured search backends (Tavily, Exa). Returns a list of results with titles, URLs, and descriptions."
}

func (t *WebSearchTool) Check() bool {
	return SearchFactory != nil
}

func (t *WebSearchTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"query":       {Type: "string", Description: "The search query"},
			"max_results": {Type: "integer", Description: "Maximum number of results to return (default: 10)"},
			"time_range":  {Type: "string", Description: "Time range filter: day, week, month, year"},
			"backend":     {Type: "string", Description: "Search backend to use (e.g. tavily, exa). Uses default if omitted."},
		},
		Required: []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return tool.ToolResult{Content: "Error: query is required", IsError: true}, nil
	}

	opts := websearch.SearchOptions{
		MaxResults: 10,
	}
	if v, ok := params["max_results"]; ok {
		switch n := v.(type) {
		case float64:
			opts.MaxResults = int(n)
		case int:
			opts.MaxResults = n
		}
	}
	if v, ok := params["time_range"].(string); ok {
		opts.TimeRange = v
	}

	var backendName string
	if v, ok := params["backend"].(string); ok {
		backendName = v
	}

	if SearchFactory == nil {
		return tool.ToolResult{
			Content: "Error: web search is not configured",
			IsError: true,
		}, nil
	}
	results, err := SearchFactory.Search(ctx, query, opts, backendName)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error searching: %v", err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{Content: formatSearchResults(query, results)}, nil
}

// formatSearchResults renders search results in a human-readable format.
func formatSearchResults(query string, results []websearch.SearchResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Found %d results for %q:\n", len(results), query)

	for i, r := range results {
		fmt.Fprintf(&b, "\n%d. %s\n", i+1, r.Title)
		fmt.Fprintf(&b, "   %s\n", r.URL)
		if r.Description != "" {
			fmt.Fprintf(&b, "   %s\n", r.Description)
		}
	}

	return b.String()
}

func init() {
	tool.GlobalRegistry.Register(&WebSearchTool{})
}
