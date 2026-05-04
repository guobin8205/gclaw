package vision

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openclaw/gclaw/internal/model"
	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface check.
var _ tool.Tool = (*VisionTool)(nil)

// ModelRef is the model instance used for vision calls, set by main.go.
var ModelRef model.Model

// maxImageSize is the maximum allowed image size (20MB).
const maxImageSize = 20 * 1024 * 1024

// VisionTool analyzes images using a vision-capable model.
type VisionTool struct{}

func (t *VisionTool) Name() string        { return "Vision" }
func (t *VisionTool) Toolset() string     { return "vision" }
func (t *VisionTool) ConcurrencySafe() bool { return true }
func (t *VisionTool) RequiresApproval(map[string]any) bool { return false }

func (t *VisionTool) Description() string {
	return "Analyze an image using a vision-capable model. Provide an image URL or local file path and a prompt describing what to analyze."
}

func (t *VisionTool) Check() bool {
	return ModelRef != nil && ModelRef.SupportsVision()
}

func (t *VisionTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"image": {
				Type:        "string",
				Description: "URL or local file path of the image to analyze",
			},
			"prompt": {
				Type:        "string",
				Description: "The prompt describing what to analyze or extract from the image",
			},
		},
		Required: []string{"image", "prompt"},
	}
}

func (t *VisionTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	if ModelRef == nil {
		return tool.ToolResult{Content: "Error: no vision model available", IsError: true}, nil
	}

	image, _ := params["image"].(string)
	if image == "" {
		return tool.ToolResult{Content: "Error: image parameter is required", IsError: true}, nil
	}

	prompt, _ := params["prompt"].(string)
	if prompt == "" {
		return tool.ToolResult{Content: "Error: prompt parameter is required", IsError: true}, nil
	}

	var data []byte
	var mediaType string
	var err error

	if strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://") {
		data, mediaType, err = downloadImage(ctx, image)
	} else {
		data, mediaType, err = readLocalImage(image)
	}
	if err != nil {
		return tool.ToolResult{Content: fmt.Sprintf("Error: %v", err), IsError: true}, nil
	}

	if len(data) > maxImageSize {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: image size %d bytes exceeds maximum allowed size of %d bytes", len(data), maxImageSize),
			IsError: true,
		}, nil
	}

	base64Data := base64.StdEncoding.EncodeToString(data)

	resp, err := ModelRef.Call(ctx, model.CallParams{
		Messages: []model.Message{
			{
				Role:    "user",
				Content: prompt,
				Images: []model.ImageContent{
					{Data: base64Data, MediaType: mediaType},
				},
			},
		},
		MaxTokens: 4096,
	})
	if err != nil {
		return tool.ToolResult{Content: fmt.Sprintf("Error calling vision model: %v", err), IsError: true}, nil
	}

	return tool.ToolResult{Content: resp.Text}, nil
}

// downloadImage fetches an image from a URL and returns its data and media type.
func downloadImage(ctx context.Context, url string) ([]byte, string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("downloading image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("reading image data: %w", err)
	}

	// Determine media type from URL extension or Content-Type header.
	mediaType := mediaTypeFromExt(url)
	if mediaType == "image/png" {
		// Fallback: try Content-Type header if extension didn't help.
		if ct := resp.Header.Get("Content-Type"); ct != "" && strings.HasPrefix(ct, "image/") {
			mediaType = ct
		}
	}

	return data, mediaType, nil
}

// readLocalImage reads an image from a local file path and returns its data and media type.
func readLocalImage(path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("reading image file: %w", err)
	}

	mediaType := mediaTypeFromExt(path)
	return data, mediaType, nil
}

// mediaTypeFromExt returns the MIME media type based on the file extension.
func mediaTypeFromExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func init() {
	tool.GlobalRegistry.Register(&VisionTool{})
}
