package ollama

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

// Ollama implements the Model interface for local Ollama servers.
type Ollama struct {
	id      string
	baseURL string
	client  *http.Client
}

// New creates a new Ollama provider.
func New(id, baseURL string) *Ollama {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &Ollama{
		id:      id,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

func (o *Ollama) ID() string              { return o.id }
func (o *Ollama) MaxTokens() int          { return 128000 }
func (o *Ollama) SupportsThinking() bool  { return false }
func (o *Ollama) SupportsStreaming() bool { return true }
func (o *Ollama) SupportsVision() bool    { return false }

func (o *Ollama) CountTokens(messages []model.Message) int {
	count := 0
	for _, m := range messages {
		count += len(m.Content) / 4
	}
	return count
}

// ollamaRequest mirrors Ollama's /api/chat format.
type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMsg     `json:"messages"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
	Options  ollamaOptions   `json:"options,omitempty"`
}

type ollamaOptions struct {
	NumPredict int     `json:"num_predict,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

type ollamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaTool struct {
	Type     string       `json:"type"`
	Function ollamaFunc   `json:"function"`
}

type ollamaFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ollamaResponse struct {
	Model     string      `json:"model"`
	Message   ollamaMsg   `json:"message"`
	Done      bool        `json:"done"`
	EvalCount int         `json:"eval_count,omitempty"`
	PromptEvalCount int   `json:"prompt_eval_count,omitempty"`
}

type ollamaStreamChunk struct {
	Message ollamaMsg `json:"message"`
	Done    bool      `json:"done"`
}

// Call makes a non-streaming API call to Ollama.
func (o *Ollama) Call(ctx context.Context, params model.CallParams) (*model.Response, error) {
	req := o.buildRequest(params)
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(b))
	}

	var or ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &model.Response{
		Text: or.Message.Content,
		Usage: model.Usage{
			InputTokens:  or.PromptEvalCount,
			OutputTokens: or.EvalCount,
		},
	}, nil
}

// Stream makes a streaming API call to Ollama.
func (o *Ollama) Stream(ctx context.Context, params model.StreamParams) (<-chan model.StreamEvent, error) {
	req := o.buildRequest(model.CallParams{
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama stream: %w", err)
	}

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(b))
	}

	events := make(chan model.StreamEvent, 10)
	go o.processStream(resp, events)
	return events, nil
}

func (o *Ollama) buildRequest(params model.CallParams) ollamaRequest {
	req := ollamaRequest{
		Model:  o.id,
		Stream: true,
		Options: ollamaOptions{
			NumPredict: params.MaxTokens,
			Temperature: params.Temperature,
		},
	}

	// Ollama uses system as the first message
	if params.SystemPrompt != "" {
		req.Messages = append(req.Messages, ollamaMsg{
			Role:    "system",
			Content: params.SystemPrompt,
		})
	}

	for _, m := range params.Messages {
		role := m.Role
		if role == "tool" {
			role = "user"
		}
		content := m.Content
		req.Messages = append(req.Messages, ollamaMsg{
			Role:    role,
			Content: content,
		})
	}

	for _, t := range params.Tools {
		req.Tools = append(req.Tools, ollamaTool{
			Type: "function",
			Function: ollamaFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	return req
}

func (o *Ollama) processStream(resp *http.Response, events chan<- model.StreamEvent) {
	defer close(events)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var fullText strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var chunk ollamaStreamChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}

		if chunk.Message.Content != "" {
			fullText.WriteString(chunk.Message.Content)
			events <- model.StreamEvent{
				Type: model.StreamEventText,
				Text: chunk.Message.Content,
			}
		}

		if chunk.Done {
			events <- model.StreamEvent{
				Type:  model.StreamEventComplete,
				Usage: &model.Usage{OutputTokens: len(fullText.String()) / 4},
			}
			return
		}
	}
}
