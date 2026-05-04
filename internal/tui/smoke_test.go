package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
)

func TestSmokeAppInit(t *testing.T) {
	th := LoadTheme("tokyo-night")
	app := NewApp(Deps{
		Theme: th,
	})
	cmd := app.Init()
	if cmd == nil {
		t.Error("Init should return a non-nil Cmd")
	}
}

func TestSmokeAppWindowSize(t *testing.T) {
	th := LoadTheme("tokyo-night")
	app := NewApp(Deps{Theme: th})
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a := model.(*App)
	if a.width != 80 || a.height != 24 {
		t.Errorf("expected 80x24, got %dx%d", a.width, a.height)
	}
}

func TestSmokeAppView(t *testing.T) {
	th := LoadTheme("tokyo-night")
	app := NewApp(Deps{Theme: th})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := app.View()
	if view.Content == "" {
		t.Error("View should produce output")
	}
}

func TestSmokeFullInteraction(t *testing.T) {
	th := LoadTheme("tokyo-night")
	logBuf := NewLogBuffer(200)
	hist := NewHistory("", 100)
	compEng := NewCompletionEngine(nil)

	var receivedSlash string
	app := NewApp(Deps{
		Theme:   th,
		LogBuf:  logBuf,
		History: hist,
		CompEng: compEng,
		OnSubmit: func(_ context.Context, input string, _ []string) (string, error) {
			return "echo: " + input, nil
		},
		OnSlash: func(cmd string) {
			receivedSlash = cmd
		},
	})

	// Resize
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Type some text
	for _, r := range "hello" {
		app.Update(tea.KeyPressMsg{Text: string(r)})
	}

	// Check composer has text
	if app.composer.Text() != "hello" {
		t.Errorf("expected 'hello', got %q", app.composer.Text())
	}

	// Submit
	model, _ := app.Update(tea.KeyPressMsg{Text: "enter"})
	a := model.(*App)
	if !a.busy {
		t.Error("expected busy after submit")
	}

	// Check transcript has user message
	msgs := a.transcript.Messages()
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	if msgs[0].Content != "hello" {
		t.Errorf("expected user msg 'hello', got %q", msgs[0].Content)
	}

	// Simulate agent response
	app.Update(agentResponseMsg{text: "echo: hello"})
	if app.busy {
		t.Error("expected not busy after response")
	}

	// Check assistant message in transcript
	msgs = app.transcript.Messages()
	if len(msgs) < 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[1].Kind != MsgAssistant {
		t.Errorf("expected MsgAssistant, got %v", msgs[1].Kind)
	}

	// Test slash command
	for _, r := range "/help" {
		app.Update(tea.KeyPressMsg{Text: string(r)})
	}
	app.Update(tea.KeyPressMsg{Text: "enter"})
	if receivedSlash != "/help" {
		t.Errorf("expected /help slash command, got %q", receivedSlash)
	}

	// Test view renders
	view := app.View()
	if view.Content == "" {
		t.Error("View should produce non-empty output")
	}
	if !strings.Contains(view.Content, "─") {
		t.Error("View should contain divider line")
	}
}

func TestSmokeStreaming(t *testing.T) {
	th := LoadTheme("tokyo-night")
	var sentMsgs []tea.Msg
	app := NewApp(Deps{
		Theme: th,
		Send:  func(msg tea.Msg) { sentMsgs = append(sentMsgs, msg) },
	})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Simulate stream chunks
	app.Update(streamChunkMsg{text: "Hello "})
	app.Update(streamChunkMsg{text: "world"})

	msgs := app.transcript.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", msgs[0].Content)
	}
}

func TestSmokeApproval(t *testing.T) {
	th := LoadTheme("tokyo-night")
	app := NewApp(Deps{Theme: th})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Set approval request
	app.approval = &ApprovalRequest{ToolName: "Bash", Detail: "rm -rf /tmp"}

	// View should show approval popup
	view := app.View()
	if !strings.Contains(view.Content, "Approval") {
		t.Error("View should contain approval text")
	}

	// Press 'y' to approve
	model, _ := app.Update(tea.KeyPressMsg{Text: "y"})
	a := model.(*App)
	if a.approval != nil {
		t.Error("approval should be cleared after allow")
	}
}

func TestSmokeThemes(t *testing.T) {
	for _, name := range []string{"tokyo-night", "catppuccin-mocha", "light", "terminal"} {
		th := LoadTheme(name)
		app := NewApp(Deps{Theme: th})
		app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		view := app.View()
		if view.Content == "" {
			t.Errorf("theme %s: view should not be empty", name)
		}
	}
}

func TestSmokeHistory(t *testing.T) {
	th := LoadTheme("tokyo-night")
	hist := NewHistory("", 100)
	app := NewApp(Deps{Theme: th, History: hist})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Type and submit to add to history
	for _, r := range "first" {
		app.Update(tea.KeyPressMsg{Text: string(r)})
	}
	app.Update(tea.KeyPressMsg{Text: "enter"})
	// busy=true, simulate response
	app.Update(agentResponseMsg{text: "ok"})

	// Type and submit second
	for _, r := range "second" {
		app.Update(tea.KeyPressMsg{Text: string(r)})
	}
	app.Update(tea.KeyPressMsg{Text: "enter"})
	app.Update(agentResponseMsg{text: "ok"})

	// Navigate history
	app.Update(tea.KeyPressMsg{Text: "up"})
	if app.composer.Text() != "second" {
		t.Errorf("expected 'second' from history, got %q", app.composer.Text())
	}
	app.Update(tea.KeyPressMsg{Text: "up"})
	if app.composer.Text() != "first" {
		t.Errorf("expected 'first' from history, got %q", app.composer.Text())
	}
}
