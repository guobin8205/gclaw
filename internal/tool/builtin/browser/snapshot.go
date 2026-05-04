package browser

import (
	"context"
	"fmt"

	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface check.
var _ tool.Tool = (*SnapshotTool)(nil)

// SnapshotTool captures an accessibility-tree text representation of the current page.
type SnapshotTool struct{}

func (t *SnapshotTool) Name() string             { return "browser_snapshot" }
func (t *SnapshotTool) Toolset() string           { return "browser" }
func (t *SnapshotTool) ConcurrencySafe() bool     { return false }
func (t *SnapshotTool) RequiresApproval(map[string]any) bool { return true }

func (t *SnapshotTool) Description() string {
	return "Capture an accessibility tree snapshot of the current page. Returns a text representation with @ref identifiers for interactive elements that can be used with browser_click and browser_type."
}

func (t *SnapshotTool) Check() bool {
	return BrowserRef != nil
}

func (t *SnapshotTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type:       "object",
		Properties: map[string]tool.Property{},
	}
}

func (t *SnapshotTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	if BrowserRef == nil {
		return tool.ToolResult{Content: "Error: browser not available", IsError: true}, nil
	}

	text, err := BrowserRef.Snapshot(ctx)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error taking snapshot: %v", err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{Content: text}, nil
}

func init() {
	tool.GlobalRegistry.Register(&SnapshotTool{})
}
