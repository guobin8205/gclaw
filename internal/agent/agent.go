package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"log/slog"
	"strings"
	"sync"

	ctxmgr "github.com/openclaw/gclaw/internal/context"
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
	ContextMgr   *ctxmgr.Manager // optional; nil means no auto-compaction
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

	// Tool event callbacks (set by caller for TUI integration)
	OnToolStart func(name, detail string)
	OnToolEnd   func(name string, output string, err error, duration time.Duration)

	// Interrupt system: allows injecting messages into a running agent loop
	interruptCh chan string

	// Context management for auto-compaction
	ctxMgr *ctxmgr.Manager
}

// New creates a new agent instance.
func New(cfg Config) *Agent {
	return &Agent{
		cfg:         cfg,
		messages:    nil,
		interruptCh: make(chan string, 8),
		ctxMgr:      cfg.ContextMgr,
	}
}

// SetModel replaces the agent's model at runtime.
func (a *Agent) SetModel(m model.Model) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.Model = m
}

// MaxTokens returns the model's maximum token context.
func (a *Agent) MaxTokens() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg.Model == nil {
		return 0
	}
	return a.cfg.Model.MaxTokens()
}

// Reset clears the conversation history for a fresh start.
func (a *Agent) Reset() {
	a.messages = nil
	if a.ctxMgr != nil {
		a.ctxMgr.Reset()
	}
	// Drain pending interrupts
	for {
		select {
		case <-a.interruptCh:
		default:
			return
		}
	}
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

		// Check for injected interrupts
		if msg := a.drainInterrupts(); msg != "" {
			a.messages = append(a.messages, model.Message{Role: "user", Content: msg})
			a.syncToManager()
			slog.Info("interrupt injected", "message_len", len(msg))
		}

		// Auto-compact if context threshold reached
		if a.ctxMgr != nil && a.ctxMgr.ShouldCompact() {
			slog.Info("context threshold reached, compressing", "ratio", a.ctxMgr.UsageRatio())
			a.syncToManager()
			a.ctxMgr.Compact(10)
			a.messages = a.ctxMgr.GetMessages()
		}

		response, err := a.cfg.Model.Call(ctx, model.CallParams{
			SystemPrompt: a.cfg.SystemPrompt,
			Messages:     a.messages,
			Tools:        a.cfg.Tools.AvailableTools(),
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

		// Check for injected interrupts
		if msg := a.drainInterrupts(); msg != "" {
			a.messages = append(a.messages, model.Message{Role: "user", Content: msg})
			a.syncToManager()
			slog.Info("interrupt injected", "message_len", len(msg))
		}

		// Auto-compact if context threshold reached
		if a.ctxMgr != nil && a.ctxMgr.ShouldCompact() {
			slog.Info("context threshold reached, compressing", "ratio", a.ctxMgr.UsageRatio())
			a.syncToManager()
			a.ctxMgr.Compact(10)
			a.messages = a.ctxMgr.GetMessages()
		}

		events, err := a.cfg.Model.Stream(ctx, model.StreamParams{
			SystemPrompt: a.cfg.SystemPrompt,
			Messages:     a.messages,
			Tools:        a.cfg.Tools.AvailableTools(),
			MaxTokens:    8192,
		})
		if err != nil {
			return "", fmt.Errorf("model stream at turn %d: %w", a.turnCount, err)
		}

		var fullText string
		var fullReasoning string
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
				fullReasoning = event.ReasoningContent
				slog.Debug("agent captured reasoning", "len", len(fullReasoning))
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
			Role:             "assistant",
			Content:          fullText,
			ToolCalls:        toolUses,
			ReasoningContent: fullReasoning,
		})
		slog.Debug("assistant msg appended", "reasoning_len", len(fullReasoning), "tools", len(toolUses))

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

	// Normalize array params: wrap bare scalars in single-element lists
	tool.NormalizeParams(t.InputSchema(), tu.Input)

	detail := toolDetail(tu.Name, tu.Input)
	if a.OnToolStart != nil {
		a.OnToolStart(tu.Name, detail)
	}
	start := time.Now()

	result, err := t.Execute(ctx, tu.Input)
	duration := time.Since(start)

	if a.OnToolEnd != nil {
		a.OnToolEnd(tu.Name, result.Content, err, duration)
	}

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

// Interrupt injects a message into the running agent loop.
// Non-blocking. Safe to call from any goroutine.
func (a *Agent) Interrupt(message string) {
	select {
	case a.interruptCh <- message:
		slog.Debug("interrupt queued", "message_len", len(message))
	default:
		slog.Warn("interrupt channel full, dropping message", "message", truncate(message, 100))
	}
}

// InterruptAndStop injects a message and cancels the agent loop via context.
func (a *Agent) InterruptAndStop(message string, cancel context.CancelFunc) {
	a.Interrupt(message)
	cancel()
}

// drainInterrupts reads all pending interrupts and merges them into one message.
func (a *Agent) drainInterrupts() string {
	var parts []string
	for {
		select {
		case msg := <-a.interruptCh:
			parts = append(parts, msg)
		default:
			if len(parts) > 0 {
				return "[User Interrupt] " + strings.Join(parts, "\n[User Interrupt] ")
			}
			return ""
		}
	}
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

// syncToManager pushes all current messages to the context manager.
func (a *Agent) syncToManager() {
	if a.ctxMgr == nil {
		return
	}
	a.ctxMgr.Reset()
	for _, msg := range a.messages {
		a.ctxMgr.AddMessage(msg)
	}
}


// toolDetail returns a short description of a tool call for display.
func toolDetail(name string, input map[string]any) string {
	if input == nil {
		return ""
	}
	// Strip unparsed _args if present
	if raw, ok := input["_args"].(string); ok {
		delete(input, "_args")
		var parsed map[string]any
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			for k, v := range parsed {
				input[k] = v
			}
		}
	}
	// Try common keys based on tool type
	keyOrder := []string{"command", "query", "pattern", "path", "url", "question", "input", "text"}
	for _, key := range keyOrder {
		if v, ok := input[key].(string); ok && v != "" {
			if len(v) > 80 {
				return v[:80] + "..."
			}
			return v
		}
	}
	// Fallback: first string value
	for _, v := range input {
		if s, ok := v.(string); ok && s != "" {
			if len(s) > 80 {
				return s[:80] + "..."
			}
			return s
		}
	}
	return ""
}
