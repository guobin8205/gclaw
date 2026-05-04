package browser

import (
	"context"
	"fmt"

	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface check.
var _ tool.Tool = (*TypeTool)(nil)

// TypeTool clears an input element and types text into it.
type TypeTool struct{}

func (t *TypeTool) Name() string             { return "browser_type" }
func (t *TypeTool) Toolset() string           { return "browser" }
func (t *TypeTool) ConcurrencySafe() bool     { return false }
func (t *TypeTool) RequiresApproval(map[string]any) bool { return true }

func (t *TypeTool) Description() string {
	return "Clear an input element identified by @ref and type text into it. The element must be visible and focusable."
}

func (t *TypeTool) Check() bool {
	return BrowserRef != nil
}

func (t *TypeTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"ref": {
				Type:        "string",
				Description: "The @ref identifier of the input element (e.g. '@e5')",
			},
			"text": {
				Type:        "string",
				Description: "The text to type into the element",
			},
		},
		Required: []string{"ref", "text"},
	}
}

func (t *TypeTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	if BrowserRef == nil {
		return tool.ToolResult{Content: "Error: browser not available", IsError: true}, nil
	}

	ref, _ := params["ref"].(string)
	if ref == "" {
		return tool.ToolResult{Content: "Error: ref parameter is required", IsError: true}, nil
	}

	text, _ := params["text"].(string)
	if text == "" {
		return tool.ToolResult{Content: "Error: text parameter is required", IsError: true}, nil
	}

	if err := BrowserRef.Type(ctx, ref, text); err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error typing into %s: %v", ref, err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Typed text into element %s", ref),
	}, nil
}

func init() {
	tool.GlobalRegistry.Register(&TypeTool{})
}
