package tui

import "testing"

func TestRenderCodeBlock(t *testing.T) {
	input := "```go\nfmt.Println(\"hello\")\n```"
	lines := RenderMarkdown(input, Themes["tokyo-night"])
	if len(lines) == 0 {
		t.Fatal("expected non-empty output")
	}
}

func TestRenderBold(t *testing.T) {
	lines := RenderMarkdown("this is **bold** text", Themes["tokyo-night"])
	if len(lines) == 0 {
		t.Fatal("expected non-empty")
	}
}

func TestRenderPlainText(t *testing.T) {
	lines := RenderMarkdown("hello world", Themes["tokyo-night"])
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
}
