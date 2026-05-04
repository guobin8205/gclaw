package codeexec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/openclaw/gclaw/internal/tool"
)

// CodeExecuteTool executes code in a sandboxed temporary directory.
type CodeExecuteTool struct{}

func (t *CodeExecuteTool) Name() string                              { return "code_execute" }
func (t *CodeExecuteTool) Toolset() string                            { return "codeexec" }
func (t *CodeExecuteTool) Description() string {
	return "Execute code in a sandboxed environment. Supports Go, Python, JavaScript, and Shell."
}
func (t *CodeExecuteTool) Check() bool                                { return true }
func (t *CodeExecuteTool) ConcurrencySafe() bool                      { return false }
func (t *CodeExecuteTool) RequiresApproval(params map[string]any) bool { return true }

func (t *CodeExecuteTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"code":     {Type: "string", Description: "The code to execute"},
			"language": {Type: "string", Description: "Programming language", Enum: []string{"go", "python", "javascript", "shell"}},
			"timeout":  {Type: "integer", Description: "Timeout in milliseconds (default 30000)"},
		},
		Required: []string{"code", "language"},
	}
}

func (t *CodeExecuteTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	code, ok := params["code"].(string)
	if !ok {
		return tool.ToolResult{Content: "Error: code is required", IsError: true}, nil
	}

	language, ok := params["language"].(string)
	if !ok {
		return tool.ToolResult{Content: "Error: language is required", IsError: true}, nil
	}

	timeout := 30000 // default 30s
	if ms, ok := params["timeout"].(float64); ok && ms > 0 {
		timeout = int(ms)
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "gclaw-codeexec-*")
	if err != nil {
		return tool.ToolResult{Content: fmt.Sprintf("Error creating temp dir: %v", err), IsError: true}, nil
	}
	defer os.RemoveAll(tmpDir)

	// Prepare source file and command
	filename, cmd, err := prepareExecution(language, code, tmpDir)
	if err != nil {
		return tool.ToolResult{Content: fmt.Sprintf("Error: %v", err), IsError: true}, nil
	}

	// Write source file
	if err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(code), 0644); err != nil {
		return tool.ToolResult{Content: fmt.Sprintf("Error writing source file: %v", err), IsError: true}, nil
	}

	// Execute with timeout
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmdErr := cmd.Run()

	// Check timeout first
	if execCtx.Err() == context.DeadlineExceeded {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: execution timed out after %dms", timeout),
			IsError: true,
		}, nil
	}

	combined := stdout.String() + stderr.String()

	// Build result
	var result string
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 0 {
		result = fmt.Sprintf("Exit code: %d\nOutput:\n%s", cmd.ProcessState.ExitCode(), combined)
	} else {
		result = fmt.Sprintf("Output:\n%s", combined)
	}

	isError := false
	if cmdErr != nil {
		isError = true
	}

	return tool.ToolResult{Content: result, IsError: isError}, nil
}

// prepareExecution returns the filename and prepared command for the given language.
func prepareExecution(language, code, tmpDir string) (string, *exec.Cmd, error) {
	switch language {
	case "go":
		return prepareGo(code, tmpDir)
	case "python":
		return preparePython(tmpDir)
	case "javascript":
		return prepareJavaScript(tmpDir)
	case "shell":
		return prepareShell(tmpDir)
	default:
		return "", nil, fmt.Errorf("unsupported language: %s", language)
	}
}

func prepareGo(code, tmpDir string) (string, *exec.Cmd, error) {
	// If the code doesn't have a package declaration, wrap it
	if !strings.Contains(code, "package ") {
		code = fmt.Sprintf(`package main

import "fmt"

func main() {
%s
}`, indentCode(code))
	}

	// Write the potentially wrapped code to main.go
	filename := "main.go"
	if err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(code), 0644); err != nil {
		return "", nil, fmt.Errorf("writing go source: %w", err)
	}

	cmd := exec.Command("go", "run", filename)
	cmd.Dir = tmpDir
	return filename, cmd, nil
}

func preparePython(tmpDir string) (string, *exec.Cmd, error) {
	pythonBin, err := exec.LookPath("python")
	if err != nil {
		pythonBin, err = exec.LookPath("python3")
		if err != nil {
			return "", nil, fmt.Errorf("python not found: install python or python3")
		}
	}

	cmd := exec.Command(pythonBin, "script.py")
	cmd.Dir = tmpDir
	return "script.py", cmd, nil
}

func prepareJavaScript(tmpDir string) (string, *exec.Cmd, error) {
	if _, err := exec.LookPath("node"); err != nil {
		return "", nil, fmt.Errorf("node not found: install Node.js")
	}

	cmd := exec.Command("node", "script.js")
	cmd.Dir = tmpDir
	return "script.js", cmd, nil
}

func prepareShell(tmpDir string) (string, *exec.Cmd, error) {
	// Try sh first (Git Bash), then bash, then fall back to cmd
	shellBin, err := exec.LookPath("sh")
	if err != nil {
		shellBin, err = exec.LookPath("bash")
		if err != nil {
			// Fall back to cmd
			cmd := exec.Command("cmd", "/C", "script.sh")
			cmd.Dir = tmpDir
			return "script.sh", cmd, nil
		}
	}

	cmd := exec.Command(shellBin, "script.sh")
	cmd.Dir = tmpDir
	return "script.sh", cmd, nil
}

// indentCode indents each non-empty line of code with a tab.
func indentCode(code string) string {
	lines := strings.Split(code, "\n")
	var buf strings.Builder
	for i, line := range lines {
		if line == "" {
			if i < len(lines)-1 {
				buf.WriteString("\n")
			}
			continue
		}
		buf.WriteString("\t")
		buf.WriteString(line)
		if i < len(lines)-1 {
			buf.WriteString("\n")
		}
	}
	return buf.String()
}

func init() {
	tool.GlobalRegistry.Register(&CodeExecuteTool{})
}
