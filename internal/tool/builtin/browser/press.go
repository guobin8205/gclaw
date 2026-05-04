package browser

import (
	"context"
	"fmt"

	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface check.
var _ tool.Tool = (*PressTool)(nil)

// PressTool dispatches a keyboard key press event.
type PressTool struct{}

func (t *PressTool) Name() string             { return "browser_press" }
func (t *PressTool) Toolset() string           { return "browser" }
func (t *PressTool) ConcurrencySafe() bool     { return false }
func (t *PressTool) RequiresApproval(map[string]any) bool { return true }

func (t *PressTool) Description() string {
	return "Press a keyboard key (e.g. 'Enter', 'Tab', 'Escape', 'Backspace', 'a'). Dispatches a key-down and key-up event."
}

func (t *PressTool) Check() bool {
	return BrowserRef != nil
}

func (t *PressTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"key": {
				Type:        "string",
				Description: "The key to press (e.g. 'Enter', 'Tab', 'Escape', 'a', '1')",
			},
		},
		Required: []string{"key"},
	}
}

func (t *PressTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	if BrowserRef == nil {
		return tool.ToolResult{Content: "Error: browser not available", IsError: true}, nil
	}

	key, _ := params["key"].(string)
	if key == "" {
		return tool.ToolResult{Content: "Error: key parameter is required", IsError: true}, nil
	}

	if err := BrowserRef.Press(ctx, key); err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error pressing key %q: %v", key, err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Pressed key %q", key),
	}, nil
}

func init() {
	tool.GlobalRegistry.Register(&PressTool{})
}
