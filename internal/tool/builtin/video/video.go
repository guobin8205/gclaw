package video

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openclaw/gclaw/internal/model"
	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface check.
var _ tool.Tool = (*VideoTool)(nil)

// ModelRef is the model instance used for video analysis calls, set by main.go.
var ModelRef model.Model

const (
	// maxVideoSize is the hard cap for video file size (50MB).
	maxVideoSize = 50 * 1024 * 1024

	// warnVideoSize is the threshold above which a warning is logged (20MB).
	warnVideoSize = 20 * 1024 * 1024

	// callTimeout is the timeout for the model call (videos take longer than images).
	callTimeout = 180 * time.Second

	// downloadTimeout is the timeout for downloading a video from a URL.
	downloadTimeout = 30 * time.Second
)

// allowedExtensions maps lowercase file extensions to MIME media types.
var allowedExtensions = map[string]string{
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mov":  "video/quicktime",
	".avi":  "video/x-msvideo",
	".mkv":  "video/x-matroska",
	".mpeg": "video/mpeg",
}

// VideoTool analyzes videos using a vision-capable model.
type VideoTool struct{}

func (t *VideoTool) Name() string        { return "video_analyze" }
func (t *VideoTool) Toolset() string     { return "video" }
func (t *VideoTool) ConcurrencySafe() bool { return true }
func (t *VideoTool) RequiresApproval(map[string]any) bool { return false }

func (t *VideoTool) Description() string {
	return "Analyze a video using a vision-capable model. Provide a video URL or local file path and a prompt describing what to analyze or extract from the video."
}

func (t *VideoTool) Check() bool {
	return ModelRef != nil && ModelRef.SupportsVision()
}

func (t *VideoTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"video": {
				Type:        "string",
				Description: "URL or local file path of the video to analyze",
			},
			"prompt": {
				Type:        "string",
				Description: "The prompt describing what to analyze or extract from the video",
			},
		},
		Required: []string{"video", "prompt"},
	}
}

func (t *VideoTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	if ModelRef == nil {
		return tool.ToolResult{Content: "Error: no vision model available", IsError: true}, nil
	}

	video, _ := params["video"].(string)
	if video == "" {
		return tool.ToolResult{Content: "Error: video parameter is required", IsError: true}, nil
	}

	prompt, _ := params["prompt"].(string)
	if prompt == "" {
		return tool.ToolResult{Content: "Error: prompt parameter is required", IsError: true}, nil
	}

	var data []byte
	var mediaType string
	var err error

	if strings.HasPrefix(video, "http://") || strings.HasPrefix(video, "https://") {
		data, mediaType, err = downloadVideo(video)
	} else {
		data, mediaType, err = readLocalVideo(video)
	}
	if err != nil {
		return tool.ToolResult{Content: fmt.Sprintf("Error: %v", err), IsError: true}, nil
	}

	// Size validation.
	if len(data) > maxVideoSize {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: video size %d bytes exceeds maximum allowed size of %d bytes", len(data), maxVideoSize),
			IsError: true,
		}, nil
	}
	if len(data) > warnVideoSize {
		log.Printf("[video] warning: video size %d bytes exceeds recommended limit of %d bytes, proceeding anyway", len(data), warnVideoSize)
	}

	base64Data := base64.StdEncoding.EncodeToString(data)

	// Use a 180s timeout for the model call — videos take longer than images.
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	resp, err := ModelRef.Call(callCtx, model.CallParams{
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

// downloadVideo fetches a video from a URL with SSRF protection and returns its data and media type.
func downloadVideo(url string) ([]byte, string, error) {
	// SSRF protection: resolve and check the host before making the request.
	host := extractHost(url)
	if isPrivateIP(host) {
		return nil, "", fmt.Errorf("blocked private/internal IP address for host %q", host)
	}

	client := &http.Client{Timeout: downloadTimeout}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("downloading video: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxVideoSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("reading video data: %w", err)
	}

	mediaType := mediaTypeFromURL(url)
	return data, mediaType, nil
}

// readLocalVideo reads a video from a local file path and returns its data and media type.
func readLocalVideo(path string) ([]byte, string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	mediaType, ok := allowedExtensions[ext]
	if !ok {
		allowedList := make([]string, 0, len(allowedExtensions))
		for k := range allowedExtensions {
			allowedList = append(allowedList, k)
		}
		return nil, "", fmt.Errorf("unsupported video format %q, allowed: %s", ext, strings.Join(allowedList, ", "))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("reading video file: %w", err)
	}

	return data, mediaType, nil
}

// mediaTypeFromURL determines the MIME media type from a URL's file extension.
func mediaTypeFromURL(rawURL string) string {
	// Strip query and fragment.
	s := rawURL
	if i := strings.Index(s, "?"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "#"); i >= 0 {
		s = s[:i]
	}
	ext := strings.ToLower(filepath.Ext(s))
	if mt, ok := allowedExtensions[ext]; ok {
		return mt
	}
	// Default to video/mp4 for URLs without a recognized extension.
	return "video/mp4"
}

// extractHost extracts the hostname from a URL string.
func extractHost(rawURL string) string {
	s := rawURL
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "?"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "#"); i >= 0 {
		s = s[:i]
	}
	return s
}

// isPrivateIP checks if a host resolves to a private, loopback, link-local, or unspecified IP.
func isPrivateIP(host string) bool {
	// Strip port.
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	if host == "localhost" {
		return true
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return true // if we can't resolve, be safe
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}

func init() {
	tool.GlobalRegistry.Register(&VideoTool{})
}
