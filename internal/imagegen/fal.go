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

// FalBackend uses the FAL.ai API (sync mode) for image generation.
type FalBackend struct {
	APIKey string
}

func (b *FalBackend) Name() string { return "fal" }
func (b *FalBackend) Check() bool  { return b.APIKey != "" }

type falRequest struct {
	Prompt    string `json:"prompt"`
	ImageSize string `json:"image_size,omitempty"`
}

type falResponse struct {
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}

// falSizeMap maps GenOptions Size values to FAL.ai image_size values.
var falSizeMap = map[string]string{
	"landscape": "landscape_16_9",
	"square":    "square",
	"portrait":  "portrait_16_9",
}

func (b *FalBackend) Generate(ctx context.Context, prompt string, opts GenOptions) (*GenResult, error) {
	imageSize := falSizeMap[opts.Size]

	body := falRequest{
		Prompt:    prompt,
		ImageSize: imageSize,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("fal marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://fal.run/fal-ai/flux/schnell", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("fal request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Key "+b.APIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fal call: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fal read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fal API error (status %d): %s", resp.StatusCode, string(data))
	}

	var result falResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("fal parse: %w", err)
	}

	if len(result.Images) == 0 {
		return nil, fmt.Errorf("fal returned no images")
	}

	return &GenResult{
		URL: result.Images[0].URL,
	}, nil
}

func initFal() *FalBackend {
	return &FalBackend{APIKey: os.Getenv("FAL_API_KEY")}
}
