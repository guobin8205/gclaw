package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface check.
var _ tool.Tool = (*ScreenshotTool)(nil)

// ScreenshotTool captures a full-page screenshot as a PNG image.
type ScreenshotTool struct{}

func (t *ScreenshotTool) Name() string             { return "browser_screenshot" }
func (t *ScreenshotTool) Toolset() string           { return "browser" }
func (t *ScreenshotTool) ConcurrencySafe() bool     { return false }
func (t *ScreenshotTool) RequiresApproval(map[string]any) bool { return true }

func (t *ScreenshotTool) Description() string {
	return "Capture a full-page screenshot of the current browser page. Returns the path to the saved PNG file."
}

func (t *ScreenshotTool) Check() bool {
	return BrowserRef != nil
}

func (t *ScreenshotTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type:       "object",
		Properties: map[string]tool.Property{},
	}
}

func (t *ScreenshotTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	if BrowserRef == nil {
		return tool.ToolResult{Content: "Error: browser not available", IsError: true}, nil
	}

	pngBytes, err := BrowserRef.Screenshot(ctx)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error taking screenshot: %v", err),
			IsError: true,
		}, nil
	}

	// Save to a temp file.
	tmpDir := os.TempDir()
	filename := fmt.Sprintf("browser_screenshot_%d.png", os.Getpid())
	path := filepath.Join(tmpDir, filename)

	if err := os.WriteFile(path, pngBytes, 0644); err != nil {
		// Fallback: return base64-encoded data.
		b64 := base64.StdEncoding.EncodeToString(pngBytes)
		return tool.ToolResult{
			Content: fmt.Sprintf("Screenshot captured (%d bytes). Base64:\n%s", len(pngBytes), b64),
		}, nil
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Screenshot saved to %s (%d bytes)", path, len(pngBytes)),
	}, nil
}

func init() {
	tool.GlobalRegistry.Register(&ScreenshotTool{})
}
