package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openclaw/gclaw/internal/tool"
)

// Glob finds files matching a pattern.
type Glob struct{}

func (t *Glob) Name() string        { return "Glob" }
func (t *Glob) Description() string {
	return "Find files matching a glob pattern (e.g., **/*.go, src/**/*.ts)."
}
func (t *Glob) ConcurrencySafe() bool { return true }
func (t *Glob) RequiresApproval(params map[string]any) bool { return false }

func (t *Glob) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"pattern": {Type: "string", Description: "The glob pattern to match"},
			"path":    {Type: "string", Description: "The directory to search in (defaults to cwd)"},
		},
		Required: []string{"pattern"},
	}
}

func (t *Glob) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	pattern, ok := params["pattern"].(string)
	if !ok {
		return tool.ToolResult{Content: "Error: pattern is required", IsError: true}, nil
	}

	searchDir := "."
	if p, ok := params["path"].(string); ok && p != "" {
		searchDir = p
	}

	var matches []string
	err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip on error
		}
		// Skip .git and node_modules
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "__pycache__" || name == ".claude" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(searchDir, path)
		if err != nil {
			return nil
		}

		matched, err := filepath.Match(pattern, info.Name())
		if err == nil && matched {
			matches = append(matches, rel)
			return nil
		}

		// Also try matching against the full relative path
		matched, err = filepath.Match(pattern, rel)
		if err == nil && matched {
			matches = append(matches, rel)
		}
		return nil
	})

	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error walking directory: %v", err),
			IsError: true,
		}, nil
	}

	sort.Strings(matches)

	if len(matches) > 250 {
		matches = matches[:250]
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Found %d files:\n%s", len(matches), strings.Join(matches, "\n")),
	}, nil
}
