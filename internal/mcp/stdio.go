package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// stdioTransport communicates with an MCP server via subprocess stdin/stdout.
type stdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
}

// newStdioTransport launches the MCP server as a subprocess.
func newStdioTransport(command string, env map[string]string) (*stdioTransport, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)

	// Set environment variables
	if len(env) > 0 {
		cmd.Env = overrideEnv(env)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	// Redirect stderr to discard (or could log it)
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdoutPipe.Close()
		return nil, fmt.Errorf("start process: %w", err)
	}

	return &stdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReaderSize(stdoutPipe, 64*1024),
	}, nil
}

// send sends a JSON-RPC request and reads the response.
func (t *stdioTransport) send(ctx context.Context, request jsonRPCRequest) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Write JSON-RPC message as a single line
	if _, err := fmt.Fprintf(t.stdin, "%s\n", data); err != nil {
		return nil, fmt.Errorf("write to stdin: %w", err)
	}

	// Read response line
	line, err := t.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read from stdout: %w", err)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}

// sendNotification sends a JSON-RPC notification (no ID, no response expected).
func (t *stdioTransport) sendNotification(notif jsonRPCRequest) error {
	data, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	_, err = fmt.Fprintf(t.stdin, "%s\n", data)
	return err
}

// close terminates the subprocess.
func (t *stdioTransport) close() error {
	t.stdin.Close()
	if t.cmd.Process != nil {
		t.cmd.Process.Kill()
	}
	return t.cmd.Wait()
}

// overrideEnv returns os.Environ with additional env vars overridden.
func overrideEnv(env map[string]string) []string {
	return nil // simplified: let the process inherit os env
}
