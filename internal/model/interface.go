package model

import "context"

// StreamEvent represents a single streaming chunk from an LLM.
type StreamEvent struct {
	Type       StreamEventType
	Text       string
	ToolUse    *ToolUse
	Usage      *Usage
	StopReason string
}

type StreamEventType int

const (
	StreamEventText StreamEventType = iota
	StreamEventToolUse
	StreamEventComplete
	StreamEventError
)

// ToolUse represents a tool call request from the model.
type ToolUse struct {
	ID    string
	Name  string
	Input map[string]any
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	ToolUseID string
	Content   string
	IsError   bool
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int
	OutputTokens int
	CacheRead    int
	CacheWrite   int
}

// Message represents a message in the conversation.
type Message struct {
	Role             string    // "system", "user", "assistant", "tool"
	Content          string
	ToolID           string    // for tool result messages
	ToolCalls        []ToolUse // for assistant messages with tool calls
	ReasoningContent string    // for DeepSeek-R1/V4 reasoning_content
}

// CallParams holds parameters for non-streaming model calls.
type CallParams struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDef
	MaxTokens    int
	Temperature  float64
}

// StreamParams holds parameters for streaming model calls.
type StreamParams struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDef
	MaxTokens    int
	Temperature  float64
}

// ToolDef describes a tool to the model.
type ToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// Response is a non-streaming model response.
type Response struct {
	Text             string
	Usage            Usage
	ToolUse          []ToolUse
	ReasoningContent string // DeepSeek-R1/V4 reasoning
}

// Model is the unified interface all LLM providers must implement.
type Model interface {
	ID() string
	MaxTokens() int
	SupportsThinking() bool
	SupportsStreaming() bool
	SupportsVision() bool

	Stream(ctx context.Context, params StreamParams) (<-chan StreamEvent, error)
	Call(ctx context.Context, params CallParams) (*Response, error)
	CountTokens(messages []Message) int
}
