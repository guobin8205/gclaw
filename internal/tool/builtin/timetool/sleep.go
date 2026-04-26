package timetool

import (
	"context"
	"fmt"
	"time"

	"github.com/openclaw/gclaw/internal/autonomous"
	"github.com/openclaw/gclaw/internal/tool"
)

// SleepTool pauses the autonomous agent for a configurable duration.
type SleepTool struct {
	Sleeper *autonomous.Sleeper
}

func (t *SleepTool) Name() string        { return "SleepTool" }
func (t *SleepTool) Toolset() string     { return "time" }
func (t *SleepTool) Description() string {
	return "Sleep for a specified duration when there is nothing useful to do. The agent will wake when the duration expires or when an external event occurs."
}
func (t *SleepTool) Check() bool           { return t.Sleeper != nil }
func (t *SleepTool) ConcurrencySafe() bool { return true }
func (t *SleepTool) RequiresApproval(params map[string]any) bool {
	return false
}

func (t *SleepTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"duration": {Type: "string", Description: "How long to sleep (e.g. 30s, 5m, 1h). Max 1 hour."},
			"reason":   {Type: "string", Description: "Why the agent is sleeping (for logging)"},
		},
		Required: []string{"duration"},
	}
}

func (t *SleepTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	durationStr, _ := params["duration"].(string)
	if durationStr == "" {
		durationStr = "5m"
	}

	d, err := time.ParseDuration(durationStr)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Invalid duration: %s. Use format like 30s, 5m, 1h.", durationStr),
			IsError: true,
		}, nil
	}

	if d > time.Hour {
		d = time.Hour
	}

	reason, _ := params["reason"].(string)
	if reason == "" {
		reason = "no work to do"
	}

	if t.Sleeper != nil {
		t.Sleeper.RequestSleep(d)
		return tool.ToolResult{
			Content: fmt.Sprintf("Sleeping for %s (reason: %s). Will wake on event or timeout.", d, reason),
		}, nil
	}

	return tool.ToolResult{
		Content: "SleepTool called outside autonomous mode. This is a no-op in interactive mode.",
	}, nil
}

func init() {
	tool.GlobalRegistry.Register(&SleepTool{})
}
