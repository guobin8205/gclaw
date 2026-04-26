package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openclaw/gclaw/internal/tool"
)

// GlobTool finds files matching a pattern.
type GlobTool struct{}

func (t *GlobTool) Name() string        { return "Glob" }
func (t *GlobTool) Toolset() string       { return "search" }
func (t *GlobTool) Description() string {
	return "Find files matching a glob pattern (e.g., **/*.go, src/**/*.ts)."
}
func (t *GlobTool) Check() bool            { return true }
func (t *GlobTool) ConcurrencySafe() bool  { return true }
func (t *GlobTool) RequiresApproval(params map[string]any) bool { return false }

func (t *GlobTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"pattern": {Type: "string", Description: "The glob pattern to match"},
			"path":    {Type: "string", Description: "The directory to search in (defaults to cwd)"},
		},
		Required: []string{"pattern"},
	}
}

func (t *GlobTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	pattern, ok := params["pattern"].(string)
	if !ok {
		return tool.ToolResult{Content: "Error: pattern is required", IsError: true}, nil
	}

	searchDir := "."
	if p, ok := params["path"].(string); ok && p != "" {
		searchDir = p
	}

	var matches []string
	filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
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

		matched, err = filepath.Match(pattern, rel)
		if err == nil && matched {
			matches = append(matches, rel)
		}
		return nil
	})

	sort.Strings(matches)

	if len(matches) > 250 {
		matches = matches[:250]
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Found %d files:\n%s", len(matches), strings.Join(matches, "\n")),
	}, nil
}

func init() {
	tool.GlobalRegistry.Register(&GlobTool{})
}
