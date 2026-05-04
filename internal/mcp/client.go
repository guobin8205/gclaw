package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// ServerConfig holds the configuration for an MCP server connection.
type ServerConfig struct {
	Name    string            `json:"name" yaml:"name"`
	Command string            `json:"command,omitempty" yaml:"command"` // stdio mode: command to launch
	URL     string            `json:"url,omitempty" yaml:"url"`         // HTTP mode: server URL
	Env     map[string]string `json:"env,omitempty" yaml:"env"`
}

// Tool represents a tool discovered from an MCP server.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// transport is the interface for MCP transport layers.
type transport interface {
	send(ctx context.Context, request jsonRPCRequest) (json.RawMessage, error)
	close() error
}

// Client manages a connection to a single MCP server.
type Client struct {
	config    ServerConfig
	tools     []Tool
	transport transport
	connected bool
	mu        sync.RWMutex
	nextID    atomic.Int64
}

// newClient creates a new MCP client for the given server config.
func newClient(cfg ServerConfig) *Client {
	c := &Client{
		config: cfg,
	}
	c.nextID.Store(0)
	return c
}

// Connect establishes a connection to the MCP server.
// For stdio mode, it launches the subprocess.
// For HTTP mode, it verifies the endpoint.
// Then it sends the initialize handshake.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// Create transport based on config
	var t transport
	var err error
	if c.config.Command != "" {
		t, err = newStdioTransport(c.config.Command, c.config.Env)
	} else if c.config.URL != "" {
		t, err = newHTTPTransport(c.config.URL)
	} else {
		return fmt.Errorf("mcp server %q: must specify either command or url", c.config.Name)
	}
	if err != nil {
		return fmt.Errorf("mcp server %q: create transport: %w", c.config.Name, err)
	}
	c.transport = t

	// Send initialize request
	initParams := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "gclaw",
			"version": "1.0.0",
		},
	}
	_, err = c.transport.send(ctx, c.newRequest("initialize", initParams))
	if err != nil {
		c.transport.close()
		c.transport = nil
		return fmt.Errorf("mcp server %q: initialize: %w", c.config.Name, err)
	}

	// Send initialized notification (no id, no response expected)
	notif := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	if stdio, ok := c.transport.(*stdioTransport); ok {
		stdio.sendNotification(notif)
	} else if http, ok := c.transport.(*httpTransport); ok {
		http.sendNotification(ctx, notif)
	}

	c.connected = true
	return nil
}

// DiscoverTools calls tools/list on the MCP server and caches results.
func (c *Client) DiscoverTools(ctx context.Context) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("mcp server %q: not connected", c.config.Name)
	}
	c.mu.RUnlock()

	result, err := c.transport.send(ctx, c.newRequest("tools/list", map[string]any{}))
	if err != nil {
		return fmt.Errorf("mcp server %q: tools/list: %w", c.config.Name, err)
	}

	var listResult struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(result, &listResult); err != nil {
		return fmt.Errorf("mcp server %q: parse tools/list response: %w", c.config.Name, err)
	}

	c.mu.Lock()
	c.tools = listResult.Tools
	c.mu.Unlock()

	return nil
}

// CallTool invokes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, toolName string, args map[string]any) (string, error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return "", fmt.Errorf("mcp server %q: not connected", c.config.Name)
	}
	c.mu.RUnlock()

	params := map[string]any{
		"name":      toolName,
		"arguments": args,
	}
	result, err := c.transport.send(ctx, c.newRequest("tools/call", params))
	if err != nil {
		return "", fmt.Errorf("mcp server %q: tools/call %s: %w", c.config.Name, toolName, err)
	}

	// Parse the result content
	var callResult struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content"`
		IsError bool `json:"isError,omitempty"`
	}
	if err := json.Unmarshal(result, &callResult); err != nil {
		// Return raw result if we can't parse the structured format
		return string(result), nil
	}

	output := ""
	for _, c := range callResult.Content {
		if c.Text != "" {
			output += c.Text
		}
	}
	if callResult.IsError {
		return output, fmt.Errorf("mcp server %q: tool %s returned error", c.config.Name, toolName)
	}

	return output, nil
}

// Close shuts down the MCP client and its transport.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.transport != nil {
		err := c.transport.close()
		c.transport = nil
		c.connected = false
		return err
	}
	c.connected = false
	return nil
}

// ListTools returns the discovered tools for this server.
func (c *Client) ListTools() []Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Tool, len(c.tools))
	copy(out, c.tools)
	return out
}

// ServerName returns the configured name of this server.
func (c *Client) ServerName() string {
	return c.config.Name
}

// IsConnected returns whether the client is currently connected.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// newRequest creates a new JSON-RPC request with an auto-incrementing ID.
func (c *Client) newRequest(method string, params any) jsonRPCRequest {
	return jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID.Add(1),
		Method:  method,
		Params:  params,
	}
}
