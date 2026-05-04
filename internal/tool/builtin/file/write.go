package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openclaw/gclaw/internal/tool"
)

// CheckpointManager is injected by main.go when checkpointing is enabled.
var CheckpointManager interface {
	EnsureCheckpoint(dir string, reason string) error
}

// WriteFileTool creates or overwrites a file with content.
type WriteFileTool struct{}

func (t *WriteFileTool) Name() string                              { return "WriteFile" }
func (t *WriteFileTool) Toolset() string                            { return "file" }
func (t *WriteFileTool) Description() string                        { return "Write content to a file, overwriting if it exists." }
func (t *WriteFileTool) Check() bool                                { return true }
func (t *WriteFileTool) ConcurrencySafe() bool                      { return false }
func (t *WriteFileTool) RequiresApproval(params map[string]any) bool { return true }

func (t *WriteFileTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"file_path": {Type: "string", Description: "The absolute path to the file to write"},
			"content":   {Type: "string", Description: "The content to write to the file"},
		},
		Required: []string{"file_path", "content"},
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	filePath, ok := params["file_path"].(string)
	if !ok {
		return tool.ToolResult{Content: "Error: file_path is required", IsError: true}, nil
	}
	content, ok := params["content"].(string)
	if !ok {
		return tool.ToolResult{Content: "Error: content is required", IsError: true}, nil
	}

	dir := filepath.Dir(filePath)
	if CheckpointManager != nil {
		_ = CheckpointManager.EnsureCheckpoint(dir, "write_file: "+filePath)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error creating directory %s: %v", dir, err),
			IsError: true,
		}, nil
	}

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error writing file %s: %v", filePath, err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{Content: fmt.Sprintf("File written: %s (%d bytes)", filePath, len(content))}, nil
}
