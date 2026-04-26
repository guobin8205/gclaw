package model

import (
	"context"
	"encoding/json"
	"strings"
)

// MockProvider is a deterministic model for development and testing.
type MockProvider struct {
	id          string
	maxTokens   int
}

// NewMock creates a mock model that echoes structured responses.
func NewMock(id string) *MockProvider {
	return &MockProvider{
		id:        id,
		maxTokens: 128000,
	}
}

func (m *MockProvider) ID() string              { return m.id }
func (m *MockProvider) MaxTokens() int          { return m.maxTokens }
func (m *MockProvider) SupportsThinking() bool  { return false }
func (m *MockProvider) SupportsStreaming() bool { return true }
func (m *MockProvider) SupportsVision() bool    { return false }

func (m *MockProvider) CountTokens(messages []Message) int {
	count := 0
	for _, msg := range messages {
		count += len(msg.Content) / 4
	}
	return count
}

// Call generates a mock response. If tools are available and the user
// mentions "read" or "file", it returns a tool_use. Otherwise text.
func (m *MockProvider) Call(ctx context.Context, params CallParams) (*Response, error) {
	lastMsg := ""
	if len(params.Messages) > 0 {
		lastMsg = params.Messages[len(params.Messages)-1].Content
	}

	resp := &Response{
		Usage: Usage{
			InputTokens:  m.CountTokens(params.Messages),
			OutputTokens: 50,
		},
	}

	// Only trigger tool use for user messages (not tool results)
	lastUserMsg := lastMsg
	hasToolResult := false
	for i := len(params.Messages) - 1; i >= 0; i-- {
		msg := params.Messages[i]
		if msg.Role == "tool" {
			hasToolResult = true
		}
		if msg.Role == "user" && msg.Content != "" && !strings.HasPrefix(msg.Content, "[Earlier messages") {
			lastUserMsg = msg.Content
			break
		}
	}

	// If tools are available and the user asks to read/write, trigger tool use
	if len(params.Tools) > 0 && !hasToolResult {
		lower := strings.ToLower(lastUserMsg)
		if strings.Contains(lower, "read") || strings.Contains(lower, "file") {
			for _, t := range params.Tools {
				if t.Name == "ReadFile" {
					resp.ToolUse = append(resp.ToolUse, ToolUse{
						ID:   "mock-tu-1",
						Name: "ReadFile",
						Input: map[string]any{"file_path": "mock/path.txt"},
					})
					return resp, nil
				}
			}
		}
		if strings.Contains(lower, "bash") || strings.Contains(lower, "run") {
			for _, t := range params.Tools {
				if t.Name == "Bash" {
					resp.ToolUse = append(resp.ToolUse, ToolUse{
						ID:   "mock-tu-1",
						Name: "Bash",
						Input: map[string]any{"command": "echo hello"},
					})
					return resp, nil
				}
			}
		}
	}

	resp.Text = "Mock response: I received your message \"" + lastUserMsg + "\". No tools needed."
	return resp, nil
}

// Stream returns streaming events from the mock.
func (m *MockProvider) Stream(ctx context.Context, params StreamParams) (<-chan StreamEvent, error) {
	events := make(chan StreamEvent, 5)

	go func() {
		defer close(events)

		lastMsg := ""
		if len(params.Messages) > 0 {
			lastMsg = params.Messages[len(params.Messages)-1].Content
		}

		// Only trigger tool use for user messages
		lastUserMsg := lastMsg
		hasToolResult := false
		for i := len(params.Messages) - 1; i >= 0; i-- {
			msg := params.Messages[i]
			if msg.Role == "tool" {
				hasToolResult = true
			}
			if msg.Role == "user" && msg.Content != "" && !strings.HasPrefix(msg.Content, "[Earlier messages") {
				lastUserMsg = msg.Content
				break
			}
		}

		// If tools are available and relevant, use them
		if len(params.Tools) > 0 && !hasToolResult {
			lower := strings.ToLower(lastUserMsg)
			if strings.Contains(lower, "read") || strings.Contains(lower, "file") {
				for _, t := range params.Tools {
					if t.Name == "ReadFile" {
						events <- StreamEvent{
							Type: StreamEventToolUse,
							ToolUse: &ToolUse{
								ID:    "mock-tu-1",
								Name:  "ReadFile",
								Input: map[string]any{"file_path": "mock/path.txt"},
							},
						}
						events <- StreamEvent{
							Type:  StreamEventComplete,
							Usage: &Usage{InputTokens: 100, OutputTokens: 30},
						}
						return
					}
				}
			}
		}

		text := "Mock response: " + lastUserMsg
		for _, ch := range text {
			events <- StreamEvent{
				Type: StreamEventText,
				Text: string(ch),
			}
		}
		events <- StreamEvent{
			Type:  StreamEventComplete,
			Usage: &Usage{InputTokens: 100, OutputTokens: len(text)},
		}
	}()

	return events, nil
}

// ToolUseJSON is a helper to extract tool parameters as JSON-structured string.
func ToolUseJSON(input map[string]any) string {
	b, _ := json.Marshal(input)
	return string(b)
}
