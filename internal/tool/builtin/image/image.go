package image

import (
	"context"
	"fmt"

	"github.com/openclaw/gclaw/internal/imagegen"
	"github.com/openclaw/gclaw/internal/tool"
)

// GenFactory is the image generation factory, set by main.go.
var GenFactory *imagegen.Factory

// ImageGenTool generates images via configured backends.
type ImageGenTool struct{}

func (t *ImageGenTool) Name() string        { return "ImageGen" }
func (t *ImageGenTool) Toolset() string      { return "image" }
func (t *ImageGenTool) ConcurrencySafe() bool { return true }
func (t *ImageGenTool) RequiresApproval(params map[string]any) bool { return false }

func (t *ImageGenTool) Description() string {
	return "Generate images from text prompts using configured backends (FAL.ai, OpenAI DALL-E). Returns the generated image URL."
}

func (t *ImageGenTool) Check() bool {
	return GenFactory != nil
}

func (t *ImageGenTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"prompt": {Type: "string", Description: "The text prompt describing the image to generate"},
			"model":  {Type: "string", Description: "Model to use for generation (e.g. dall-e-3). Uses default if omitted."},
			"size":   {Type: "string", Description: "Image size: landscape, square, or portrait", Enum: []string{"landscape", "square", "portrait"}},
		},
		Required: []string{"prompt"},
	}
}

func (t *ImageGenTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	prompt, ok := params["prompt"].(string)
	if !ok || prompt == "" {
		return tool.ToolResult{Content: "Error: prompt is required", IsError: true}, nil
	}

	opts := imagegen.GenOptions{}
	if v, ok := params["model"].(string); ok {
		opts.Model = v
	}
	if v, ok := params["size"].(string); ok {
		opts.Size = v
	}

	result, err := GenFactory.Generate(ctx, prompt, opts, "")
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error generating image: %v", err),
			IsError: true,
		}, nil
	}

	var b fmt.Stringer = &resultFormatter{
		result: result,
		prompt: prompt,
		size:   opts.Size,
	}

	return tool.ToolResult{Content: b.String()}, nil
}

type resultFormatter struct {
	result *imagegen.GenResult
	prompt string
	size   string
}

func (f *resultFormatter) String() string {
	size := f.size
	if size == "" {
		size = "default"
	}
	return fmt.Sprintf("Image generated:\nURL: %s\nPrompt: %s\nSize: %s", f.result.URL, f.prompt, size)
}

func init() {
	tool.GlobalRegistry.Register(&ImageGenTool{})
}
