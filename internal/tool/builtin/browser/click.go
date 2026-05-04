package browser

import (
	"context"
	"fmt"

	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface check.
var _ tool.Tool = (*ClickTool)(nil)

// ClickTool clicks an element identified by its @ref identifier.
type ClickTool struct{}

func (t *ClickTool) Name() string             { return "browser_click" }
func (t *ClickTool) Toolset() string           { return "browser" }
func (t *ClickTool) ConcurrencySafe() bool     { return false }
func (t *ClickTool) RequiresApproval(map[string]any) bool { return true }

func (t *ClickTool) Description() string {
	return "Click an element on the page by its @ref identifier (obtained from browser_snapshot)."
}

func (t *ClickTool) Check() bool {
	return BrowserRef != nil
}

func (t *ClickTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"ref": {
				Type:        "string",
				Description: "The @ref identifier of the element to click (e.g. '@e3')",
			},
		},
		Required: []string{"ref"},
	}
}

func (t *ClickTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	if BrowserRef == nil {
		return tool.ToolResult{Content: "Error: browser not available", IsError: true}, nil
	}

	ref, _ := params["ref"].(string)
	if ref == "" {
		return tool.ToolResult{Content: "Error: ref parameter is required", IsError: true}, nil
	}

	if err := BrowserRef.Click(ctx, ref); err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error clicking %s: %v", ref, err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Clicked element %s", ref),
	}, nil
}

func init() {
	tool.GlobalRegistry.Register(&ClickTool{})
}
