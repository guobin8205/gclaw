package clarify

import (
	"context"
	"fmt"

	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface check.
var _ tool.Tool = (*ClarifyTool)(nil)

// Option represents a single choice presented to the user.
type Option struct {
	Label       string
	Description string
}

// Callback is set by the application to handle user interaction.
// When nil, the tool is unavailable (Check returns false).
var Callback func(question string, options []Option) (string, error)

// ClarifyTool asks the user a question and returns their answer.
type ClarifyTool struct{}

func (t *ClarifyTool) Name() string        { return "Clarify" }
func (t *ClarifyTool) Toolset() string     { return "clarify" }
func (t *ClarifyTool) Description() string { return "Ask the user a question and wait for their response. Use when you need clarification or a decision." }
func (t *ClarifyTool) Check() bool         { return Callback != nil }
func (t *ClarifyTool) ConcurrencySafe() bool     { return true }
func (t *ClarifyTool) RequiresApproval(map[string]any) bool { return false }

func (t *ClarifyTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"question": {
				Type:        "string",
				Description: "The question to ask the user",
			},
			"options": {
				Type:        "array",
				Description: "Optional list of choices for the user to select from",
				Items: &tool.Property{
					Type: "object",
					Properties: map[string]tool.Property{
						"label": {
							Type:        "string",
							Description: "Short option label",
						},
						"description": {
							Type:        "string",
							Description: "Explanation of this option",
						},
					},
				},
			},
		},
		Required: []string{"question"},
	}
}

func (t *ClarifyTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	question, _ := params["question"].(string)
	if question == "" {
		return tool.ToolResult{
			Content: "Error: question is required.",
			IsError: true,
		}, nil
	}

	var options []Option
	if raw, ok := params["options"]; ok {
		options = parseOptions(raw)
	}

	answer, err := Callback(question, options)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: %v", err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{Content: answer}, nil
}

// parseOptions converts a raw []any of map[string]any into []Option.
func parseOptions(raw any) []Option {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	options := make([]Option, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		opt := Option{}
		if v, ok := m["label"].(string); ok {
			opt.Label = v
		}
		if v, ok := m["description"].(string); ok {
			opt.Description = v
		}
		options = append(options, opt)
	}
	return options
}

func init() {
	tool.GlobalRegistry.Register(&ClarifyTool{})
}
