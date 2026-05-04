package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const openAIEndpoint = "https://api.openai.com/v1/audio/speech"

// OpenAIBackend implements TTS via the OpenAI API.
type OpenAIBackend struct {
	APIKey string
	Model  string // defaults to "tts-1"
}

// Compile-time interface check.
var _ Backend = (*OpenAIBackend)(nil)

// Name returns the backend name.
func (b *OpenAIBackend) Name() string { return "openai" }

// Check returns true if the API key is configured.
func (b *OpenAIBackend) Check() bool {
	return b.APIKey != ""
}

// openAIRequest represents the request body for the OpenAI TTS API.
type openAIRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
	Voice string `json:"voice"`
	Speed float64 `json:"speed,omitempty"`
}

// Speak calls the OpenAI TTS API and saves the result to a temp file.
func (b *OpenAIBackend) Speak(ctx context.Context, text string, opts TTSOptions) (*TTSResult, error) {
	apiKey := b.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("tts openai: OPENAI_API_KEY not set")
	}

	model := b.Model
	if model == "" {
		model = "tts-1"
	}

	voice := opts.Voice
	if voice == "" {
		voice = "alloy"
	}

	format := opts.Format
	if format == "" {
		format = "mp3"
	}

	reqBody := openAIRequest{
		Model: model,
		Input: text,
		Voice: voice,
	}
	if opts.Speed > 0 {
		reqBody.Speed = opts.Speed
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("tts openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("tts openai: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tts openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tts openai: API returned %d: %s", resp.StatusCode, string(body))
	}

	// Create temp file
	ext := "." + format
	tmpFile, err := os.CreateTemp(os.TempDir(), "tts_*"+ext)
	if err != nil {
		return nil, fmt.Errorf("tts openai: create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("tts openai: write audio data: %w", err)
	}

	return &TTSResult{
		FilePath: filepath.ToSlash(tmpFile.Name()),
	}, nil
}
