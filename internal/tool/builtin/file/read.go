package file

import (
	"context"
	"fmt"
	"os"

	"github.com/openclaw/gclaw/internal/tool"
)

// ReadFileTool reads the contents of a file.
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string                              { return "ReadFile" }
func (t *ReadFileTool) Toolset() string                            { return "file" }
func (t *ReadFileTool) Description() string                        { return "Read the contents of a file at the given path." }
func (t *ReadFileTool) Check() bool                                { return true }
func (t *ReadFileTool) ConcurrencySafe() bool                      { return true }
func (t *ReadFileTool) RequiresApproval(params map[string]any) bool { return false }

func (t *ReadFileTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"file_path": {Type: "string", Description: "The absolute path to the file to read"},
			"offset":    {Type: "integer", Description: "Line number to start reading from (0-indexed)"},
			"limit":     {Type: "integer", Description: "Maximum number of lines to read"},
		},
		Required: []string{"file_path"},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	filePath, ok := params["file_path"].(string)
	if !ok {
		return tool.ToolResult{Content: "Error: file_path is required", IsError: true}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error reading file %s: %v", filePath, err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{Content: string(data)}, nil
}

func init() {
	tool.GlobalRegistry.Register(&ReadFileTool{})
}
