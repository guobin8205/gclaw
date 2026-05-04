package clarify

import (
	"context"
	"fmt"
	"testing"

	"github.com/openclaw/gclaw/internal/tool"
)

func TestInterfaceCompliance(t *testing.T) {
	var _ tool.Tool = &ClarifyTool{}
}

func TestOpenEndedQuestion(t *testing.T) {
	origFn := Callback
	Callback = func(question string, options []Option) (string, error) {
		if question != "What is your name?" {
			t.Errorf("expected question 'What is your name?', got %q", question)
		}
		if len(options) != 0 {
			t.Errorf("expected no options, got %d", len(options))
		}
		return "Alice", nil
	}
	defer func() { Callback = origFn }()

	ct := &ClarifyTool{}
	result, err := ct.Execute(context.Background(), map[string]any{
		"question": "What is your name?",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error result: %s", result.Content)
	}
	if result.Content != "Alice" {
		t.Errorf("expected answer 'Alice', got %q", result.Content)
	}
}

func TestQuestionWithOptions(t *testing.T) {
	origFn := Callback
	Callback = func(question string, options []Option) (string, error) {
		if len(options) != 3 {
			t.Errorf("expected 3 options, got %d", len(options))
		}
		return options[1].Label, nil
	}
	defer func() { Callback = origFn }()

	ct := &ClarifyTool{}
	result, err := ct.Execute(context.Background(), map[string]any{
		"question": "Choose a color",
		"options": []any{
			map[string]any{"label": "Red", "description": "The color of fire"},
			map[string]any{"label": "Blue", "description": "The color of sky"},
			map[string]any{"label": "Green", "description": "The color of grass"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error result: %s", result.Content)
	}
	if result.Content != "Blue" {
		t.Errorf("expected answer 'Blue', got %q", result.Content)
	}
}

func TestNoCallback(t *testing.T) {
	origFn := Callback
	Callback = nil
	defer func() { Callback = origFn }()

	ct := &ClarifyTool{}
	if ct.Check() {
		t.Error("expected Check() to return false when Callback is nil")
	}
}

func TestCheckWithCallback(t *testing.T) {
	origFn := Callback
	Callback = func(question string, options []Option) (string, error) {
		return "", nil
	}
	defer func() { Callback = origFn }()

	ct := &ClarifyTool{}
	if !ct.Check() {
		t.Error("expected Check() to return true when Callback is set")
	}
}

func TestMissingQuestion(t *testing.T) {
	origFn := Callback
	Callback = func(question string, options []Option) (string, error) {
		return "should not be called", nil
	}
	defer func() { Callback = origFn }()

	ct := &ClarifyTool{}
	result, err := ct.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing question")
	}
	if result.Content != "Error: question is required." {
		t.Errorf("expected question required message, got %q", result.Content)
	}
}

func TestEmptyQuestion(t *testing.T) {
	origFn := Callback
	Callback = func(question string, options []Option) (string, error) {
		return "should not be called", nil
	}
	defer func() { Callback = origFn }()

	ct := &ClarifyTool{}
	result, err := ct.Execute(context.Background(), map[string]any{
		"question": "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for empty question")
	}
}

func TestCallbackError(t *testing.T) {
	origFn := Callback
	Callback = func(question string, options []Option) (string, error) {
		return "", fmt.Errorf("user cancelled")
	}
	defer func() { Callback = origFn }()

	ct := &ClarifyTool{}
	result, err := ct.Execute(context.Background(), map[string]any{
		"question": "Continue?",
	})
	if err != nil {
		t.Fatalf("expected no Go error, got: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result")
	}
	if result.Content != "Error: user cancelled" {
		t.Errorf("expected error message 'Error: user cancelled', got %q", result.Content)
	}
}

func TestParseOptions(t *testing.T) {
	raw := []any{
		map[string]any{"label": "Yes", "description": "Proceed with action"},
		map[string]any{"label": "No", "description": "Cancel action"},
		map[string]any{"label": "Maybe"},
	}
	options := parseOptions(raw)
	if len(options) != 3 {
		t.Fatalf("expected 3 options, got %d", len(options))
	}
	if options[0].Label != "Yes" {
		t.Errorf("expected label 'Yes', got %q", options[0].Label)
	}
	if options[0].Description != "Proceed with action" {
		t.Errorf("expected description 'Proceed with action', got %q", options[0].Description)
	}
	if options[1].Label != "No" {
		t.Errorf("expected label 'No', got %q", options[1].Label)
	}
	if options[2].Label != "Maybe" {
		t.Errorf("expected label 'Maybe', got %q", options[2].Label)
	}
	if options[2].Description != "" {
		t.Errorf("expected empty description, got %q", options[2].Description)
	}
}

func TestParseOptionsNil(t *testing.T) {
	options := parseOptions(nil)
	if options != nil {
		t.Errorf("expected nil for nil input, got %v", options)
	}
}

func TestParseOptionsWrongType(t *testing.T) {
	options := parseOptions("not an array")
	if options != nil {
		t.Errorf("expected nil for wrong type, got %v", options)
	}
}

func TestSchemaAndMetadata(t *testing.T) {
	ct := &ClarifyTool{}

	if ct.Name() != "Clarify" {
		t.Errorf("expected name Clarify, got %s", ct.Name())
	}
	if ct.Toolset() != "clarify" {
		t.Errorf("expected toolset clarify, got %s", ct.Toolset())
	}
	if !ct.ConcurrencySafe() {
		t.Error("expected ConcurrencySafe() to return true")
	}
	if ct.RequiresApproval(nil) {
		t.Error("expected RequiresApproval() to return false")
	}

	schema := ct.InputSchema()
	if schema.Type != "object" {
		t.Errorf("expected schema type object, got %s", schema.Type)
	}
	if _, ok := schema.Properties["question"]; !ok {
		t.Error("expected question property")
	}
	optProp, ok := schema.Properties["options"]
	if !ok {
		t.Fatal("expected options property")
	}
	if optProp.Type != "array" {
		t.Errorf("expected options type array, got %s", optProp.Type)
	}
	if optProp.Items == nil {
		t.Fatal("expected options items to be non-nil")
	}
	if optProp.Items.Type != "object" {
		t.Errorf("expected items type object, got %s", optProp.Items.Type)
	}
	if _, ok := optProp.Items.Properties["label"]; !ok {
		t.Error("expected items label property")
	}
	if _, ok := optProp.Items.Properties["description"]; !ok {
		t.Error("expected items description property")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "question" {
		t.Errorf("expected required [question], got %v", schema.Required)
	}
}
