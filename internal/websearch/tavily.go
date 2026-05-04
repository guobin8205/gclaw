package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// TavilyBackend uses the Tavily Search API.
type TavilyBackend struct {
	APIKey string
}

func (b *TavilyBackend) Name() string { return "tavily" }
func (b *TavilyBackend) Check() bool  { return b.APIKey != "" }

type tavilyRequest struct {
	Query          string `json:"query"`
	MaxResults     int    `json:"max_results"`
	SearchDepth    string `json:"search_depth"`
	IncludeContent bool   `json:"include_content"`
}

type tavilyResponse struct {
	Results []struct {
		Title    string `json:"title"`
		URL      string `json:"url"`
		Content  string `json:"content"`
		Snippet  string `json:"snippet"`
	} `json:"results"`
}

func (b *TavilyBackend) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	maxResults := opts.MaxResults
	if maxResults == 0 {
		maxResults = 10
	}

	body := tavilyRequest{
		Query:          query,
		MaxResults:     maxResults,
		SearchDepth:    "basic",
		IncludeContent: true,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("tavily marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("tavily request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily call: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tavily read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily API error (status %d): %s", resp.StatusCode, string(data))
	}

	var result tavilyResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("tavily parse: %w", err)
	}

	results := make([]SearchResult, 0, len(result.Results))
	for _, r := range result.Results {
		results = append(results, SearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Snippet,
			RawContent:  r.Content,
		})
	}
	return results, nil
}

func initTavily() *TavilyBackend {
	return &TavilyBackend{APIKey: os.Getenv("TAVILY_API_KEY")}
}
