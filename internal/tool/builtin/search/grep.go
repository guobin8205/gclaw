package search

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openclaw/gclaw/internal/tool"
)

// GrepTool searches for a pattern in files.
type GrepTool struct{}

func (t *GrepTool) Name() string        { return "Grep" }
func (t *GrepTool) Toolset() string       { return "search" }
func (t *GrepTool) Description() string {
	return "Search for a regex pattern in file contents within a directory."
}
func (t *GrepTool) Check() bool            { return true }
func (t *GrepTool) ConcurrencySafe() bool  { return true }
func (t *GrepTool) RequiresApproval(params map[string]any) bool { return false }

func (t *GrepTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"pattern": {Type: "string", Description: "The regex pattern to search for"},
			"path":    {Type: "string", Description: "Directory to search in (defaults to cwd)"},
			"include": {Type: "string", Description: "Glob to filter files (e.g. *.go, *.ts)"},
		},
		Required: []string{"pattern"},
	}
}

func (t *GrepTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	pattern, ok := params["pattern"].(string)
	if !ok {
		return tool.ToolResult{Content: "Error: pattern is required", IsError: true}, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Invalid regex pattern: %v", err),
			IsError: true,
		}, nil
	}

	searchDir := "."
	if p, ok := params["path"].(string); ok && p != "" {
		searchDir = p
	}

	var includeGlob string
	if inc, ok := params["include"].(string); ok {
		includeGlob = inc
	}

	type match struct {
		file string
		line int
		text string
	}
	var matches []match

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

		if includeGlob != "" {
			matched, _ := filepath.Match(includeGlob, info.Name())
			if !matched {
				return nil
			}
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if re.MatchString(line) {
				rel, _ := filepath.Rel(searchDir, path)
				matches = append(matches, match{rel, lineNum, strings.TrimSpace(line)})
			}
		}
		return nil
	})

	if len(matches) > 250 {
		matches = matches[:250]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matches:\n", len(matches)))
	for _, m := range matches {
		sb.WriteString(fmt.Sprintf("%s:%d: %s\n", m.file, m.line, m.text))
	}

	return tool.ToolResult{Content: sb.String()}, nil
}

func init() {
	tool.GlobalRegistry.Register(&GrepTool{})
}
