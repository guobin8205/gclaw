package mcp

import (
	"encoding/json"
	"testing"
)

// TestServerConfigParsing verifies ServerConfig JSON/YAML field tags.
func TestServerConfigParsing(t *testing.T) {
	jsonData := `{"name": "test-server", "command": "node server.js", "url": "http://localhost:3000", "env": {"KEY": "value"}}`

	var cfg ServerConfig
	if err := json.Unmarshal([]byte(jsonData), &cfg); err != nil {
		t.Fatalf("failed to unmarshal ServerConfig: %v", err)
	}

	if cfg.Name != "test-server" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-server")
	}
	if cfg.Command != "node server.js" {
		t.Errorf("Command = %q, want %q", cfg.Command, "node server.js")
	}
	if cfg.URL != "http://localhost:3000" {
		t.Errorf("URL = %q, want %q", cfg.URL, "http://localhost:3000")
	}
	if cfg.Env["KEY"] != "value" {
		t.Errorf("Env[KEY] = %q, want %q", cfg.Env["KEY"], "value")
	}
}

// TestServerConfigMinimal verifies ServerConfig with only required fields.
func TestServerConfigMinimal(t *testing.T) {
	jsonData := `{"name": "minimal"}`

	var cfg ServerConfig
	if err := json.Unmarshal([]byte(jsonData), &cfg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if cfg.Name != "minimal" {
		t.Errorf("Name = %q, want %q", cfg.Name, "minimal")
	}
	if cfg.Command != "" {
		t.Errorf("Command should be empty, got %q", cfg.Command)
	}
	if cfg.URL != "" {
		t.Errorf("URL should be empty, got %q", cfg.URL)
	}
}

// TestManagerAddServer tests adding servers to the manager.
func TestManagerAddServer(t *testing.T) {
	m := NewManager()

	m.AddServer(ServerConfig{Name: "server-a", Command: "echo"})
	m.AddServer(ServerConfig{Name: "server-b", URL: "http://localhost:8080"})

	servers := m.ListServers()
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}

	// Should be sorted
	if servers[0] != "server-a" {
		t.Errorf("servers[0] = %q, want %q", servers[0], "server-a")
	}
	if servers[1] != "server-b" {
		t.Errorf("servers[1] = %q, want %q", servers[1], "server-b")
	}
}

// TestManagerListServersEmpty tests ListServers with no servers.
func TestManagerListServersEmpty(t *testing.T) {
	m := NewManager()
	servers := m.ListServers()
	if len(servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(servers))
	}
}

// TestManagerListToolsNotFound tests ListTools with unknown server.
func TestManagerListToolsNotFound(t *testing.T) {
	m := NewManager()
	_, err := m.ListTools("nonexistent")
	if err == nil {
		t.Error("expected error for unknown server")
	}
}

// TestManagerCallToolNotFound tests CallTool with unknown server.
func TestManagerCallToolNotFound(t *testing.T) {
	m := NewManager()
	_, err := m.CallTool(nil, "nonexistent", "tool", nil)
	if err == nil {
		t.Error("expected error for unknown server")
	}
}

// TestManagerGetClient tests GetClient.
func TestManagerGetClient(t *testing.T) {
	m := NewManager()
	m.AddServer(ServerConfig{Name: "test", Command: "echo"})

	c, ok := m.GetClient("test")
	if !ok {
		t.Fatal("expected to find client 'test'")
	}
	if c.ServerName() != "test" {
		t.Errorf("ServerName() = %q, want %q", c.ServerName(), "test")
	}

	_, ok = m.GetClient("nonexistent")
	if ok {
		t.Error("should not find nonexistent client")
	}
}

// TestManagerServerStatus tests ServerStatus.
func TestManagerServerStatus(t *testing.T) {
	m := NewManager()
	m.AddServer(ServerConfig{Name: "s1", Command: "echo"})
	m.AddServer(ServerConfig{Name: "s2", URL: "http://localhost:9999"})

	status := m.ServerStatus()
	if len(status) != 2 {
		t.Errorf("expected 2 entries, got %d", len(status))
	}
	// Not connected yet
	if status["s1"] {
		t.Error("s1 should not be connected")
	}
	if status["s2"] {
		t.Error("s2 should not be connected")
	}
}

// TestJSONRPCMessageFormatting verifies JSON-RPC request serialization.
func TestJSONRPCMessageFormatting(t *testing.T) {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
		Params:  map[string]any{},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", parsed["jsonrpc"])
	}
	if parsed["method"] != "tools/list" {
		t.Errorf("method = %v, want tools/list", parsed["method"])
	}
	if parsed["id"] != float64(1) {
		t.Errorf("id = %v, want 1", parsed["id"])
	}
}

// TestJSONRPCResponseParsing verifies JSON-RPC response deserialization.
func TestJSONRPCResponseParsing(t *testing.T) {
	data := `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"test_tool","description":"A test","inputSchema":{}}]}}`

	var resp jsonRPCResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.ID != 1 {
		t.Errorf("ID = %d, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("Error should be nil, got %v", resp.Error)
	}

	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "test_tool" {
		t.Errorf("tool name = %q, want %q", result.Tools[0].Name, "test_tool")
	}
}

// TestJSONRPCErrorParsing verifies error response parsing.
func TestJSONRPCErrorParsing(t *testing.T) {
	data := `{"jsonrpc":"2.0","id":2,"error":{"code":-32600,"message":"Invalid Request"}}`

	var resp jsonRPCResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error in response")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("error code = %d, want -32600", resp.Error.Code)
	}
	if resp.Error.Message != "Invalid Request" {
		t.Errorf("error message = %q, want %q", resp.Error.Message, "Invalid Request")
	}
}

// TestNewClient tests client creation.
func TestNewClient(t *testing.T) {
	cfg := ServerConfig{Name: "test", Command: "echo hello"}
	c := newClient(cfg)

	if c.ServerName() != "test" {
		t.Errorf("ServerName() = %q, want %q", c.ServerName(), "test")
	}
	if c.IsConnected() {
		t.Error("should not be connected initially")
	}
	tools := c.ListTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

// TestClientConnectNoTransport tests that Connect fails when neither command nor URL is set.
func TestClientConnectNoTransport(t *testing.T) {
	cfg := ServerConfig{Name: "empty"}
	c := newClient(cfg)

	err := c.Connect(nil)
	if err == nil {
		t.Error("expected error for empty config")
		c.Close()
	}
}

// TestNotificationFormatting tests that notifications are properly formatted.
func TestNotificationFormatting(t *testing.T) {
	notif := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}

	data, err := json.Marshal(notif)
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, hasID := parsed["id"]; hasID {
		t.Error("notification should not have id field when zero value is omitted")
	}
	if parsed["method"] != "notifications/initialized" {
		t.Errorf("method = %v, want notifications/initialized", parsed["method"])
	}
}
