package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openclaw/gclaw/internal/channel"
	"github.com/openclaw/gclaw/internal/cron"
	"github.com/openclaw/gclaw/internal/delegate"
	"github.com/openclaw/gclaw/internal/task"
	"github.com/openclaw/gclaw/internal/tool"
)

// Global references set by main.go.
var (
	DispatcherRef *delegate.Dispatcher
	CronSchedRef  *cron.Scheduler
	WeixinChRef   channel.Channel
	TaskMgrRef    *task.Manager
)

// ---- delegate_task ----

type delegateTaskTool struct{}

func (t *delegateTaskTool) Name() string        { return "delegate_task" }
func (t *delegateTaskTool) Toolset() string       { return "meta" }
func (t *delegateTaskTool) Description() string {
	return "Delegate a self-contained sub-task to a worker agent. The worker has its own tools but cannot delegate further. Use for parallelizable sub-tasks."
}
func (t *delegateTaskTool) Check() bool           { return DispatcherRef != nil }
func (t *delegateTaskTool) ConcurrencySafe() bool { return true }
func (t *delegateTaskTool) RequiresApproval(map[string]any) bool { return true }

func (t *delegateTaskTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"goal": {Type: "string", Description: "Clear, self-contained task description for the worker (single mode)"},
			"tasks": {
				Type:        "array",
				Description: "Array of tasks to delegate in parallel (batch mode). Each item must have a 'goal'.",
				Items: &tool.Property{
					Type: "object",
					Properties: map[string]tool.Property{
						"goal":    {Type: "string", Description: "Task goal"},
						"context": {Type: "string", Description: "Optional additional context"},
					},
				},
			},
			"max_concurrent": {Type: "integer", Description: "Max concurrent workers for batch mode (capped by dispatcher limit), default 3"},
			"timeout":        {Type: "string", Description: "Timeout (e.g., '30s', '5m'), default 5m"},
		},
		Required: []string{},
	}
}

func (t *delegateTaskTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	timeout := 5 * time.Minute
	if ts, ok := params["timeout"].(string); ok && ts != "" {
		if d, err := time.ParseDuration(ts); err == nil {
			timeout = d
		}
	}

	// Batch mode
	if tasksRaw, ok := params["tasks"].([]any); ok && len(tasksRaw) > 0 {
		tasks := make([]delegate.Task, 0, len(tasksRaw))
		for i, raw := range tasksRaw {
			m, ok := raw.(map[string]any)
			if !ok {
				return tool.ToolResult{Content: fmt.Sprintf("Error: task %d is not an object", i), IsError: true}, nil
			}
			goal, _ := m["goal"].(string)
			if goal == "" {
				return tool.ToolResult{Content: fmt.Sprintf("Error: task %d missing goal", i), IsError: true}, nil
			}
			ctxStr, _ := m["context"].(string)
			tasks = append(tasks, delegate.Task{
				ID:      fmt.Sprintf("task-%d", i),
				Goal:    goal,
				Context: ctxStr,
			})
		}

		results, err := DispatcherRef.BatchDelegate(ctx, tasks, 1, timeout, nil)
		if err != nil {
			return tool.ToolResult{Content: fmt.Sprintf("Batch delegate failed: %v", err), IsError: true}, nil
		}

		// Build JSON array response
		type resultItem struct {
			TaskID string `json:"task_id"`
			Goal   string `json:"goal"`
			Output string `json:"output"`
			Error  string `json:"error,omitempty"`
		}
		items := make([]resultItem, len(results))
		for i, r := range results {
			items[i] = resultItem{
				TaskID: r.TaskID,
				Goal:   tasks[i].Goal,
				Output: r.Output,
			}
			if r.Error != nil {
				items[i].Error = r.Error.Error()
			}
		}
		jsonBytes, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return tool.ToolResult{Content: fmt.Sprintf("Error encoding results: %v", err), IsError: true}, nil
		}
		return tool.ToolResult{Content: string(jsonBytes)}, nil
	}

	// Single mode
	goal, _ := params["goal"].(string)
	if goal == "" {
		return tool.ToolResult{Content: "Error: goal or tasks is required", IsError: true}, nil
	}

	result, err := DispatcherRef.Delegate(ctx, goal, 1, timeout)
	if err != nil {
		return tool.ToolResult{Content: fmt.Sprintf("Delegate failed: %v", err), IsError: true}, nil
	}
	return tool.ToolResult{Content: result.Output}, nil
}

func init() {
	tool.GlobalRegistry.Register(&delegateTaskTool{})
}

// ---- cron_list ----

type cronListTool struct{}

func (t *cronListTool) Name() string        { return "cron_list" }
func (t *cronListTool) Toolset() string       { return "meta" }
func (t *cronListTool) Description() string { return "List all configured cron jobs and their status." }
func (t *cronListTool) Check() bool           { return CronSchedRef != nil }
func (t *cronListTool) ConcurrencySafe() bool { return true }
func (t *cronListTool) RequiresApproval(map[string]any) bool { return false }

