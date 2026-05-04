package imagegen

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

// OpenAIBackend uses the OpenAI DALL-E API for image generation.
type OpenAIBackend struct {
	APIKey string
}

func (b *OpenAIBackend) Name() string { return "openai" }
func (b *OpenAIBackend) Check() bool  { return b.APIKey != "" }

type openaiImageRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Size   string `json:"size"`
	N      int    `json:"n"`
}

type openaiImageResponse struct {
	Data []struct {
		URL string `json:"url"`
	} `json:"data"`
}

// openaiSizeMap maps GenOptions Size values to OpenAI DALL-E size strings.
var openaiSizeMap = map[string]string{
	"landscape": "1792x1024",
	"portrait":  "1024x1792",
	"square":    "1024x1024",
}

func (b *OpenAIBackend) Generate(ctx context.Context, prompt string, opts GenOptions) (*GenResult, error) {
	size := openaiSizeMap[opts.Size]
	if size == "" {
		size = "1024x1024"
	}

	model := "dall-e-3"
	if opts.Model != "" {
		model = opts.Model
	}

	body := openaiImageRequest{
		Model:  model,
		Prompt: prompt,
		Size:   size,
		N:      1,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/images/generations", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.APIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai call: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai API error (status %d): %s", resp.StatusCode, string(data))
	}

	var result openaiImageResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("openai parse: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("openai returned no images")
	}

	return &GenResult{
		URL: result.Data[0].URL,
	}, nil
}

func initOpenAI() *OpenAIBackend {
	return &OpenAIBackend{APIKey: os.Getenv("OPENAI_API_KEY")}
}
