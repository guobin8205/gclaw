package web

import (
	"strings"
	"testing"

	"github.com/openclaw/gclaw/internal/tool"
	"github.com/openclaw/gclaw/internal/websearch"
)

// Compile-time interface compliance check.
func TestWebSearchInterfaceCompliance(t *testing.T) {
	var _ tool.Tool = (*WebSearchTool)(nil)
}

func TestWebSearchCheckNilFactory(t *testing.T) {
	orig := SearchFactory
	SearchFactory = nil
	defer func() { SearchFactory = orig }()

	st := &WebSearchTool{}
	if st.Check() {
		t.Error("Check() should return false when SearchFactory is nil")
	}
}

func TestWebSearchCheckWithFactory(t *testing.T) {
	orig := SearchFactory
	SearchFactory = websearch.NewFactory(nil, "")
	defer func() { SearchFactory = orig }()

	st := &WebSearchTool{}
	if !st.Check() {
		t.Error("Check() should return true when SearchFactory is set")
	}
}

func TestFormatSearchResults(t *testing.T) {
	results := []websearch.SearchResult{
		{Title: "Go Programming", URL: "https://go.dev", Description: "The Go programming language"},
		{Title: "GitHub", URL: "https://github.com", Description: "Where the world builds software"},
		{Title: "No Description", URL: "https://example.com", Description: ""},
	}

	got := formatSearchResults("test query", results)

	if !strings.Contains(got, `Found 3 results for "test query":`) {
		t.Errorf("expected header, got: %s", got)
	}
	if !strings.Contains(got, "1. Go Programming") {
		t.Errorf("expected first result title, got: %s", got)
	}
	if !strings.Contains(got, "https://go.dev") {
		t.Errorf("expected first result URL, got: %s", got)
	}
	if !strings.Contains(got, "The Go programming language") {
		t.Errorf("expected first result description, got: %s", got)
	}
	if !strings.Contains(got, "2. GitHub") {
		t.Errorf("expected second result title, got: %s", got)
	}
	if !strings.Contains(got, "3. No Description") {
		t.Errorf("expected third result title, got: %s", got)
	}
}

func TestFormatSearchResultsEmpty(t *testing.T) {
	got := formatSearchResults("nothing", nil)
	if !strings.Contains(got, `Found 0 results for "nothing":`) {
		t.Errorf("expected empty header, got: %s", got)
	}
}

func TestWebSearchInputSchema(t *testing.T) {
	st := &WebSearchTool{}
	schema := st.InputSchema()

	if schema.Type != "object" {
		t.Errorf("schema type = %q, want 'object'", schema.Type)
	}
	if _, ok := schema.Properties["query"]; !ok {
		t.Error("missing 'query' property")
	}
	if _, ok := schema.Properties["max_results"]; !ok {
		t.Error("missing 'max_results' property")
	}
	if _, ok := schema.Properties["time_range"]; !ok {
		t.Error("missing 'time_range' property")
	}
	if _, ok := schema.Properties["backend"]; !ok {
		t.Error("missing 'backend' property")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "query" {
		t.Errorf("required = %v, want [query]", schema.Required)
	}
}

func TestWebSearchMetadata(t *testing.T) {
	st := &WebSearchTool{}

	if st.Name() != "WebSearch" {
		t.Errorf("Name() = %q, want 'WebSearch'", st.Name())
	}
	if st.Toolset() != "web" {
		t.Errorf("Toolset() = %q, want 'web'", st.Toolset())
	}
	if !st.ConcurrencySafe() {
		t.Error("ConcurrencySafe() should return true")
	}
	if st.RequiresApproval(nil) {
		t.Error("RequiresApproval() should return false")
	}
}
