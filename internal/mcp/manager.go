package mcp

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Manager manages multiple MCP server connections.
type Manager struct {
	clients map[string]*Client
	mu      sync.RWMutex
}

// NewManager creates a new MCP manager.
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*Client),
	}
}

// AddServer adds a new MCP server configuration.
func (m *Manager) AddServer(cfg ServerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	client := newClient(cfg)
	m.clients[cfg.Name] = client
}

// ConnectAll connects to all configured MCP servers.
// Returns the first error encountered, but continues connecting to others.
func (m *Manager) ConnectAll(ctx context.Context) error {
	m.mu.RLock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.RUnlock()

	var firstErr error
	for _, c := range clients {
		if err := c.Connect(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// DiscoverAll discovers tools from all connected MCP servers.
// Returns the first error encountered, but continues discovering from others.
func (m *Manager) DiscoverAll(ctx context.Context) error {
	m.mu.RLock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.RUnlock()

	var firstErr error
	for _, c := range clients {
		if err := c.DiscoverTools(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ListServers returns the names of all configured servers.
func (m *Manager) ListServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListTools returns the discovered tools for a specific server.
func (m *Manager) ListTools(serverName string) ([]Tool, error) {
	m.mu.RLock()
	c, ok := m.clients[serverName]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("mcp server %q not found", serverName)
	}
	return c.ListTools(), nil
}

// CallTool invokes a tool on a specific MCP server.
func (m *Manager) CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (string, error) {
	m.mu.RLock()
	c, ok := m.clients[serverName]
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("mcp server %q not found", serverName)
	}
	return c.CallTool(ctx, toolName, args)
}

// CloseAll shuts down all MCP server connections.
func (m *Manager) CloseAll() {
	m.mu.RLock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.RUnlock()

	for _, c := range clients {
		c.Close()
	}
}

// GetClient returns a client by server name.
func (m *Manager) GetClient(serverName string) (*Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clients[serverName]
	return c, ok
}

// ServerStatus returns the connection status of all servers.
func (m *Manager) ServerStatus() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]bool, len(m.clients))
	for name, c := range m.clients {
		status[name] = c.IsConnected()
	}
	return status
}
