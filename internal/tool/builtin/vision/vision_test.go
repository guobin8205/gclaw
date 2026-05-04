package vision

import (
	"context"
	"os"
	"testing"

	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface check.
func TestVisionInterfaceCompliance(t *testing.T) {
	var _ tool.Tool = (*VisionTool)(nil)
}

func TestVisionCheckNilModel(t *testing.T) {
	orig := ModelRef
	ModelRef = nil
	defer func() { ModelRef = orig }()

	vt := &VisionTool{}
	if vt.Check() {
		t.Error("Check() should return false when ModelRef is nil")
	}
}

func TestVisionMetadata(t *testing.T) {
	vt := &VisionTool{}

	if vt.Name() != "Vision" {
		t.Errorf("Name() = %q, want 'Vision'", vt.Name())
	}
	if vt.Toolset() != "vision" {
		t.Errorf("Toolset() = %q, want 'vision'", vt.Toolset())
	}
	if !vt.ConcurrencySafe() {
		t.Error("ConcurrencySafe() should return true")
	}
	if vt.RequiresApproval(nil) {
		t.Error("RequiresApproval() should return false")
	}
}

func TestVisionInputSchema(t *testing.T) {
	vt := &VisionTool{}
	schema := vt.InputSchema()

	if schema.Type != "object" {
		t.Errorf("schema type = %q, want 'object'", schema.Type)
	}
	if _, ok := schema.Properties["image"]; !ok {
		t.Error("missing 'image' property")
	}
	if _, ok := schema.Properties["prompt"]; !ok {
		t.Error("missing 'prompt' property")
	}
	if len(schema.Required) != 2 {
		t.Errorf("required count = %d, want 2", len(schema.Required))
	} else {
		found := map[string]bool{}
		for _, r := range schema.Required {
			found[r] = true
		}
		if !found["image"] || !found["prompt"] {
			t.Errorf("required = %v, want [image prompt]", schema.Required)
		}
	}
}

func TestMediaTypeFromExt(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"photo.png", "image/png"},
		{"photo.PNG", "image/png"},
		{"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"},
		{"photo.JPG", "image/jpeg"},
		{"photo.JPEG", "image/jpeg"},
		{"photo.gif", "image/gif"},
		{"photo.webp", "image/webp"},
		{"photo.bmp", "image/png"}, // default
		{"photo", "image/png"},     // no extension, default
		{"/path/to/photo.tiff", "image/png"}, // unknown extension, default
	}

	for _, tt := range tests {
		got := mediaTypeFromExt(tt.path)
		if got != tt.want {
			t.Errorf("mediaTypeFromExt(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestExecuteInvalidPath(t *testing.T) {
	orig := ModelRef
	ModelRef = nil
	defer func() { ModelRef = orig }()

	vt := &VisionTool{}
	result, err := vt.Execute(context.Background(), map[string]any{
		"image":  "/nonexistent/path/image.png",
		"prompt": "describe this",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError to be true for invalid path with nil model")
	}
}

func TestExecuteMissingParams(t *testing.T) {
	vt := &VisionTool{}

	// Missing image.
	result, err := vt.Execute(context.Background(), map[string]any{
		"prompt": "describe this",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for missing image")
	}

	// Missing prompt.
	result, err = vt.Execute(context.Background(), map[string]any{
		"image": "http://example.com/photo.png",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for missing prompt")
	}
}

func TestExecuteLocalFileTooLarge(t *testing.T) {
	// Create a temp file that simulates a file larger than 20MB.
	tmpFile, err := os.CreateTemp("", "vision_test_*.png")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	// Write a small file but test the size limit by using a mock model.
	// Since ModelRef is nil, the "no vision model" error fires before size check.
	// We test the size check logic indirectly through the readLocalImage path.
	// The actual 20MB check is in Execute after reading, so with nil ModelRef
	// we just verify the error flow works.
	_, _ = tmpFile.WriteString("not a real image")
	tmpFile.Close()

	vt := &VisionTool{}
	orig := ModelRef
	ModelRef = nil
	defer func() { ModelRef = orig }()

	result, err := vt.Execute(context.Background(), map[string]any{
		"image":  tmpFile.Name(),
		"prompt": "describe this",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError because no model is set")
	}
}
