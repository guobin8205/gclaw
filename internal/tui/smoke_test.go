package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
)

func TestSmokeAppInit(t *testing.T) {
	app := NewApp(Deps{Theme: LoadTheme("tokyo-night")})
	if app.Init() == nil {
		t.Error("Init should return a non-nil Cmd")
	}
}

func TestSmokeAppWindowSize(t *testing.T) {
	app := NewApp(Deps{Theme: LoadTheme("tokyo-night")})
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a := model.(*App)
	if a.width != 80 || a.height != 24 {
		t.Errorf("expected 80x24, got %dx%d", a.width, a.height)
	}
}

func TestSmokeAppView(t *testing.T) {
	app := NewApp(Deps{Theme: LoadTheme("tokyo-night")})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if app.View().Content == "" {
		t.Error("View should produce output")
	}
}

func TestSmokeFullInteraction(t *testing.T) {
	th := LoadTheme("tokyo-night")
	var receivedSlash string
	app := NewApp(Deps{
		Theme:   th,
		LogBuf:  NewLogBuffer(200),
		History: NewHistory("", 100),
		CompEng: NewCompletionEngine(nil),
		OnSubmit: func(_ context.Context, input string, _ []string) (string, error) {
			return "echo: " + input, nil
		},
		OnSlash: func(cmd string) { receivedSlash = cmd },
	})

	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Type text
	for _, r := range "hello" {
		app.Update(tea.KeyPressMsg{Text: string(r)})
	}
	if app.composer.Text() != "hello" {
		t.Errorf("expected 'hello', got %q", app.composer.Text())
	}

	// Submit
	model, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	a := model.(*App)
	if !a.busy {
		t.Error("expected busy after submit")
	}

	msgs := a.transcript.Messages()
	if len(msgs) != 1 || msgs[0].Content != "hello" {
		t.Fatalf("transcript: got %+v", msgs)
	}

	// Respond
	app.Update(agentResponseMsg{text: "echo: hello"})
	if app.busy {
		t.Error("should not be busy after response")
	}

	// Slash command
	for _, r := range "/help" {
		app.Update(tea.KeyPressMsg{Text: string(r)})
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if receivedSlash != "/help" {
		t.Errorf("expected /help, got %q", receivedSlash)
	}

	// View renders
	v := app.View()
	if v.Content == "" || !strings.Contains(v.Content, "─") {
		t.Error("view should have content and divider")
	}
}

func TestSmokeStreaming(t *testing.T) {
	app := NewApp(Deps{Theme: LoadTheme("tokyo-night")})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	app.Update(streamChunkMsg{text: "Hello "})
	app.Update(streamChunkMsg{text: "world"})

	msgs := app.transcript.Messages()
	if len(msgs) != 1 || msgs[0].Content != "Hello world" {
		t.Errorf("streaming: got %+v", msgs)
	}
}

func TestSmokeApproval(t *testing.T) {
	app := NewApp(Deps{Theme: LoadTheme("tokyo-night")})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	app.approval = &ApprovalRequest{ToolName: "Bash", Detail: "rm -rf /tmp"}
	if !strings.Contains(app.View().Content, "Approval") {
		t.Error("should show approval")
	}

	// Press 'y' (printable → goes through approval handler at top of handleKey)
	app.Update(tea.KeyPressMsg{Text: "y"})
	// Approval uses msg.String() which for Text="y" returns "y"
	if app.approval != nil {
		t.Error("approval should be cleared")
	}
}

func TestSmokeThemes(t *testing.T) {
	for _, name := range []string{"tokyo-night", "catppuccin-mocha", "light", "terminal"} {
		app := NewApp(Deps{Theme: LoadTheme(name)})
		app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		if app.View().Content == "" {
			t.Errorf("theme %s: empty", name)
		}
	}
}

func TestSmokeHistory(t *testing.T) {
	hist := NewHistory("", 100)
	app := NewApp(Deps{Theme: LoadTheme("tokyo-night"), History: hist})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Submit messages
	for _, r := range "first" {
		app.Update(tea.KeyPressMsg{Text: string(r)})
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app.Update(agentResponseMsg{text: "ok"})

	for _, r := range "second" {
		app.Update(tea.KeyPressMsg{Text: string(r)})
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app.Update(agentResponseMsg{text: "ok"})

	app.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if app.composer.Text() != "second" {
		t.Errorf("up: got %q", app.composer.Text())
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if app.composer.Text() != "first" {
		t.Errorf("up 2x: got %q", app.composer.Text())
	}
}
