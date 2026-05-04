package agent

import (
	"context"
	"testing"

	"github.com/openclaw/gclaw/internal/model"
	"github.com/openclaw/gclaw/internal/tool"
	"github.com/openclaw/gclaw/internal/tool/builtin/file"
	"github.com/openclaw/gclaw/internal/tool/builtin/shell"
)

func TestAgentRun(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(&file.ReadFileTool{})
	registry.Register(&shell.BashTool{})

	mock := model.NewMock("mock-model")

	ag := New(Config{
		Model:        mock,
		Tools:        registry,
		SystemPrompt: "You are a test agent.",
		MaxTurns:     5,
		Autonomy:     Interactive,
	})

	// Non-tool prompt should get a text response
	resp, err := ag.Run(context.Background(), "Hello, how are you?")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp == "" {
		t.Error("expected non-empty response")
	}
	t.Logf("Response: %s", resp)
}

func TestAgentToolUse(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(&file.ReadFileTool{})

	mock := model.NewMock("mock-model")

	ag := New(Config{
		Model:        mock,
		Tools:        registry,
		SystemPrompt: "You are a test agent.",
		MaxTurns:     5,
		Autonomy:     Interactive,
	})

	// "read" or "file" in prompt triggers tool use in mock
	resp, err := ag.Run(context.Background(), "Please read the file at /tmp/test.txt")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	t.Logf("Response: %s", resp)

	// Check that tool result was appended
	msgs := ag.Messages()
	if len(msgs) < 3 {
		t.Errorf("expected at least 3 messages (user, assistant, tool_result), got %d", len(msgs))
	}
}

func TestAgentMaxTurns(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(&file.ReadFileTool{})

	mock := model.NewMock("mock-model")

	ag := New(Config{
		Model:        mock,
		Tools:        registry,
		SystemPrompt: "You are a test agent.",
		MaxTurns:     1, // Only 1 turn, but tool use will require 2
		Autonomy:     Interactive,
	})

	_, err := ag.Run(context.Background(), "Please read the file at /tmp/test.txt")
	if err == nil {
		t.Error("expected max turns exceeded error")
	}
	t.Logf("Error (expected): %v", err)
}

func TestAgentRunStreaming(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(&file.ReadFileTool{})

	mock := model.NewMock("mock-model")

	ag := New(Config{
		Model:        mock,
		Tools:        registry,
		SystemPrompt: "You are a test agent.",
		MaxTurns:     5,
		Autonomy:     Interactive,
	})

	var texts []string
	resp, err := ag.RunStreaming(context.Background(), "Hello", func(text string) {
		texts = append(texts, text)
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	t.Logf("Streaming response: %s (received %d chunks)", resp, len(texts))
}
