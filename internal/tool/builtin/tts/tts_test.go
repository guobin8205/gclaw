package tts

import (
	"testing"

	"github.com/openclaw/gclaw/internal/tool"
	ttsbackend "github.com/openclaw/gclaw/internal/tts"
)

// TestCompileTimeInterfaceCheck verifies TTSTool implements tool.Tool.
func TestCompileTimeInterfaceCheck(t *testing.T) {
	var _ tool.Tool = &TTSTool{}
}

// TestCheckFalseWhenFactoryNil verifies Check returns false when TTSFactory is nil.
func TestCheckFalseWhenFactoryNil(t *testing.T) {
	orig := TTSFactory
	TTSFactory = nil
	defer func() { TTSFactory = orig }()

	tt := &TTSTool{}
	if tt.Check() {
		t.Error("expected Check() to return false when TTSFactory is nil")
	}
}

// TestCheckTrueWhenFactorySet verifies Check returns true when TTSFactory is set.
func TestCheckTrueWhenFactorySet(t *testing.T) {
	orig := TTSFactory
	TTSFactory = nil
	defer func() { TTSFactory = orig }()

	TTSFactory = ttsbackend.NewFactory(nil, "")

	tt := &TTSTool{}
	if !tt.Check() {
		t.Error("expected Check() to return true when TTSFactory is set")
	}
}

// TestNameAndToolset verifies metadata.
func TestNameAndToolset(t *testing.T) {
	tt := &TTSTool{}
	if tt.Name() != "TTS" {
		t.Errorf("expected name TTS, got %s", tt.Name())
	}
	if tt.Toolset() != "tts" {
		t.Errorf("expected toolset tts, got %s", tt.Toolset())
	}
}

// TestConcurrencySafe verifies the tool is concurrency-safe.
func TestConcurrencySafe(t *testing.T) {
	tt := &TTSTool{}
	if !tt.ConcurrencySafe() {
		t.Error("expected ConcurrencySafe() to return true")
	}
}

// TestRequiresApproval verifies no approval is needed.
func TestRequiresApproval(t *testing.T) {
	tt := &TTSTool{}
	if tt.RequiresApproval(nil) {
		t.Error("expected RequiresApproval() to return false")
	}
}

// TestSchema verifies the input schema.
func TestSchema(t *testing.T) {
	tt := &TTSTool{}
	schema := tt.InputSchema()

	if schema.Type != "object" {
		t.Errorf("expected schema type object, got %s", schema.Type)
	}
	if _, ok := schema.Properties["text"]; !ok {
		t.Error("expected text property in schema")
	}
	if _, ok := schema.Properties["voice"]; !ok {
		t.Error("expected voice property in schema")
	}
	if _, ok := schema.Properties["backend"]; !ok {
		t.Error("expected backend property in schema")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "text" {
		t.Errorf("expected required [text], got %v", schema.Required)
	}
}
