package openai

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

// OpenAI implements the Model interface for OpenAI and any OpenAI-compatible API.
// This covers: OpenAI, DeepSeek, Zhipu (GLM), Baidu Qianfan, Moonshot, etc.
type OpenAI struct {
	id              string
	apiKey          string
	baseURL         string
	endpoint        string // chat completions path, default /v1/chat/completions
	client          *http.Client
	extraHeaders    map[string]string
	supportsThinking bool
}

// New creates a new OpenAI-compatible provider.
func New(id, apiKey, baseURL string) *OpenAI {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &OpenAI{
		id:           id,
		apiKey:       apiKey,
		baseURL:      strings.TrimRight(baseURL, "/"),
		endpoint:     "/v1/chat/completions",
		client:       &http.Client{},
		extraHeaders: make(map[string]string),
	}
}

// SetHeader adds a custom header for providers that need it (e.g., Zhipu).
func (oa *OpenAI) SetHeader(key, value string) {
	oa.extraHeaders[key] = value
}

// SetBaseURL overrides the base URL.
func (oa *OpenAI) SetBaseURL(url string) {
	oa.baseURL = strings.TrimRight(url, "/")
}

// SetEndpoint overrides the chat completions endpoint path (default /v1/chat/completions).
// Use this for providers that use a different path, e.g. Zhipu uses /chat/completions.
func (oa *OpenAI) SetEndpoint(path string) {
	oa.endpoint = path
}

// SetThinking enables or disables the thinking/reasoning capability flag.
func (oa *OpenAI) SetThinking(v bool) {
	oa.supportsThinking = v
}

func (oa *OpenAI) ID() string              { return oa.id }
func (oa *OpenAI) MaxTokens() int          { return 128000 }
func (oa *OpenAI) SupportsThinking() bool  { return oa.supportsThinking }
func (oa *OpenAI) SupportsStreaming() bool { return true }
func (oa *OpenAI) SupportsVision() bool    { return false }

func (oa *OpenAI) CountTokens(messages []model.Message) int {
	count := 0
	for _, m := range messages {
		count += len(m.Content) / 4
	}
	return count
}

// --- OpenAI-compatible types (shared pattern with DeepSeek) ---

