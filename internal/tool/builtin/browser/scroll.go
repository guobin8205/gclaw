package browser

import (
	"context"
	"fmt"

	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface check.
var _ tool.Tool = (*ScrollTool)(nil)

// ScrollTool scrolls the page up or down.
type ScrollTool struct{}

func (t *ScrollTool) Name() string             { return "browser_scroll" }
func (t *ScrollTool) Toolset() string           { return "browser" }
func (t *ScrollTool) ConcurrencySafe() bool     { return false }
func (t *ScrollTool) RequiresApproval(map[string]any) bool { return true }

func (t *ScrollTool) Description() string {
	return "Scroll the page up or down by a specified number of viewport heights (default 3)."
}

func (t *ScrollTool) Check() bool {
	return BrowserRef != nil
}

func (t *ScrollTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"direction": {
				Type:        "string",
				Description: "Scroll direction: 'up' or 'down'",
				Enum:        []string{"up", "down"},
			},
			"amount": {
				Type:        "integer",
				Description: "Number of viewport heights to scroll (default: 3)",
			},
		},
		Required: []string{"direction"},
	}
}

func (t *ScrollTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	if BrowserRef == nil {
		return tool.ToolResult{Content: "Error: browser not available", IsError: true}, nil
	}

	direction, _ := params["direction"].(string)
	if direction == "" {
		return tool.ToolResult{Content: "Error: direction parameter is required", IsError: true}, nil
	}

	amount := 3
	if v, ok := params["amount"]; ok {
		switch n := v.(type) {
		case float64:
			amount = int(n)
		case int:
			amount = n
		}
	}

	if err := BrowserRef.Scroll(ctx, direction, amount); err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error scrolling %s: %v", direction, err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Scrolled %s by %d viewport height(s)", direction, amount),
	}, nil
}

func init() {
	tool.GlobalRegistry.Register(&ScrollTool{})
}
