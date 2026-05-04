package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface compliance check.
var _ tool.Tool = (*MemoryTool)(nil)

// Dir is the directory where memory files are stored. Set by main.go.
var Dir string

// threatPatterns contains substrings that indicate potential prompt injection.
var threatPatterns = []string{
	"ignore previous instructions",
	"ignore all previous",
	"disregard previous",
	"forget everything",
	"new instructions:",
	"system prompt:",
	"you are now",
	"act as if",
	"jailbreak",
	"ignore the above",
	"ignore above instructions",
}

// MemoryTool provides persistent cross-session memory via file-backed key-value storage.
type MemoryTool struct{}

func (t *MemoryTool) Name() string        { return "Memory" }
func (t *MemoryTool) Toolset() string     { return "memory" }
func (t *MemoryTool) ConcurrencySafe() bool { return true }
func (t *MemoryTool) RequiresApproval(map[string]any) bool { return false }

func (t *MemoryTool) Description() string {
	return "Persistent cross-session memory. Store and retrieve key-value notes across conversations."
}

func (t *MemoryTool) Check() bool {
	return Dir != ""
}

func (t *MemoryTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"action": {
				Type:        "string",
				Description: "Action to perform",
				Enum:        []string{"read", "add", "replace", "remove"},
			},
			"key": {
				Type:        "string",
				Description: "Memory key (used as filename)",
			},
			"content": {
				Type:        "string",
				Description: "Content to store (for add/replace)",
			},
		},
		Required: []string{"action"},
	}
}

func (t *MemoryTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	action, _ := params["action"].(string)
	key, _ := params["key"].(string)
	content, _ := params["content"].(string)

	switch action {
	case "read":
		return t.read()
	case "add":
		return t.add(key, content)
	case "replace":
		return t.replace(key, content)
	case "remove":
		return t.remove(key)
	default:
		return tool.ToolResult{
			Content: fmt.Sprintf("Unknown action %q. Use: read, add, replace, remove", action),
			IsError: true,
		}, nil
	}
}

// read lists all memory files and returns a summary.
func (t *MemoryTool) read() (tool.ToolResult, error) {
	entries, err := os.ReadDir(Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return tool.ToolResult{Content: "0 memories:\n"}, nil
		}
		return tool.ToolResult{
			Content: fmt.Sprintf("Error reading memory directory: %v", err),
			IsError: true,
		}, nil
	}

	var memories []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		key := strings.TrimSuffix(e.Name(), ".md")
		filePath := filepath.Join(Dir, e.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		firstLine := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)[0]
		memories = append(memories, fmt.Sprintf("- %s: %s", key, firstLine))
	}

	if len(memories) == 0 {
		return tool.ToolResult{Content: "0 memories:\n"}, nil
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("%d memories:\n%s\n", len(memories), strings.Join(memories, "\n")),
	}, nil
}

// add creates a new memory file. Returns error if key already exists.
func (t *MemoryTool) add(key, content string) (tool.ToolResult, error) {
	if key == "" {
		return tool.ToolResult{Content: "Error: key is required for add action", IsError: true}, nil
	}
	if content == "" {
		return tool.ToolResult{Content: "Error: content is required for add action", IsError: true}, nil
	}
	if containsThreat(content) {
		return tool.ToolResult{
			Content: "Error: content contains potentially unsafe instructions",
			IsError: true,
		}, nil
	}

	sanitized := sanitizeKey(key)
	filePath := filepath.Join(Dir, sanitized+".md")

	if _, err := os.Stat(filePath); err == nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: memory key %q already exists (use replace to update)", key),
			IsError: true,
		}, nil
	}

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error writing memory: %v", err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Memory %q added successfully.", key),
	}, nil
}

// replace overwrites an existing memory file. Returns error if not found.
func (t *MemoryTool) replace(key, content string) (tool.ToolResult, error) {
	if key == "" {
		return tool.ToolResult{Content: "Error: key is required for replace action", IsError: true}, nil
	}
	if content == "" {
		return tool.ToolResult{Content: "Error: content is required for replace action", IsError: true}, nil
	}
	if containsThreat(content) {
		return tool.ToolResult{
			Content: "Error: content contains potentially unsafe instructions",
			IsError: true,
		}, nil
	}

	sanitized := sanitizeKey(key)
	filePath := filepath.Join(Dir, sanitized+".md")

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: memory key %q not found (use add to create)", key),
			IsError: true,
		}, nil
	}

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error writing memory: %v", err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Memory %q replaced successfully.", key),
	}, nil
}

// remove deletes a memory file. Returns error if not found.
func (t *MemoryTool) remove(key string) (tool.ToolResult, error) {
	if key == "" {
		return tool.ToolResult{Content: "Error: key is required for remove action", IsError: true}, nil
	}

	sanitized := sanitizeKey(key)
	filePath := filepath.Join(Dir, sanitized+".md")

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: memory key %q not found", key),
			IsError: true,
		}, nil
	}

	if err := os.Remove(filePath); err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error removing memory: %v", err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Memory %q removed.", key),
	}, nil
}

// sanitizeKey normalizes a memory key for use as a filename.
func sanitizeKey(key string) string {
	s := strings.ToLower(key)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.TrimLeft(s, "-")
	return s
}

// containsThreat checks content for known injection patterns.
func containsThreat(content string) bool {
	lower := strings.ToLower(content)
	for _, pattern := range threatPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func init() {
	tool.GlobalRegistry.Register(&MemoryTool{})
}
