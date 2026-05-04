package tts

import (
	"context"
	"fmt"

	"github.com/openclaw/gclaw/internal/tool"
	ttsbackend "github.com/openclaw/gclaw/internal/tts"
)

// Compile-time interface check.
var _ tool.Tool = (*TTSTool)(nil)

// TTSFactory is the package-level factory set by the application.
// When nil, the tool is unavailable (Check returns false).
var TTSFactory *ttsbackend.Factory

// TTSTool provides text-to-speech synthesis.
type TTSTool struct{}

func (t *TTSTool) Name() string        { return "TTS" }
func (t *TTSTool) Toolset() string     { return "tts" }
func (t *TTSTool) Description() string { return "Convert text to speech audio using a TTS backend (e.g. OpenAI). Returns the path to the generated audio file." }
func (t *TTSTool) Check() bool         { return TTSFactory != nil }
func (t *TTSTool) ConcurrencySafe() bool     { return true }
func (t *TTSTool) RequiresApproval(map[string]any) bool { return false }

func (t *TTSTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"text": {
				Type:        "string",
				Description: "The text to convert to speech",
			},
			"voice": {
				Type:        "string",
				Description: "Voice to use (e.g. alloy, echo, fable, onyx, nova, shimmer). Defaults to alloy.",
			},
			"backend": {
				Type:        "string",
				Description: "TTS backend to use. Defaults to the configured default backend.",
			},
		},
		Required: []string{"text"},
	}
}

func (t *TTSTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	text, _ := params["text"].(string)
	if text == "" {
		return tool.ToolResult{
			Content: "Error: text is required.",
			IsError: true,
		}, nil
	}

	voice, _ := params["voice"].(string)
	backend, _ := params["backend"].(string)

	opts := ttsbackend.TTSOptions{
		Voice:  voice,
		Format: "mp3",
	}

	result, err := TTSFactory.Speak(ctx, text, opts, backend)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error: %v", err),
			IsError: true,
		}, nil
	}

	return tool.ToolResult{
		Content: fmt.Sprintf("Audio generated: %s", result.FilePath),
	}, nil
}

func init() {
	tool.GlobalRegistry.Register(&TTSTool{})
}
