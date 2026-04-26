package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/openclaw/gclaw/internal/model"
	"github.com/openclaw/gclaw/internal/tool"
	"github.com/openclaw/gclaw/internal/perm"
)

// AutonomyLevel defines how independently the agent operates.
type AutonomyLevel int

const (
	Interactive    AutonomyLevel = 1
	SemiAutonomous AutonomyLevel = 2
	FullyAutonomous AutonomyLevel = 3
)

// Config holds agent configuration.
type Config struct {
	Model        model.Model
	Tools        *tool.Registry
	SystemPrompt string
	MaxTurns     int
	Autonomy     AutonomyLevel
	Permissions  *perm.Checker
}

// Agent is the core agent loop.
type Agent struct {
	cfg        Config
	messages   []model.Message
	turnCount  int
	totalUsage model.Usage

	// Concurrency control for autonomous mode
	busy bool
	mu   sync.Mutex
}

// New creates a new agent instance.
func New(cfg Config) *Agent {
	return &Agent{
		cfg:      cfg,
		messages: nil,
	}
}

// Reset clears the conversation history for a fresh start.
func (a *Agent) Reset() {
	a.messages = nil
}

// Messages returns a copy of the current message history.
func (a *Agent) Messages() []model.Message {
	cp := make([]model.Message, len(a.messages))
	copy(cp, a.messages)
	return cp
}

// Usage returns total usage so far.
func (a *Agent) Usage() model.Usage {
	return a.totalUsage
}

// Run executes the agent loop with the given user prompt.
// It returns the final text response and any error.
func (a *Agent) Run(ctx context.Context, prompt string) (string, error) {
	a.messages = append(a.messages, model.Message{Role: "user", Content: prompt})
	a.turnCount = 0

	for a.turnCount < a.cfg.MaxTurns {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		a.turnCount++
		slog.Debug("agent turn", "turn", a.turnCount, "messages", len(a.messages))

		response, err := a.cfg.Model.Call(ctx, model.CallParams{
			SystemPrompt: a.cfg.SystemPrompt,
			Messages:     a.messages,
			Tools:        a.cfg.Tools.List(),
			MaxTokens:    8192,
		})
		if err != nil {
			return "", fmt.Errorf("model call at turn %d: %w", a.turnCount, err)
		}

		a.totalUsage.InputTokens += response.Usage.InputTokens
		a.totalUsage.OutputTokens += response.Usage.OutputTokens

		// Append assistant response
		a.messages = append(a.messages, model.Message{
			Role:             "assistant",
			Content:          response.Text,
			ToolCalls:        response.ToolUse,
			ReasoningContent: response.ReasoningContent,
		})

		// No tool calls — agent is done
		if len(response.ToolUse) == 0 {
			return response.Text, nil
		}

		slog.Info("agent turn done", "turn", a.turnCount, "text_len", len(response.Text), "tool_calls", len(response.ToolUse), "tools", toolNames(response.ToolUse))
		if len(response.Text) > 0 {
			slog.Info("agent text response", "turn", a.turnCount, "text", truncate(response.Text, 200))
		}

		// Execute tool calls
		for _, tu := range response.ToolUse {
			if err := a.executeTool(ctx, tu); err != nil {
				slog.Error("tool execution failed", "tool", tu.Name, "error", err)
				a.messages = append(a.messages, model.Message{
					Role:    "tool",
					Content: fmt.Sprintf("Error: %v", err),
					ToolID:  tu.ID,
				})
			}
		}
	}

	return "", fmt.Errorf("exceeded max turns (%d) without completion", a.cfg.MaxTurns)
}

// RunStreaming executes the agent loop with streaming responses.
func (a *Agent) RunStreaming(ctx context.Context, prompt string, onText func(text string)) (string, error) {
	a.messages = append(a.messages, model.Message{Role: "user", Content: prompt})
	a.turnCount = 0

	for a.turnCount < a.cfg.MaxTurns {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		a.turnCount++

		events, err := a.cfg.Model.Stream(ctx, model.StreamParams{
			SystemPrompt: a.cfg.SystemPrompt,
			Messages:     a.messages,
			Tools:        a.cfg.Tools.List(),
			MaxTokens:    8192,
		})
		if err != nil {
			return "", fmt.Errorf("model stream at turn %d: %w", a.turnCount, err)
		}

		var fullText string
		var toolUses []model.ToolUse

	streamLoop:
		for event := range events {
			switch event.Type {
			case model.StreamEventText:
				fullText += event.Text
				if onText != nil {
					onText(event.Text)
				}
			case model.StreamEventToolUse:
				if event.ToolUse != nil {
					toolUses = append(toolUses, *event.ToolUse)
				}
			case model.StreamEventComplete:
				if event.Usage != nil {
					a.totalUsage.InputTokens += event.Usage.InputTokens
					a.totalUsage.OutputTokens += event.Usage.OutputTokens
				}
				break streamLoop
			case model.StreamEventError:
				return "", fmt.Errorf("stream error at turn %d", a.turnCount)
			}
		}

		a.messages = append(a.messages, model.Message{
			Role:      "assistant",
			Content:   fullText,
			ToolCalls: toolUses,
		})

		if len(toolUses) == 0 {
			return fullText, nil
		}

		for _, tu := range toolUses {
			if err := a.executeTool(ctx, tu); err != nil {
				slog.Error("tool execution failed", "tool", tu.Name, "error", err)
				a.messages = append(a.messages, model.Message{
					Role:    "tool",
					Content: fmt.Sprintf("Error: %v", err),
					ToolID:  tu.ID,
				})
			}
		}
	}

	return "", fmt.Errorf("exceeded max turns (%d) without completion", a.cfg.MaxTurns)
}

// executeTool checks permissions and runs a single tool.
func (a *Agent) executeTool(ctx context.Context, tu model.ToolUse) error {
	t, ok := a.cfg.Tools.Get(tu.Name)
	if !ok {
		return fmt.Errorf("unknown tool: %s", tu.Name)
	}

	// Permission check
	if a.cfg.Permissions != nil && t.RequiresApproval(tu.Input) {
		if a.cfg.Autonomy < SemiAutonomous {
			if err := a.cfg.Permissions.Check(tu.Name, tu.Input); err != nil {
				a.messages = append(a.messages, model.Message{
					Role:    "tool",
					Content: fmt.Sprintf("Denied: %v", err),
					ToolID:  tu.ID,
				})
				return nil
			}
		}
	}

	result, err := t.Execute(ctx, tu.Input)
	if err != nil {
		return err
	}

	slog.Info("tool result", "tool", tu.Name, "result_len", len(result.Content), "preview", truncate(result.Content, 150))

	a.messages = append(a.messages, model.Message{
		Role:    "tool",
		Content: result.Content,
		ToolID:  tu.ID,
	})

	return nil
}

// Submit feeds a message into the agent loop for autonomous mode.
// It returns (response, error) and supports concurrent calls via busy lock.
func (a *Agent) Submit(ctx context.Context, message string) (string, error) {
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return "", fmt.Errorf("agent is busy processing a previous request")
	}
	a.busy = true
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.busy = false
		a.mu.Unlock()
	}()

	return a.Run(ctx, message)
}

// IsBusy returns whether the agent is currently processing.
func (a *Agent) IsBusy() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.busy
}

func toolNames(tools []model.ToolUse) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
