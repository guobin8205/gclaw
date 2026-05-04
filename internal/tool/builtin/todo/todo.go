package todo

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface check.
var _ tool.Tool = (*TodoTool)(nil)

// TodoItem represents a single task in the todo list.
type TodoItem struct {
	ID          string
	Subject     string
	Description string
	Status      string
}

// TodoTool provides in-memory task tracking.
type TodoTool struct{}

func (t *TodoTool) Name() string        { return "Todo" }
func (t *TodoTool) Toolset() string     { return "todo" }
func (t *TodoTool) Description() string { return "Manage an in-memory task list (add, update, remove, list)." }
func (t *TodoTool) Check() bool         { return true }
func (t *TodoTool) ConcurrencySafe() bool                                  { return true }
func (t *TodoTool) RequiresApproval(params map[string]any) bool            { return false }

func (t *TodoTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"action": {
				Type:        "string",
				Description: "Action to perform",
				Enum:        []string{"list", "add", "update", "remove"},
			},
			"id": {
				Type:        "string",
				Description: "Task ID (required for update/remove)",
			},
			"subject": {
				Type:        "string",
				Description: "Task subject/title",
			},
			"description": {
				Type:        "string",
				Description: "Task description",
			},
			"status": {
				Type:        "string",
				Description: "Task status",
				Enum:        []string{"pending", "in_progress", "completed", "cancelled"},
			},
		},
		Required: []string{"action"},
	}
}

// Global in-memory store.
var (
	store   []TodoItem
	storeMu sync.Mutex
	nextID  int
)

// resetStore clears the global store (used in tests).
func resetStore() {
	storeMu.Lock()
	defer storeMu.Unlock()
	store = nil
	nextID = 0
}

func (t *TodoTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	action, _ := params["action"].(string)

	switch action {
	case "list":
		return t.doList()
	case "add":
		return t.doAdd(params)
	case "update":
		return t.doUpdate(params)
	case "remove":
		return t.doRemove(params)
	default:
		return tool.ToolResult{
			Content: fmt.Sprintf("Unknown action %q. Valid actions: list, add, update, remove.", action),
			IsError: true,
		}, nil
	}
}

func (t *TodoTool) doList() (tool.ToolResult, error) {
	storeMu.Lock()
	defer storeMu.Unlock()

	return tool.ToolResult{Content: formatTasks(store)}, nil
}

func (t *TodoTool) doAdd(params map[string]any) (tool.ToolResult, error) {
	subject, _ := params["subject"].(string)
	if subject == "" {
		return tool.ToolResult{
			Content: "Error: subject is required for add.",
			IsError: true,
		}, nil
	}
	description, _ := params["description"].(string)

	storeMu.Lock()
	defer storeMu.Unlock()

	nextID++
	item := TodoItem{
		ID:          strconv.Itoa(nextID),
		Subject:     subject,
		Description: description,
		Status:      "pending",
	}
	store = append(store, item)

	return tool.ToolResult{Content: formatTasks(store)}, nil
}

func (t *TodoTool) doUpdate(params map[string]any) (tool.ToolResult, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return tool.ToolResult{
			Content: "Error: id is required for update.",
			IsError: true,
		}, nil
	}

	storeMu.Lock()
	defer storeMu.Unlock()

	idx := findByID(id)
	if idx == -1 {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: task %s not found.", id),
			IsError: true,
		}, nil
	}

	if s, ok := params["status"].(string); ok && s != "" {
		store[idx].Status = s
	}
	if s, ok := params["subject"].(string); ok && s != "" {
		store[idx].Subject = s
	}
	if d, ok := params["description"].(string); ok && d != "" {
		store[idx].Description = d
	}

	return tool.ToolResult{Content: formatTasks(store)}, nil
}

func (t *TodoTool) doRemove(params map[string]any) (tool.ToolResult, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return tool.ToolResult{
			Content: "Error: id is required for remove.",
			IsError: true,
		}, nil
	}

	storeMu.Lock()
	defer storeMu.Unlock()

	idx := findByID(id)
	if idx == -1 {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: task %s not found.", id),
			IsError: true,
		}, nil
	}

	store = append(store[:idx], store[idx+1:]...)

	return tool.ToolResult{Content: formatTasks(store)}, nil
}

func findByID(id string) int {
	for i, item := range store {
		if item.ID == id {
			return i
		}
	}
	return -1
}

func formatTasks(tasks []TodoItem) string {
	if len(tasks) == 0 {
		return "No tasks."
	}
	result := fmt.Sprintf("%d tasks:\n", len(tasks))
	for _, t := range tasks {
		if t.Description != "" {
			result += fmt.Sprintf("  [%s] #%s %s — %s\n", t.Status, t.ID, t.Subject, t.Description)
		} else {
			result += fmt.Sprintf("  [%s] #%s %s\n", t.Status, t.ID, t.Subject)
		}
	}
	return result
}

func init() {
	tool.GlobalRegistry.Register(&TodoTool{})
}
