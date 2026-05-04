package browser

import (
	"context"
	"fmt"

	"github.com/openclaw/gclaw/internal/browser"
	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface check.
var _ tool.Tool = (*NavigateTool)(nil)

// BrowserRef is the shared browser instance, set by main.go.
var BrowserRef browser.Browser

// NavigateTool opens a URL in the browser.
type NavigateTool struct{}

func (t *NavigateTool) Name() string             { return "browser_navigate" }
func (t *NavigateTool) Toolset() string           { return "browser" }
func (t *NavigateTool) ConcurrencySafe() bool     { return false }
func (t *NavigateTool) RequiresApproval(map[string]any) bool { return true }

func (t *NavigateTool) Description() string {
	return "Navigate the browser to a URL. Use this to open web pages for interaction."
}

func (t *NavigateTool) Check() bool {
	return BrowserRef != nil
}

func (t *NavigateTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"url": {
				Type:        "string",
				Description: "The URL to navigate to",
			},
		},
		Required: []string{"url"},
	}
}

func (t *NavigateTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	if BrowserRef == nil {
		return tool.ToolResult{Content: "Error: browser not available", IsError: true}, nil
	}

	url, _ := params["url"].(string)
	if url == "" {
		return tool.ToolResult{Content: "Error: url parameter is required", IsError: true}, nil
	}

	if err := BrowserRef.Navigate(ctx, url); err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error navigating to %s: %v", url, err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Navigated to %s", url),
	}, nil
}

func init() {
	tool.GlobalRegistry.Register(&NavigateTool{})
}