type oaiRequest struct {
	Model       string       `json:"model"`
	Messages    []oaiMessage `json:"messages"`
	Tools       []oaiTool    `json:"tools,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature float64      `json:"temperature,omitempty"`
	Stream      bool         `json:"stream"`
}

type oaiMessage struct {
	Role             string        `json:"role"`
	Content          any           `json:"content"`
	ToolCalls        []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string        `json:"tool_call_id,omitempty"`
	ReasoningContent string        `json:"reasoning_content,omitempty"`
}

type oaiToolCall struct {
	Index    int           `json:"index"`
	ID       string        `json:"id"`
	Type     string        `json:"type"`
	Function oaiToolCallFn `json:"function"`
}

type oaiToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded string
}

type oaiTool struct {
	Type     string  `json:"type"`
	Function oaiFunc `json:"function"`
}

type oaiFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type oaiResponse struct {
	Choices []oaiChoice `json:"choices"`
	Usage   *oaiUsage   `json:"usage,omitempty"`
}

type oaiChoice struct {
	Index        int        `json:"index"`
	Message      oaiMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type oaiStreamChunk struct {
	Choices []oaiStreamChoice `json:"choices"`
	Usage   *oaiUsage         `json:"usage,omitempty"`
}

type oaiStreamChoice struct {
	Index        int      `json:"index"`
	Delta        oaiDelta `json:"delta"`
	FinishReason *string  `json:"finish_reason,omitempty"`
}

type oaiDelta struct {
	Role             string        `json:"role,omitempty"`
	Content          string        `json:"content,omitempty"`
	ReasoningContent string        `json:"reasoning_content,omitempty"`
	ToolCalls        []oaiToolCall `json:"tool_calls,omitempty"`
}

// Call makes a non-streaming API call.
func (oa *OpenAI) Call(ctx context.Context, params model.CallParams) (*model.Response, error) {
	req := oa.buildRequest(params)
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", oa.baseURL+oa.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	oa.setHeaders(httpReq)

	resp, err := oa.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai error %d: %s", resp.StatusCode, string(b))
	}

	var or oaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return oa.toResponse(&or), nil
}

// Stream makes a streaming API call.
func (oa *OpenAI) Stream(ctx context.Context, params model.StreamParams) (<-chan model.StreamEvent, error) {
	req := oa.buildRequest(model.CallParams{
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", oa.baseURL+oa.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	oa.setHeaders(httpReq)

	resp, err := oa.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai stream: %w", err)
	}

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai error %d: %s", resp.StatusCode, string(b))
	}

	events := make(chan model.StreamEvent, 10)
	go oa.processStream(resp, events)
	return events, nil
}

func (oa *OpenAI) buildRequest(params model.CallParams) oaiRequest {
	req := oaiRequest{
		Model:       oa.id,
		MaxTokens:   params.MaxTokens,
		Temperature: params.Temperature,
	}

	if params.MaxTokens == 0 {
		req.MaxTokens = 8192
	}

	if params.SystemPrompt != "" {
		req.Messages = append(req.Messages, oaiMessage{
			Role:    "system",
			Content: params.SystemPrompt,
		})
	}

	for _, m := range params.Messages {
		om := oaiMessage{
			Role:             m.Role,
			Content:          m.Content,
			ToolCallID:       m.ToolID,
			ReasoningContent: m.ReasoningContent,
		}
		// Handle multimodal content (text + images)
		if len(m.Images) > 0 && (m.Role == "user" || m.Role == "system") {
			var content []any
			for _, img := range m.Images {
				url := img.URL
				if img.Data != "" {
					url = "data:" + img.MediaType + ";base64," + img.Data
				}
				if url != "" {
					content = append(content, map[string]any{
						"type":      "image_url",
						"image_url": map[string]any{"url": url},
					})
				}
			}
			content = append(content, map[string]any{"type": "text", "text": m.Content})
			om.Content = content
		}
		for _, tc := range m.ToolCalls {
			args, _ := json.Marshal(tc.Input)
			om.ToolCalls = append(om.ToolCalls, oaiToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: oaiToolCallFn{
					Name:      tc.Name,
					Arguments: string(args),
				},
			})
		}
		req.Messages = append(req.Messages, om)
	}

	for _, t := range params.Tools {
		req.Tools = append(req.Tools, oaiTool{
			Type: "function",
			Function: oaiFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	return req
}

func (oa *OpenAI) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+oa.apiKey)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range oa.extraHeaders {
		req.Header.Set(k, v)
	}
}

func (oa *OpenAI) toResponse(or *oaiResponse) *model.Response {
	resp := &model.Response{}

	if or.Usage != nil {
		resp.Usage = model.Usage{
			InputTokens:  or.Usage.PromptTokens,
			OutputTokens: or.Usage.CompletionTokens,
		}
	}

	for _, choice := range or.Choices {
		if s, ok := choice.Message.Content.(string); ok && s != "" {
			resp.Text += s
		}
		if choice.Message.ReasoningContent != "" {
			resp.ReasoningContent += choice.Message.ReasoningContent
		}
		for _, tc := range choice.Message.ToolCalls {
			input := make(map[string]any)
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
					input["_raw"] = tc.Function.Arguments
				}
			}
			resp.ToolUse = append(resp.ToolUse, model.ToolUse{
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}
	}

	return resp
}

func (oa *OpenAI) processStream(resp *http.Response, events chan<- model.StreamEvent) {
	defer close(events)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var fullText strings.Builder
	var fullReasoning strings.Builder
	// Accumulate tool calls across chunks (OpenAI sends them incrementally)
	toolCalls := make(map[int]*model.ToolUse)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			events <- model.StreamEvent{
				Type:             model.StreamEventComplete,
			ReasoningContent: fullReasoning.String(),
				Usage:            &model.Usage{OutputTokens: len(fullText.String()) / 4},
			}
			return
		}

		var chunk oaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Usage != nil {
			for _, tu := range toolCalls {
				events <- model.StreamEvent{
					Type:    model.StreamEventToolUse,
					ToolUse: tu,
				}
			}
			events <- model.StreamEvent{
				Type: model.StreamEventComplete,
				Usage: &model.Usage{
					InputTokens:  chunk.Usage.PromptTokens,
					OutputTokens: chunk.Usage.CompletionTokens,
				},
			}
			return
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.ReasoningContent != "" {
				fullReasoning.WriteString(choice.Delta.ReasoningContent)
			}
			if choice.Delta.Content != "" {
				fullText.WriteString(choice.Delta.Content)
				events <- model.StreamEvent{
					Type: model.StreamEventText,
					Text: choice.Delta.Content,
				}
			}
			for _, tc := range choice.Delta.ToolCalls {
				idx := tc.Index
				if _, exists := toolCalls[idx]; !exists {
					toolCalls[idx] = &model.ToolUse{
						ID:   tc.ID,
						Name: tc.Function.Name,
					}
				}
				tu := toolCalls[idx]
				if tc.ID != "" {
					tu.ID = tc.ID
				}
				if tc.Function.Name != "" {
					tu.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					// Accumulate arguments (chunk may contain partial JSON)
					if tu.Input == nil {
						tu.Input = make(map[string]any)
					}
					if existing, ok := tu.Input["_args"]; ok {
						tu.Input["_args"] = existing.(string) + tc.Function.Arguments
					} else {
						tu.Input["_args"] = tc.Function.Arguments
					}
				}
			}
		}
	}
}
