package file

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/openclaw/gclaw/internal/tool"
)

// PatchTool performs targeted string replacement in a file, similar to sed
// but with exact-match semantics and unified diff output.
type PatchTool struct{}

func (t *PatchTool) Name() string                              { return "Patch" }
func (t *PatchTool) Toolset() string                            { return "file" }
func (t *PatchTool) Description() string                        { return "Replace exact string matches in a file. Returns a unified diff of the change." }
func (t *PatchTool) Check() bool                                { return true }
func (t *PatchTool) ConcurrencySafe() bool                      { return false }
func (t *PatchTool) RequiresApproval(params map[string]any) bool { return true }

func (t *PatchTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"file_path":   {Type: "string", Description: "The absolute path to the file to patch"},
			"old_string":  {Type: "string", Description: "The exact string to find and replace"},
			"new_string":  {Type: "string", Description: "The replacement string"},
			"replace_all": {Type: "boolean", Description: "If true, replace all occurrences instead of just the first"},
		},
		Required: []string{"file_path", "old_string", "new_string"},
	}
}

func (t *PatchTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	filePath, ok := params["file_path"].(string)
	if !ok {
		return tool.ToolResult{Content: "Error: file_path is required", IsError: true}, nil
	}
	oldString, ok := params["old_string"].(string)
	if !ok {
		return tool.ToolResult{Content: "Error: old_string is required", IsError: true}, nil
	}
	newString, ok := params["new_string"].(string)
	if !ok {
		return tool.ToolResult{Content: "Error: new_string is required", IsError: true}, nil
	}

	replaceAll := false
	if v, ok := params["replace_all"]; ok {
		if b, ok := v.(bool); ok {
			replaceAll = b
		}
	}

	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error reading file %s: %v", filePath, err),
			IsError: true,
		}, nil
	}

	// Reject binary files (check for null bytes)
	if bytes.IndexByte(data, 0) >= 0 {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: file %s appears to be a binary file (contains null bytes)", filePath),
			IsError: true,
		}, nil
	}

	content := string(data)

	// Count occurrences
	count := strings.Count(content, oldString)
	if count == 0 {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: old_string not found in %s", filePath),
			IsError: true,
		}, nil
	}

	if count > 1 && !replaceAll {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: old_string found %d times in %s; set replace_all to true to replace all occurrences", count, filePath),
			IsError: true,
		}, nil
	}

	// Perform replacement
	var newContent string
	if replaceAll {
		newContent = strings.ReplaceAll(content, oldString, newString)
	} else {
		newContent = strings.Replace(content, oldString, newString, 1)
	}

	// Write back
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error writing file %s: %v", filePath, err),
			IsError: true,
		}, nil
	}

	// Generate unified diff
	diff := unifiedDiff(content, newContent, filePath)
	replaced := count
	if !replaceAll {
		replaced = 1
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Patched %s (%d replacement%s)\n%s", filePath, replaced, pluralS(replaced), diff),
	}, nil
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// unifiedDiff produces a unified diff between old and new content.
func unifiedDiff(oldContent, newContent, filePath string) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	// Remove trailing empty line that comes from trailing newline
	if len(oldLines) > 0 && oldLines[len(oldLines)-1] == "" {
		oldLines = oldLines[:len(oldLines)-1]
	}
	if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
		newLines = newLines[:len(newLines)-1]
	}

	var buf strings.Builder

	// Simple line-by-line diff: find first and last differing lines
	startOld := 0
	startNew := 0
	for startOld < len(oldLines) && startNew < len(newLines) && oldLines[startOld] == newLines[startNew] {
		startOld++
		startNew++
	}

	endOld := len(oldLines) - 1
	endNew := len(newLines) - 1
	for endOld > startOld && endNew > startNew && oldLines[endOld] == newLines[endNew] {
		endOld--
		endNew--
	}

	// Add context lines (up to 3 before and after)
	contextLines := 3
	ctxStart := startOld - contextLines
	if ctxStart < 0 {
		ctxStart = 0
	}
	ctxEndOld := endOld + contextLines
	if ctxEndOld >= len(oldLines) {
		ctxEndOld = len(oldLines) - 1
	}
	ctxEndNew := endNew + contextLines
	if ctxEndNew >= len(newLines) {
		ctxEndNew = len(newLines) - 1
	}

	// Header
	buf.WriteString(fmt.Sprintf("--- %s\n", filePath))
	buf.WriteString(fmt.Sprintf("+++ %s\n", filePath))
	buf.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", ctxStart+1, ctxEndOld-ctxStart+1, ctxStart+1, ctxEndNew-ctxStart+1))

	// Context before
	for i := ctxStart; i < startOld; i++ {
		buf.WriteString(" " + oldLines[i] + "\n")
	}

	// Removed lines
	for i := startOld; i <= endOld; i++ {
		buf.WriteString("-" + oldLines[i] + "\n")
	}

	// Added lines
	for i := startNew; i <= endNew; i++ {
		buf.WriteString("+" + newLines[i] + "\n")
	}

	// Context after
	commonEnd := ctxEndOld
	if commonEnd > ctxEndNew {
		commonEnd = ctxEndNew
	}
	for i := endOld + 1; i <= ctxEndOld && i < len(oldLines); i++ {
		buf.WriteString(" " + oldLines[i] + "\n")
	}

	return buf.String()
}

func init() {
	tool.GlobalRegistry.Register(&PatchTool{})
}
