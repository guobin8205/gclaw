package tui

import "testing"

func TestCompletionMatchSlash(t *testing.T) {
	engine := NewCompletionEngine(defaultCommands)
	items := engine.Match("/h")
	if len(items) == 0 {
		t.Fatal("expected matches")
	}
	found := false
	for _, item := range items {
		if item.Text == "/help" {
			found = true
		}
	}
	if !found {
		t.Error("expected /help in results")
	}
}

func TestCompletionNoMatch(t *testing.T) {
	engine := NewCompletionEngine(defaultCommands)
	items := engine.Match("/xyz")
	if len(items) != 0 {
		t.Fatalf("expected 0, got %d", len(items))
	}
}

func TestCompletionNotSlash(t *testing.T) {
	engine := NewCompletionEngine(defaultCommands)
	items := engine.Match("hello")
	if len(items) != 0 {
		t.Fatalf("expected 0 for non-slash")
	}
}
