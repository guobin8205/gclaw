package image

import (
	"testing"

	"github.com/openclaw/gclaw/internal/imagegen"
	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface compliance check.
func TestImageGenInterfaceCompliance(t *testing.T) {
	var _ tool.Tool = (*ImageGenTool)(nil)
}

func TestImageGenCheckNilFactory(t *testing.T) {
	orig := GenFactory
	GenFactory = nil
	defer func() { GenFactory = orig }()

	it := &ImageGenTool{}
	if it.Check() {
		t.Error("Check() should return false when GenFactory is nil")
	}
}

func TestImageGenCheckWithFactory(t *testing.T) {
	orig := GenFactory
	GenFactory = imagegen.NewFactory(nil, "")
	defer func() { GenFactory = orig }()

	it := &ImageGenTool{}
	if !it.Check() {
		t.Error("Check() should return true when GenFactory is set")
	}
}

func TestImageGenMetadata(t *testing.T) {
	it := &ImageGenTool{}

	if it.Name() != "ImageGen" {
		t.Errorf("Name() = %q, want 'ImageGen'", it.Name())
	}
	if it.Toolset() != "image" {
		t.Errorf("Toolset() = %q, want 'image'", it.Toolset())
	}
	if !it.ConcurrencySafe() {
		t.Error("ConcurrencySafe() should return true")
	}
	if it.RequiresApproval(nil) {
		t.Error("RequiresApproval() should return false")
	}
}

func TestImageGenInputSchema(t *testing.T) {
	it := &ImageGenTool{}
	schema := it.InputSchema()

	if schema.Type != "object" {
		t.Errorf("schema type = %q, want 'object'", schema.Type)
	}
	if _, ok := schema.Properties["prompt"]; !ok {
		t.Error("missing 'prompt' property")
	}
	if _, ok := schema.Properties["model"]; !ok {
		t.Error("missing 'model' property")
	}
	if _, ok := schema.Properties["size"]; !ok {
		t.Error("missing 'size' property")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "prompt" {
		t.Errorf("required = %v, want [prompt]", schema.Required)
	}

	// Check size enum
	sizeProp := schema.Properties["size"]
	if len(sizeProp.Enum) != 3 {
		t.Errorf("size enum count = %d, want 3", len(sizeProp.Enum))
	}
	enumMap := map[string]bool{}
	for _, v := range sizeProp.Enum {
		enumMap[v] = true
	}
	if !enumMap["landscape"] || !enumMap["square"] || !enumMap["portrait"] {
		t.Errorf("size enum = %v, want [landscape square portrait]", sizeProp.Enum)
	}
}

func TestResultFormatter(t *testing.T) {
	result := &imagegen.GenResult{URL: "https://example.com/image.png"}
	f := &resultFormatter{result: result, prompt: "a cat", size: "landscape"}
	got := f.String()

	if got != "Image generated:\nURL: https://example.com/image.png\nPrompt: a cat\nSize: landscape" {
		t.Errorf("unexpected format: %s", got)
	}

	// Test with empty size (should show "default")
	f2 := &resultFormatter{result: result, prompt: "a dog", size: ""}
	got2 := f2.String()
	if got2 != "Image generated:\nURL: https://example.com/image.png\nPrompt: a dog\nSize: default" {
		t.Errorf("unexpected format with empty size: %s", got2)
	}
}
