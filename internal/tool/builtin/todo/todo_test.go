package todo

import (
	"context"
	"strings"
	"testing"

	"github.com/openclaw/gclaw/internal/tool"
)

func TestInterfaceCompliance(t *testing.T) {
	// Compile-time check already at package level via var _ tool.Tool = (*TodoTool)(nil).
	// This test confirms the tool can be used as a tool.Tool.
	var t2 tool.Tool = &TodoTool{}
	if t2.Name() != "Todo" {
		t.Errorf("expected name Todo, got %s", t2.Name())
	}
}

func TestListEmpty(t *testing.T) {
	resetStore()
	defer resetStore()

	tl := &TodoTool{}
	result, err := tl.Execute(context.Background(), map[string]any{"action": "list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected no error result")
	}
	if result.Content != "No tasks." {
		t.Errorf("expected 'No tasks.', got %q", result.Content)
	}
}

func TestAddAndList(t *testing.T) {
	resetStore()
	defer resetStore()

	tl := &TodoTool{}

	// Add a task
	result, err := tl.Execute(context.Background(), map[string]any{
		"action":      "add",
		"subject":     "Write tests",
		"description": "Cover all actions",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "1 tasks:") {
		t.Errorf("expected '1 tasks:' in output, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "[pending] #1 Write tests") {
		t.Errorf("expected task line in output, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "— Cover all actions") {
		t.Errorf("expected description in output, got %q", result.Content)
	}

	// List should show same result
	result, err = tl.Execute(context.Background(), map[string]any{"action": "list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "[pending] #1 Write tests") {
		t.Errorf("list: expected task, got %q", result.Content)
	}
}

func TestAddWithoutSubject(t *testing.T) {
	resetStore()
	defer resetStore()

	tl := &TodoTool{}
	result, err := tl.Execute(context.Background(), map[string]any{
		"action": "add",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing subject")
	}
	if !strings.Contains(result.Content, "subject is required") {
		t.Errorf("expected subject required message, got %q", result.Content)
	}
}

func TestUpdateStatus(t *testing.T) {
	resetStore()
	defer resetStore()

	tl := &TodoTool{}

	// Add task
	tl.Execute(context.Background(), map[string]any{
		"action":  "add",
		"subject": "Fix bug",
	})

	// Update status
	result, err := tl.Execute(context.Background(), map[string]any{
		"action": "update",
		"id":     "1",
		"status": "in_progress",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "[in_progress] #1 Fix bug") {
		t.Errorf("expected updated status, got %q", result.Content)
	}
}

func TestUpdateSubjectAndDescription(t *testing.T) {
	resetStore()
	defer resetStore()

	tl := &TodoTool{}

	tl.Execute(context.Background(), map[string]any{
		"action":  "add",
		"subject": "Old subject",
	})

	result, err := tl.Execute(context.Background(), map[string]any{
		"action":      "update",
		"id":          "1",
		"subject":     "New subject",
		"description": "New desc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "New subject") {
		t.Errorf("expected updated subject, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "New desc") {
		t.Errorf("expected updated description, got %q", result.Content)
	}
}

func TestRemove(t *testing.T) {
	resetStore()
	defer resetStore()

	tl := &TodoTool{}

	// Add two tasks
	tl.Execute(context.Background(), map[string]any{
		"action":  "add",
		"subject": "Task A",
	})
	tl.Execute(context.Background(), map[string]any{
		"action":  "add",
		"subject": "Task B",
	})

	// Remove first task
	result, err := tl.Execute(context.Background(), map[string]any{
		"action": "remove",
		"id":     "1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "1 tasks:") {
		t.Errorf("expected 1 task after remove, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "Task B") {
		t.Errorf("expected Task B to remain, got %q", result.Content)
	}
	if strings.Contains(result.Content, "Task A") {
		t.Errorf("expected Task A to be removed, got %q", result.Content)
	}
}

func TestUpdateNonExistent(t *testing.T) {
	resetStore()
	defer resetStore()

	tl := &TodoTool{}
	result, err := tl.Execute(context.Background(), map[string]any{
		"action": "update",
		"id":     "999",
		"status": "completed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for non-existent ID")
	}
	if !strings.Contains(result.Content, "not found") {
		t.Errorf("expected not found message, got %q", result.Content)
	}
}

func TestRemoveNonExistent(t *testing.T) {
	resetStore()
	defer resetStore()

	tl := &TodoTool{}
	result, err := tl.Execute(context.Background(), map[string]any{
		"action": "remove",
		"id":     "999",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for non-existent ID")
	}
	if !strings.Contains(result.Content, "not found") {
		t.Errorf("expected not found message, got %q", result.Content)
	}
}

func TestInvalidAction(t *testing.T) {
	resetStore()
	defer resetStore()

	tl := &TodoTool{}
	result, err := tl.Execute(context.Background(), map[string]any{
		"action": "explode",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for invalid action")
	}
	if !strings.Contains(result.Content, "Unknown action") {
		t.Errorf("expected unknown action message, got %q", result.Content)
	}
}

func TestMultipleItems(t *testing.T) {
	resetStore()
	defer resetStore()

	tl := &TodoTool{}

	// Add three tasks
	for i, subj := range []string{"Alpha", "Beta", "Gamma"} {
		result, err := tl.Execute(context.Background(), map[string]any{
			"action":  "add",
			"subject": subj,
		})
		if err != nil {
			t.Fatalf("add %d: unexpected error: %v", i, err)
		}
		if result.IsError {
			t.Fatalf("add %d: unexpected error: %s", i, result.Content)
		}
	}

	// List all
	result, err := tl.Execute(context.Background(), map[string]any{"action": "list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "3 tasks:") {
		t.Errorf("expected 3 tasks, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "#1 Alpha") ||
		!strings.Contains(result.Content, "#2 Beta") ||
		!strings.Contains(result.Content, "#3 Gamma") {
		t.Errorf("expected all three tasks, got %q", result.Content)
	}

	// Update second to completed
	result, _ = tl.Execute(context.Background(), map[string]any{
		"action": "update",
		"id":     "2",
		"status": "completed",
	})
	if !strings.Contains(result.Content, "[completed] #2 Beta") {
		t.Errorf("expected completed Beta, got %q", result.Content)
	}

	// Remove first
	result, _ = tl.Execute(context.Background(), map[string]any{
		"action": "remove",
		"id":     "1",
	})
	if !strings.Contains(result.Content, "2 tasks:") {
		t.Errorf("expected 2 tasks after remove, got %q", result.Content)
	}
}

func TestSchemaAndMetadata(t *testing.T) {
	tl := &TodoTool{}

	if tl.Name() != "Todo" {
		t.Errorf("expected name Todo, got %s", tl.Name())
	}
	if tl.Toolset() != "todo" {
		t.Errorf("expected toolset todo, got %s", tl.Toolset())
	}
	if !tl.Check() {
		t.Error("expected Check() to return true")
	}
	if !tl.ConcurrencySafe() {
		t.Error("expected ConcurrencySafe() to return true")
	}
	if tl.RequiresApproval(nil) {
		t.Error("expected RequiresApproval() to return false")
	}

	schema := tl.InputSchema()
	if schema.Type != "object" {
		t.Errorf("expected schema type object, got %s", schema.Type)
	}
	actionProp, ok := schema.Properties["action"]
	if !ok {
		t.Fatal("expected action property")
	}
	if len(actionProp.Enum) != 4 {
		t.Errorf("expected 4 action enums, got %d", len(actionProp.Enum))
	}
	statusProp, ok := schema.Properties["status"]
	if !ok {
		t.Fatal("expected status property")
	}
	if len(statusProp.Enum) != 4 {
		t.Errorf("expected 4 status enums, got %d", len(statusProp.Enum))
	}
	if len(schema.Required) != 1 || schema.Required[0] != "action" {
		t.Errorf("expected required [action], got %v", schema.Required)
	}
}
