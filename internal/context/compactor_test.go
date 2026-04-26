package context

import (
	"context"
	"testing"

	"github.com/openclaw/gclaw/internal/model"
)

// mockCompactorModel returns a fixed summary for testing.
type mockCompactorModel struct {
	summary string
}

func (m *mockCompactorModel) ID() string                         { return "mock-compressor" }
func (m *mockCompactorModel) MaxTokens() int                     { return 4096 }
func (m *mockCompactorModel) SupportsThinking() bool             { return false }
func (m *mockCompactorModel) SupportsStreaming() bool            { return false }
func (m *mockCompactorModel) SupportsVision() bool               { return false }
func (m *mockCompactorModel) CountTokens(_ []model.Message) int { return 100 }
func (m *mockCompactorModel) Stream(_ context.Context, _ model.StreamParams) (<-chan model.StreamEvent, error) {
	return nil, nil
}
func (m *mockCompactorModel) Call(_ context.Context, params model.CallParams) (*model.Response, error) {
	return &model.Response{
		Text:  m.summary,
		Usage: model.Usage{InputTokens: 100, OutputTokens: 50},
	}, nil
}

func TestLLMCompactorCompact(t *testing.T) {
	m := &mockCompactorModel{summary: "## Active Task\nTest task\n## Completed Actions\n1. Did something"}
	c := NewLLMCompactor(m, 1, 2, 2048)

	messages := []model.Message{
		{Role: "user", Content: "Initial task"},
		{Role: "assistant", Content: "Working on it"},
		{Role: "tool", Content: "file contents here", ToolID: "tool-1"},
		{Role: "assistant", Content: "Found something"},
		{Role: "user", Content: "Keep going"},
		{Role: "assistant", Content: "Latest response"},
	}

	result, err := c.Compact(messages, "")
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if result.MessagesBefore != 6 {
		t.Errorf("expected MessagesBefore=6, got %d", result.MessagesBefore)
	}
	// 1 (first) + 1 (summary) + 2 (recent) = 4
	if result.MessagesAfter != 4 {
		t.Errorf("expected MessagesAfter=4, got %d", result.MessagesAfter)
	}
}

func TestLLMCompactorNoMiddleMessages(t *testing.T) {
	m := &mockCompactorModel{summary: "summary"}
	c := NewLLMCompactor(m, 1, 2, 2048)

	// Only 3 messages = keepFirst(1) + keepRecent(2) = nothing to compress
	messages := []model.Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "response"},
		{Role: "user", Content: "last"},
	}

	result, err := c.Compact(messages, "")
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil result when nothing to compress")
	}
}

func TestLLMCompactorWithPreviousSummary(t *testing.T) {
	m := &mockCompactorModel{summary: "## Active Task\nUpdated task"}
	c := NewLLMCompactor(m, 1, 2, 2048)

	messages := []model.Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "middle content to compress"},
		{Role: "assistant", Content: "more middle"},
		{Role: "assistant", Content: "recent"},
		{Role: "user", Content: "latest"},
	}

	result, err := c.Compact(messages, "Previous summary text")
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestLLMCompactorDefaults(t *testing.T) {
	m := &mockCompactorModel{summary: "test"}

	// Test defaults kick in for zero values
	c := NewLLMCompactor(m, 0, 0, 0)
	if c.keepFirst != 1 {
		t.Errorf("expected keepFirst=1, got %d", c.keepFirst)
	}
	if c.keepRecent != 10 {
		t.Errorf("expected keepRecent=10, got %d", c.keepRecent)
	}
	if c.maxSummaryTokens != 2048 {
		t.Errorf("expected maxSummaryTokens=2048, got %d", c.maxSummaryTokens)
	}
}