func (t *cronListTool) InputSchema() tool.Schema {
	return tool.Schema{Type: "object", Properties: map[string]tool.Property{}}
}

func (t *cronListTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	jobs := CronSchedRef.Jobs()
	if len(jobs) == 0 {
		return tool.ToolResult{Content: "No cron jobs configured."}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d cron jobs:\n", len(jobs)))
	for _, j := range jobs {
		sb.WriteString(fmt.Sprintf("- %s: schedule=%s next=%s runs=%d enabled=%v\n",
			j.Name, j.Schedule, j.NextRun.Format("15:04:05"), j.RunCount, j.Enabled))
	}
	return tool.ToolResult{Content: sb.String()}, nil
}

func init() {
	tool.GlobalRegistry.Register(&cronListTool{})
}

// ---- cron_run ----

type cronRunTool struct{}

func (t *cronRunTool) Name() string        { return "cron_run" }
func (t *cronRunTool) Toolset() string       { return "meta" }
func (t *cronRunTool) Description() string {
	return "Immediately execute a cron job by name (for testing). This does not affect its schedule."
}
func (t *cronRunTool) Check() bool           { return CronSchedRef != nil }
func (t *cronRunTool) ConcurrencySafe() bool { return false }
func (t *cronRunTool) RequiresApproval(map[string]any) bool { return true }

func (t *cronRunTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"name": {Type: "string", Description: "Name of the cron job to run"},
		},
		Required: []string{"name"},
	}
}

func (t *cronRunTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	name, _ := params["name"].(string)
	if name == "" {
		return tool.ToolResult{Content: "Error: name is required", IsError: true}, nil
	}

	result, err := CronSchedRef.RunNow(ctx, name)
	if err != nil {
		return tool.ToolResult{Content: fmt.Sprintf("Run failed: %v", err), IsError: true}, nil
	}
	return tool.ToolResult{Content: fmt.Sprintf("Job %q executed:\n%s", name, result)}, nil
}

func init() {
	tool.GlobalRegistry.Register(&cronRunTool{})
}

// ---- weixin_status ----

type weixinStatusTool struct{}

func (t *weixinStatusTool) Name() string        { return "weixin_status" }
func (t *weixinStatusTool) Toolset() string       { return "meta" }
func (t *weixinStatusTool) Description() string { return "Get the current status of the WeChat channel." }
func (t *weixinStatusTool) Check() bool           { return WeixinChRef != nil }
func (t *weixinStatusTool) ConcurrencySafe() bool { return true }
func (t *weixinStatusTool) RequiresApproval(map[string]any) bool { return false }

func (t *weixinStatusTool) InputSchema() tool.Schema {
	return tool.Schema{Type: "object", Properties: map[string]tool.Property{}}
}

func (t *weixinStatusTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	s := WeixinChRef.Status()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("WeChat: connected=%v\n", s.Connected))
	if s.Connected {
		sb.WriteString(fmt.Sprintf("Account: %s\n", s.AccountID))
		sb.WriteString(fmt.Sprintf("User: %s\n", s.UserID))
		if !s.LastMsgAt.IsZero() {
			sb.WriteString(fmt.Sprintf("Last message: %s\n", s.LastMsgAt.Format("15:04:05")))
		}
		sb.WriteString(fmt.Sprintf("Messages: %d\n", s.MsgCount))
	}
	return tool.ToolResult{Content: sb.String()}, nil
}

func init() {
	tool.GlobalRegistry.Register(&weixinStatusTool{})
}

// ---- tasks_list ----

type tasksListTool struct{}

func (t *tasksListTool) Name() string        { return "tasks_list" }
func (t *tasksListTool) Toolset() string       { return "meta" }
func (t *tasksListTool) Description() string { return "List all running and pending background tasks." }
func (t *tasksListTool) Check() bool           { return TaskMgrRef != nil }
func (t *tasksListTool) ConcurrencySafe() bool { return true }
func (t *tasksListTool) RequiresApproval(map[string]any) bool { return false }

func (t *tasksListTool) InputSchema() tool.Schema {
	return tool.Schema{Type: "object", Properties: map[string]tool.Property{}}
}

func (t *tasksListTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	tasks := TaskMgrRef.List()
	if len(tasks) == 0 {
		return tool.ToolResult{Content: "No active background tasks."}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d tasks:\n", len(tasks)))
	for _, t := range tasks {
		sb.WriteString(fmt.Sprintf("  [%s] %s %s\n", t.Status, t.Type, t.Description))
	}
	return tool.ToolResult{Content: sb.String()}, nil
}

func init() {
	tool.GlobalRegistry.Register(&tasksListTool{})
}
