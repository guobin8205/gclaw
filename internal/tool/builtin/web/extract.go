package web

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	mdconv "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/openclaw/gclaw/internal/tool"
)

// ModelFn is an optional function for LLM summarization, set by main.go.
var ModelFn func(ctx context.Context, prompt string) (string, error)

const (
	maxContentSize  = 2_000_000 // 2M chars
	summarizeThresh = 500_000   // 500K chars
	summarizeTarget = 5_000     // ~5K chars target summary
	cacheTTL        = 30 * time.Minute
	userAgent       = "Mozilla/5.0 (compatible; GClawBot/1.0)"
)

// pageCache is a simple TTL cache for fetched pages.
var pageCache sync.Map

type cacheEntry struct {
	content  string
	fetched  time.Time
}

// WebExtractTool fetches a web page and converts it to markdown or plain text.
type WebExtractTool struct{}

func (t *WebExtractTool) Name() string        { return "WebExtract" }
func (t *WebExtractTool) Toolset() string      { return "web" }
func (t *WebExtractTool) ConcurrencySafe() bool { return true }
func (t *WebExtractTool) RequiresApproval(params map[string]any) bool { return false }

func (t *WebExtractTool) Description() string {
	return "Fetch a web page and extract its content as markdown or plain text. Supports optional LLM summarization for long pages."
}

func (t *WebExtractTool) Check() bool { return true }

func (t *WebExtractTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"url":      {Type: "string", Description: "The URL to fetch (must start with http:// or https://)"},
			"format":   {Type: "string", Description: "Output format: 'markdown' (default) or 'text'", Enum: []string{"markdown", "text"}},
			"summarize": {Type: "boolean", Description: "Whether to summarize long content using LLM (default: true when content exceeds threshold)"},
		},
		Required: []string{"url"},
	}
}

func (t *WebExtractTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	url, ok := params["url"].(string)
	if !ok || url == "" {
		return tool.ToolResult{Content: "Error: url is required", IsError: true}, nil
	}

	// Validate URL scheme
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return tool.ToolResult{
			Content: "Error: url must start with http:// or https://",
			IsError: true,
		}, nil
	}

	// SSRF protection: check host
	host := extractHost(url)
	if isPrivateIP(host) {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: blocked private/internal IP address for host %q", host),
			IsError: true,
		}, nil
	}

	// Check cache
	if cached, ok := pageCache.Load(url); ok {
		entry := cached.(*cacheEntry)
		if time.Since(entry.fetched) < cacheTTL {
			return tool.ToolResult{Content: entry.content}, nil
		}
		pageCache.Delete(url)
	}

	// HTTP GET
	content, err := fetchURL(ctx, url)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error fetching URL: %v", err),
			IsError: true,
		}, nil
	}

	// Convert HTML to markdown
	markdown, err := mdconv.ConvertString(content)
	if err != nil {
		// If conversion fails, return raw content stripped of tags
		markdown = stripHTMLTags(content)
	}

	// Determine format
	format := "markdown"
	if v, ok := params["format"].(string); ok && v != "" {
		format = v
	}

	result := markdown
	if format == "text" {
		result = stripMarkdown(markdown)
	}

	// Summarize if needed
	shouldSummarize := true
	if v, ok := params["summarize"]; ok {
		switch b := v.(type) {
		case bool:
			shouldSummarize = b
		}
	}

	if len(result) > summarizeThresh && ModelFn != nil && shouldSummarize {
		summary, err := summarize(ctx, result)
		if err == nil && len(summary) > 0 {
			result = summary
		}
	}

	// Cache the result
	pageCache.Store(url, &cacheEntry{content: result, fetched: time.Now()})

	// Evict old entries (simple: delete entries older than cacheTTL)
	pageCache.Range(func(key, value any) bool {
		entry := value.(*cacheEntry)
		if time.Since(entry.fetched) > cacheTTL {
			pageCache.Delete(key)
		}
		return true
	})

	return tool.ToolResult{Content: result}, nil
}

// fetchURL performs an HTTP GET with timeout and content checks.
func fetchURL(ctx context.Context, url string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Check Content-Type — skip binary content
	ct := resp.Header.Get("Content-Type")
	if isBinaryContentType(ct) {
		return "", fmt.Errorf("unsupported content type: %s", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading body: %w", err)
	}

	if len(body) > maxContentSize {
		return "", fmt.Errorf("content too large: %d bytes (max %d)", len(body), maxContentSize)
	}

	return string(body), nil
}

// isBinaryContentType returns true for non-text content types.
func isBinaryContentType(ct string) bool {
	ct = strings.ToLower(ct)
	for _, skip := range []string{
		"image/", "video/", "audio/",
		"application/pdf", "application/zip",
		"application/octet-stream",
		"application/x-", "multipart/",
	} {
		if strings.Contains(ct, skip) {
			return true
		}
	}
	return false
}

// extractHost extracts the hostname from a URL string.
func extractHost(rawURL string) string {
	// Strip scheme
	s := rawURL
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	// Strip path
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	// Strip query
	if i := strings.Index(s, "?"); i >= 0 {
		s = s[:i]
	}
	// Strip fragment
	if i := strings.Index(s, "#"); i >= 0 {
		s = s[:i]
	}
	return s
}

// isPrivateIP checks if a host resolves to a private/internal IP.
func isPrivateIP(host string) bool {
	// Strip port
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// Check for localhost literal
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

// summarize calls the LLM to summarize long content.
func summarize(ctx context.Context, content string) (string, error) {
	prompt := fmt.Sprintf(
		"Summarize the following web page content in approximately %d characters. "+
			"Preserve key facts, data points, and important details. "+
			"Use clear, concise language.\n\n%s",
		summarizeTarget, content,
	)
	return ModelFn(ctx, prompt)
}

// stripHTMLTags removes basic HTML tags from a string.
func stripHTMLTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// stripMarkdown removes markdown formatting for plain text output.
func stripMarkdown(s string) string {
	// Simple markdown stripping: remove common markers
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "##", "")
	s = strings.ReplaceAll(s, "###", "")
	s = strings.ReplaceAll(s, "#", "")
	s = strings.ReplaceAll(s, "```", "")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "~", "")
	// Collapse multiple blank lines
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

func init() {
	tool.GlobalRegistry.Register(&WebExtractTool{})
}
