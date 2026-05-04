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

// ExaBackend uses the Exa (Metaphor) Search API.
type ExaBackend struct {
	APIKey string
}

func (b *ExaBackend) Name() string { return "exa" }
func (b *ExaBackend) Check() bool  { return b.APIKey != "" }

type exaRequest struct {
	Query     string `json:"query"`
	NumResults int   `json:"numResults"`
	Contents   *exaContents `json:"contents,omitempty"`
}

type exaContents struct {
	Text bool `json:"text"`
}

type exaResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Text    string `json:"text"`
		Snippet string `json:"snippet,omitempty"`
	} `json:"results"`
}

func (b *ExaBackend) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	numResults := opts.MaxResults
	if numResults == 0 {
		numResults = 10
	}

	body := exaRequest{
		Query:      query,
		NumResults: numResults,
		Contents:   &exaContents{Text: true},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("exa marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.exa.ai/search", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("exa request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", b.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exa call: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("exa read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exa API error (status %d): %s", resp.StatusCode, string(data))
	}

	var result exaResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("exa parse: %w", err)
	}

	results := make([]SearchResult, 0, len(result.Results))
	for _, r := range result.Results {
		results = append(results, SearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Snippet,
			RawContent:  r.Text,
		})
	}
	return results, nil
}

func initExa() *ExaBackend {
	return &ExaBackend{APIKey: os.Getenv("EXA_API_KEY")}
}
