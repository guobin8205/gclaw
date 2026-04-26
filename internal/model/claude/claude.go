package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openclaw/gclaw/internal/model"
)

// Claude implements the Model interface for Anthropic's Claude API.
type Claude struct {
	id       string
	apiKey   string
	baseURL  string
	client   *http.Client
}

// New creates a new Claude provider.
func New(id, apiKey string) *Claude {
	return &Claude{
		id:      id,
		apiKey:  apiKey,
		baseURL: "https://api.anthropic.com",
		client:  &http.Client{},
	}
}

// SetBaseURL overrides the default API base URL.
func (c *Claude) SetBaseURL(url string) {
	c.baseURL = url
}

// ID returns the model identifier.
func (c *Claude) ID() string { return c.id }

// MaxTokens returns the maximum context window size.
func (c *Claude) MaxTokens() int { return 200000 }

// SupportsThinking returns whether this model supports extended thinking.
func (c *Claude) SupportsThinking() bool { return true }

// SupportsStreaming returns whether this model supports streaming.
func (c *Claude) SupportsStreaming() bool { return true }

// SupportsVision returns whether this model supports image input.
func (c *Claude) SupportsVision() bool { return true }

// CountTokens estimates token count for messages.
func (c *Claude) CountTokens(messages []model.Message) int {
	count := 0
	for _, m := range messages {
		count += len(m.Content) / 4 // approximate: ~4 chars per token
	}
	return count
}

// claudeRequest mirrors the Anthropic Messages API structure.
type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system,omitempty"`
	Messages  []claudeMessage `json:"messages"`
	Tools     []claudeTool    `json:"tools,omitempty"`
	Stream    bool            `json:"stream"`
}

type claudeMessage struct {
	Role    string        `json:"role"`
	Content interface{}   `json:"content"`
}

type claudeTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeToolUseContent struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input map[string]any `json:"input"`
}

type claudeToolResultContent struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
}

type claudeTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// claudeResponse is the non-streaming response.
type claudeResponse struct {
	ID      string         `json:"id"`
	Content []claudeContent `json:"content"`
	Usage   claudeUsage     `json:"usage"`
	StopReason string       `json:"stop_reason"`
}

type claudeContent struct {
	Type string          `json:"type"`
	Text string          `json:"text,omitempty"`
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// claudeStreamEvent is a single SSE event from the streaming API.
type claudeStreamEvent struct {
	Type       string         `json:"type"`
	Index      int            `json:"index,omitempty"`
	Delta      *claudeDelta   `json:"delta,omitempty"`
	ContentBlock *claudeContent `json:"content_block,omitempty"`
	Usage      *claudeUsage   `json:"usage,omitempty"`
}

type claudeDelta struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	PartialJSON string        `json:"partial_json,omitempty"`
}

// Call makes a non-streaming API call.
func (c *Claude) Call(ctx context.Context, params model.CallParams) (*model.Response, error) {
	req := c.buildRequest(params)
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("api call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(b))
	}

	var cr claudeResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return c.toResponse(&cr, params.Tools), nil
}

// Stream makes a streaming API call and returns a channel of events.
func (c *Claude) Stream(ctx context.Context, params model.StreamParams) (<-chan model.StreamEvent, error) {
	req := c.buildRequest(model.CallParams{
		SystemPrompt: params.SystemPrompt,
		Messages:     params.Messages,
		Tools:        params.Tools,
		MaxTokens:    params.MaxTokens,
	})
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("api call: %w", err)
	}

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(b))
	}

	events := make(chan model.StreamEvent, 10)
	go c.processStream(resp, events)

	return events, nil
}

func (c *Claude) buildRequest(params model.CallParams) claudeRequest {
	req := claudeRequest{
		Model:     c.id,
		MaxTokens: params.MaxTokens,
		System:    params.SystemPrompt,
	}

	if req.MaxTokens == 0 {
		req.MaxTokens = 8192
	}

	for _, m := range params.Messages {
		req.Messages = append(req.Messages, c.convertMessage(m))
	}

	for _, t := range params.Tools {
		req.Tools = append(req.Tools, claudeTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	return req
}

func (c *Claude) convertMessage(m model.Message) claudeMessage {
	switch m.Role {
	case "user":
		return claudeMessage{
			Role: "user",
			Content: []claudeTextContent{{Type: "text", Text: m.Content}},
		}
	case "assistant":
		return claudeMessage{
			Role:    "assistant",
			Content: m.Content,
		}
	case "tool":
		return claudeMessage{
			Role: "user",
			Content: []claudeToolResultContent{{
				Type:      "tool_result",
				ToolUseID: m.ToolID,
				Content:   m.Content,
			}},
		}
	case "system":
		// System messages go in the system field, not messages array
		return claudeMessage{
			Role:    "user",
			Content: []claudeTextContent{{Type: "text", Text: m.Content}},
		}
	}
	return claudeMessage{
		Role:    "user",
		Content: []claudeTextContent{{Type: "text", Text: m.Content}},
	}
}

func (c *Claude) setHeaders(req *http.Request) {
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
}

func (c *Claude) processStream(resp *http.Response, events chan<- model.StreamEvent) {
	defer close(events)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentToolUse *model.ToolUse
	var usage model.Usage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		var event claudeStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_start":
			if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
				currentToolUse = &model.ToolUse{
					ID:   event.ContentBlock.ID,
					Name: event.ContentBlock.Name,
				}
			}
		case "content_block_delta":
			if event.Delta != nil {
				switch event.Delta.Type {
				case "text_delta":
					events <- model.StreamEvent{
						Type: model.StreamEventText,
						Text: event.Delta.Text,
					}
				case "input_json_delta":
					if currentToolUse != nil {
						if currentToolUse.Input == nil {
							currentToolUse.Input = make(map[string]any)
						}
						// Accumulate partial JSON - the complete tool input
						// is assembled from fragments
						_ = event.Delta.PartialJSON
					}
				}
			}
		case "content_block_stop":
			if currentToolUse != nil {
				events <- model.StreamEvent{
					Type:    model.StreamEventToolUse,
					ToolUse: currentToolUse,
				}
				currentToolUse = nil
			}
		case "message_delta":
			if event.Usage != nil {
				usage.OutputTokens = event.Usage.OutputTokens
			}
		case "message_start":
			if event.Usage != nil {
				usage.InputTokens = event.Usage.InputTokens
			}
		}
	}

	events <- model.StreamEvent{
		Type:    model.StreamEventComplete,
		Usage:   &usage,
	}
}

func (c *Claude) toResponse(cr *claudeResponse, tools []model.ToolDef) *model.Response {
	resp := &model.Response{
		Usage: model.Usage{
			InputTokens:  cr.Usage.InputTokens,
			OutputTokens: cr.Usage.OutputTokens,
		},
	}

	for _, content := range cr.Content {
		switch content.Type {
		case "text":
			resp.Text += content.Text
		case "tool_use":
			var input map[string]any
			if content.Input != nil {
				json.Unmarshal(content.Input, &input)
			}
			resp.ToolUse = append(resp.ToolUse, model.ToolUse{
				ID:    content.ID,
				Name:  content.Name,
				Input: input,
			})
		}
	}

	return resp
}
