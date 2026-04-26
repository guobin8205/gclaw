package shell

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/openclaw/gclaw/internal/tool"
)

// BashTool executes shell commands.
type BashTool struct {
	DefaultTimeout time.Duration
}

func (t *BashTool) Name() string  { return "Bash" }
func (t *BashTool) Toolset() string { return "shell" }
func (t *BashTool) Description() string {
	return fmt.Sprintf("Execute a shell command and return stdout/stderr. Shell: %s", resolveShell())
}
func (t *BashTool) Check() bool            { return true }
func (t *BashTool) ConcurrencySafe() bool  { return false }
func (t *BashTool) RequiresApproval(params map[string]any) bool {
	cmd, ok := params["command"].(string)
	if !ok {
		return true
	}
	safe := []string{"git status", "git diff", "git log", "ls", "cat", "echo", "pwd", "whoami", "which"}
	for _, s := range safe {
		if strings.HasPrefix(strings.TrimSpace(cmd), s) {
			return false
		}
	}
	return true
}

func (t *BashTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"command":     {Type: "string", Description: "The shell command to execute"},
			"description": {Type: "string", Description: "Short description of what this command does"},
			"timeout":     {Type: "integer", Description: "Timeout in milliseconds"},
		},
		Required: []string{"command"},
	}
}

func (t *BashTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	command, ok := params["command"].(string)
	if !ok {
		return tool.ToolResult{Content: "Error: command is required", IsError: true}, nil
	}

	timeout := t.DefaultTimeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	if ms, ok := params["timeout"].(float64); ok && ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	shell := resolveShell()

	var c *exec.Cmd
	switch shell {
	case "cmd":
		c = exec.CommandContext(execCtx, "cmd", "/C", command)
	case "powershell":
		c = exec.CommandContext(execCtx, "powershell", "-NoProfile", "-Command", command)
	default:
		c = exec.CommandContext(execCtx, shell, "-c", command)
	}

	output, err := c.CombinedOutput()
	output = sanitizeOutput(output)

	if execCtx.Err() == context.DeadlineExceeded {
		return tool.ToolResult{
			Content: fmt.Sprintf("Command timed out after %v: %s", timeout, command),
			IsError: true,
		}, nil
	}

	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Command failed (exit: %v):\n%s", err, string(output)),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{Content: string(output)}, nil
}

func resolveShell() string {
	if _, err := exec.LookPath("powershell"); err == nil {
		return "powershell"
	}
	if _, err := exec.LookPath("sh"); err == nil {
		return "sh"
	}
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash"
	}
	return "cmd"
}

func sanitizeOutput(raw []byte) []byte {
	if utf8.Valid(raw) {
		return raw
	}
	if len(raw) >= 2 {
		if raw[0] == 0xFF && raw[1] == 0xFE {
			return utf16LEToUTF8(raw[2:])
		}
		if raw[0] == 0xFE && raw[1] == 0xFF {
			return utf16BEToUTF8(raw[2:])
		}
	}
	if len(raw) > 4 && raw[1] == 0x00 && raw[3] == 0x00 && raw[5] == 0x00 {
		return utf16LEToUTF8(raw)
	}
	return raw
}

func utf16LEToUTF8(raw []byte) []byte {
	var out strings.Builder
	for i := 0; i+1 < len(raw); i += 2 {
		r := rune(uint16(raw[i]) | uint16(raw[i+1])<<8)
		out.WriteRune(r)
	}
	return []byte(out.String())
}

func utf16BEToUTF8(raw []byte) []byte {
	var out strings.Builder
	for i := 0; i+1 < len(raw); i += 2 {
		r := rune(uint16(raw[i])<<8 | uint16(raw[i+1]))
		out.WriteRune(r)
	}
	return []byte(out.String())
}

func init() {
	tool.GlobalRegistry.Register(&BashTool{})
}
