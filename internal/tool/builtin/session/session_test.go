package session

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/openclaw/gclaw/internal/session"
	"github.com/openclaw/gclaw/internal/tool"
)

// mockStore implements session.Store for testing.
type mockStore struct {
	results []session.SearchResult
	err     error
}

func (m *mockStore) CreateSession(agentType string) (string, error)        { return "s1", nil }
func (m *mockStore) AddMessage(sessionID, role, content, toolName, toolCalls, reasoning string) error {
	return nil
}
func (m *mockStore) GetMessages(sessionID string) ([]session.Message, error) {
	return nil, nil
}
func (m *mockStore) Search(query string, limit int) ([]session.SearchResult, error) {
	return m.results, m.err
}
func (m *mockStore) ListSessions() ([]session.Info, error) { return nil, nil }
func (m *mockStore) Stats() session.Stats                  { return session.Stats{} }
func (m *mockStore) Close() error                          { return nil }

// TestInterfaceCompliance verifies SessionSearchTool implements tool.Tool at compile time.
func TestInterfaceCompliance(t *testing.T) {
	var _ tool.Tool = (*SessionSearchTool)(nil)
}

// TestNameAndToolset tests the Name and Toolset methods.
func TestNameAndToolset(t *testing.T) {
	tt := &SessionSearchTool{}
	if tt.Name() != "SessionSearch" {
		t.Errorf("Name() = %q, want %q", tt.Name(), "SessionSearch")
	}
	if tt.Toolset() != "session" {
		t.Errorf("Toolset() = %q, want %q", tt.Toolset(), "session")
	}
}

// TestConcurrencySafeAndRequiresApproval tests the safety methods.
func TestConcurrencySafeAndRequiresApproval(t *testing.T) {
	tt := &SessionSearchTool{}
	if !tt.ConcurrencySafe() {
		t.Error("ConcurrencySafe() should return true")
	}
	if tt.RequiresApproval(nil) {
		t.Error("RequiresApproval() should return false")
	}
}

// TestCheckNil tests that Check returns false when StoreRef is nil.
func TestCheckNil(t *testing.T) {
	orig := StoreRef
	defer func() { StoreRef = orig }()

	StoreRef = nil
	tt := &SessionSearchTool{}
	if tt.Check() {
		t.Error("Check() should return false when StoreRef is nil")
	}
}

// TestCheckSet tests that Check returns true when StoreRef is set.
func TestCheckSet(t *testing.T) {
	orig := StoreRef
	defer func() { StoreRef = orig }()

	StoreRef = &mockStore{}
	tt := &SessionSearchTool{}
	if !tt.Check() {
		t.Error("Check() should return true when StoreRef is set")
	}
}

// TestExecuteNoStore tests that Execute returns an error when StoreRef is nil.
func TestExecuteNoStore(t *testing.T) {
	orig := StoreRef
	defer func() { StoreRef = orig }()

	StoreRef = nil
	tt := &SessionSearchTool{}
	result, err := tt.Execute(context.Background(), map[string]any{"query": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when StoreRef is nil")
	}
	if !strings.Contains(result.Content, "not available") {
		t.Errorf("expected 'not available' message, got: %s", result.Content)
	}
}

// TestExecuteMissingQuery tests that Execute returns an error when query is missing.
func TestExecuteMissingQuery(t *testing.T) {
	orig := StoreRef
	defer func() { StoreRef = orig }()

	StoreRef = &mockStore{}
	tt := &SessionSearchTool{}

	result, err := tt.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing query")
	}
	if !strings.Contains(result.Content, "query") {
		t.Errorf("expected 'query' in error message, got: %s", result.Content)
	}
}

// TestExecuteWithResults tests Execute with mock search results.
func TestExecuteWithResults(t *testing.T) {
	orig := StoreRef
	defer func() { StoreRef = orig }()

	StoreRef = &mockStore{
		results: []session.SearchResult{
			{
				SessionID: "abc-123",
				Role:      "user",
				Content:   "How do I implement a REST API in Go?",
				Timestamp: time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC),
			},
			{
				SessionID: "def-456",
				Role:      "assistant",
				Content:   "You can use the standard library net/http package or a framework like Gin.",
				Timestamp: time.Date(2026, 5, 2, 14, 0, 0, 0, time.UTC),
			},
		},
	}

	tt := &SessionSearchTool{}
	result, err := tt.Execute(context.Background(), map[string]any{
		"query": "REST API",
		"limit": float64(10),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Found 2 matching sessions") {
		t.Errorf("expected 'Found 2 matching sessions', got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "abc-123") {
		t.Errorf("expected session ID 'abc-123', got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "def-456") {
		t.Errorf("expected session ID 'def-456', got: %s", result.Content)
	}
}

// TestExecuteEmptyResults tests Execute with no search results.
func TestExecuteEmptyResults(t *testing.T) {
	orig := StoreRef
	defer func() { StoreRef = orig }()

	StoreRef = &mockStore{results: nil}
	tt := &SessionSearchTool{}

	result, err := tt.Execute(context.Background(), map[string]any{"query": "nonexistent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Found 0 matching sessions") {
		t.Errorf("expected 'Found 0 matching sessions', got: %s", result.Content)
	}
}

// TestFormatResultsTruncation tests that long content is truncated to 200 chars.
func TestFormatResultsTruncation(t *testing.T) {
	longContent := strings.Repeat("a", 300)
	results := []session.SearchResult{
		{
			SessionID: "long-1",
			Role:      "user",
			Content:   longContent,
			Timestamp: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	out := formatResults(results)
	if strings.Contains(out, strings.Repeat("a", 201)) {
		t.Error("content should be truncated to 200 chars")
	}
	if !strings.Contains(out, strings.Repeat("a", 200)) {
		t.Error("content should contain first 200 chars")
	}
}

// TestInputSchema tests the schema is correctly defined.
func TestInputSchema(t *testing.T) {
	tt := &SessionSearchTool{}
	schema := tt.InputSchema()

	if schema.Type != "object" {
		t.Errorf("schema type = %q, want 'object'", schema.Type)
	}
	if len(schema.Properties) != 2 {
		t.Errorf("expected 2 properties, got %d", len(schema.Properties))
	}
	if _, ok := schema.Properties["query"]; !ok {
		t.Error("missing 'query' property")
	}
	if _, ok := schema.Properties["limit"]; !ok {
		t.Error("missing 'limit' property")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "query" {
		t.Errorf("required = %v, want [query]", schema.Required)
	}
}
