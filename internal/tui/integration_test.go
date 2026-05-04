package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
)

// simulateSession runs a full TUI session simulation with key events.
type simSession struct {
	app    *App
	t      *testing.T
	events []string // captured slash commands
}

func newSimSession(t *testing.T) *simSession {
	t.Helper()
	th := LoadTheme("tokyo-night")
	s := &simSession{t: t}

	s.app = NewApp(Deps{
		Theme:   th,
		LogBuf:  NewLogBuffer(200),
		History: NewHistory("", 100),
		CompEng: NewCompletionEngine(defaultCommands),
		OnSubmit: func(_ context.Context, input string, _ []string) (string, error) {
			return "response to: " + input, nil
		},
		OnSlash: func(cmd string) {
			s.events = append(s.events, cmd)
		},
	})

	s.app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return s
}

func (s *simSession) typeText(text string) {
	for _, r := range text {
		s.app.Update(tea.KeyPressMsg{Text: string(r)})
	}
}

func (s *simSession) press(key string) {
	s.app.Update(tea.KeyPressMsg{Text: key})
}

func (s *simSession) submit() {
	s.app.Update(tea.KeyPressMsg{Text: "enter"})
}

func (s *simSession) respond(text string) {
	s.app.Update(agentResponseMsg{text: text})
}

func (s *simSession) streamChunk(text string) {
	s.app.Update(streamChunkMsg{text: text})
}

func (s *simSession) viewContent() string {
	return s.app.View().Content
}

// ============================================================
// Test 1: Basic typing and editing
// ============================================================

func TestSimBasicTyping(t *testing.T) {
	s := newSimSession(t)

	s.typeText("hello")
	if s.app.composer.Text() != "hello" {
		t.Errorf("after typing 'hello': got %q", s.app.composer.Text())
	}

	s.press("backspace")
	if s.app.composer.Text() != "hell" {
		t.Errorf("after backspace: got %q", s.app.composer.Text())
	}

	s.press("left")
	s.press("delete")
	if s.app.composer.Text() != "hel" {
		t.Errorf("after left+delete: got %q", s.app.composer.Text())
	}

	s.press("esc")
	if !s.app.composer.IsEmpty() {
		t.Errorf("after esc: should be empty, got %q", s.app.composer.Text())
	}

	v := s.viewContent()
	if !strings.Contains(v, "❯") {
		t.Error("empty composer should show prompt")
	}
}

// ============================================================
// Test 2: Multi-line editing
// ============================================================

func TestSimMultiLine(t *testing.T) {
	s := newSimSession(t)

	s.typeText("line1")
	s.press("ctrl+enter")
	s.typeText("line2")

	if s.app.composer.Text() != "line1\nline2" {
		t.Errorf("multi-line: got %q", s.app.composer.Text())
	}
	if s.app.composer.LineCount() != 2 {
		t.Errorf("line count: got %d, want 2", s.app.composer.LineCount())
	}

	// Delete all of line2 content: "line2" has 5 chars
	for i := 0; i < 5; i++ {
		s.press("backspace")
	}
	// Now at "line1\n", cursor at col 0 of empty line2
	if s.app.composer.Text() != "line1\n" {
		t.Errorf("after deleting line2 content: got %q", s.app.composer.Text())
	}

	// One more backspace merges lines
	s.press("backspace")
	if s.app.composer.Text() != "line1" {
		t.Errorf("after merge: got %q", s.app.composer.Text())
	}
	if s.app.composer.LineCount() != 1 {
		t.Errorf("after merge: got %d lines, want 1", s.app.composer.LineCount())
	}
}

// ============================================================
// Test 3: Cursor movement
// ============================================================

func TestSimCursorMovement(t *testing.T) {
	s := newSimSession(t)

	s.typeText("abc")
	s.press("home")
	s.press("delete")
	if s.app.composer.Text() != "bc" {
		t.Errorf("after home+delete: got %q", s.app.composer.Text())
	}

	s.press("end")
	s.typeText("d")
	if s.app.composer.Text() != "bcd" {
		t.Errorf("after end+type: got %q", s.app.composer.Text())
	}

	s.press("left")
	s.press("left")
	s.typeText("X")
	if s.app.composer.Text() != "bXcd" {
		t.Errorf("after left+insert: got %q", s.app.composer.Text())
	}
}

// ============================================================
// Test 4: Submit, response, transcript
// ============================================================

func TestSimSubmitAndResponse(t *testing.T) {
	s := newSimSession(t)

	s.typeText("what is Go?")
	s.submit()

	if !s.app.busy {
		t.Error("should be busy after submit")
	}
	if !s.app.composer.IsEmpty() {
		t.Errorf("composer should be cleared, got %q", s.app.composer.Text())
	}

	msgs := s.app.transcript.Messages()
	if len(msgs) != 1 || msgs[0].Kind != MsgUser || msgs[0].Content != "what is Go?" {
		t.Fatalf("transcript should have user msg, got %+v", msgs)
	}

	s.respond("Go is a compiled language.")
	if s.app.busy {
		t.Error("should not be busy after response")
	}

	msgs = s.app.transcript.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[1].Kind != MsgAssistant {
		t.Errorf("second msg should be assistant, got %v", msgs[1].Kind)
	}

	v := s.viewContent()
	if !strings.Contains(v, "Go is a compiled language") {
		t.Error("view should contain assistant response")
	}
}

// ============================================================
// Test 5: Streaming output
// ============================================================

func TestSimStreaming(t *testing.T) {
	s := newSimSession(t)

	s.streamChunk("Hello ")
	s.streamChunk("from ")
	s.streamChunk("streaming!")

	msgs := s.app.transcript.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 streaming message, got %d", len(msgs))
	}
	if msgs[0].Content != "Hello from streaming!" {
		t.Errorf("streaming content: got %q", msgs[0].Content)
	}

	// Continuous streaming appends
	s.streamChunk("New turn")
	msgs = s.app.transcript.Messages()
	if len(msgs) != 1 {
		t.Fatalf("continuous streaming should append, got %d", len(msgs))
	}

	// After response, new stream creates new message
	s.respond("")
	s.streamChunk("Second ")
	s.streamChunk("response")

	msgs = s.app.transcript.Messages()
	if len(msgs) != 2 {
		t.Fatalf("after response+stream: got %d", len(msgs))
	}
	if msgs[1].Content != "Second response" {
		t.Errorf("second streaming: got %q", msgs[1].Content)
	}
}

// ============================================================
// Test 6: Slash commands
// ============================================================

func TestSimSlashCommands(t *testing.T) {
	s := newSimSession(t)

	s.press("enter") // empty → no action
	if len(s.events) != 0 {
		t.Error("empty input should not trigger slash command")
	}

	s.typeText("/help")
	s.submit()
	if len(s.events) != 1 || s.events[0] != "/help" {
		t.Errorf("expected /help slash event, got %v", s.events)
	}

	if !s.app.composer.IsEmpty() {
		t.Errorf("composer should be cleared after slash, got %q", s.app.composer.Text())
	}

	s.typeText("normal text")
	s.submit()
	if len(s.events) != 1 {
		t.Errorf("normal text should not trigger OnSlash, got %d events", len(s.events))
	}
}

// ============================================================
// Test 7: History navigation
// ============================================================

func TestSimHistory(t *testing.T) {
	s := newSimSession(t)

	s.typeText("first")
	s.submit()
	s.respond("r1")

	s.typeText("second")
	s.submit()
	s.respond("r2")

	s.typeText("third")
	s.submit()
	s.respond("r3")

	s.press("up")
	if s.app.composer.Text() != "third" {
		t.Errorf("up once: got %q", s.app.composer.Text())
	}

	s.press("up")
	if s.app.composer.Text() != "second" {
		t.Errorf("up twice: got %q", s.app.composer.Text())
	}

	s.press("up")
	if s.app.composer.Text() != "first" {
		t.Errorf("up three times: got %q", s.app.composer.Text())
	}

	s.press("down")
	if s.app.composer.Text() != "second" {
		t.Errorf("down from first: got %q", s.app.composer.Text())
	}

	s.press("down")
	if s.app.composer.Text() != "third" {
		t.Errorf("down from second: got %q", s.app.composer.Text())
	}

	s.press("down")
	if s.app.composer.Text() != "" {
		t.Errorf("down past end: got %q, want empty", s.app.composer.Text())
	}
}

// ============================================================
// Test 8: Approval flow
// ============================================================

func TestSimApprovalFlow(t *testing.T) {
	s := newSimSession(t)

	// Set approval
	s.app.approval = &ApprovalRequest{ToolName: "Bash", Detail: "rm -rf /tmp"}
	v := s.viewContent()
	if !strings.Contains(v, "Approval") || !strings.Contains(v, "rm -rf /tmp") {
		t.Error("view should show approval with detail")
	}

	// Unknown keys during approval don't go to composer
	// Use keys that won't match approval actions (not y/n/a/esc)
	s.press("x")
	if s.app.composer.Text() != "" {
		t.Error("unknown key during approval should not go to composer")
	}
	if s.app.approval == nil {
		t.Error("approval should stay pending on unknown key")
	}

	// 'n' denies and clears
	s.press("n")
	if s.app.approval != nil {
		t.Error("approval should be cleared after deny")
	}

	// 'y' allows
	s.app.approval = &ApprovalRequest{ToolName: "WriteFile", Detail: "/tmp/test.go"}
	s.press("y")
	if s.app.approval != nil {
		t.Error("approval should be cleared after allow")
	}

	// 'a' always allow
	s.app.approval = &ApprovalRequest{ToolName: "ReadFile", Detail: "main.go"}
	s.press("a")
	if s.app.approval != nil {
		t.Error("approval should be cleared after always")
	}

	// 'esc' cancels
	s.app.approval = &ApprovalRequest{ToolName: "Grep", Detail: "pattern"}
	s.press("esc")
	if s.app.approval != nil {
		t.Error("approval should be cleared after cancel")
	}
}

// ============================================================
// Test 9: Busy blocks submit
// ============================================================

func TestSimBusyBlocksSubmit(t *testing.T) {
	s := newSimSession(t)

	s.typeText("first")
	s.submit()
	if !s.app.busy {
		t.Fatal("should be busy after submit")
	}

	s.typeText("second")
	s.press("enter")
	if !s.app.busy {
		t.Error("should still be busy during blocked submit")
	}
	if s.app.composer.Text() != "second" {
		t.Errorf("composer should keep text, got %q", s.app.composer.Text())
	}

	userMsgs := 0
	for _, m := range s.app.transcript.Messages() {
		if m.Kind == MsgUser {
			userMsgs++
		}
	}
	if userMsgs != 1 {
		t.Errorf("should have 1 user msg, got %d", userMsgs)
	}
}

// ============================================================
// Test 10: Ctrl+C and Ctrl+L
// ============================================================

func TestSimCtrlCAndCtrlL(t *testing.T) {
	s := newSimSession(t)

	s.typeText("some text")
	s.press("ctrl+c")
	if !s.app.composer.IsEmpty() {
		t.Error("ctrl+c should clear composer")
	}

	s.typeText("msg1")
	s.submit()
	s.respond("resp1")
	if len(s.app.transcript.Messages()) == 0 {
		t.Fatal("should have messages before clear")
	}

	s.press("ctrl+l")
	if len(s.app.transcript.Messages()) != 0 {
		t.Error("ctrl+l should clear transcript")
	}
}

// ============================================================
// Test 11: Scrolling
// ============================================================

func TestSimScrolling(t *testing.T) {
	s := newSimSession(t)

	for i := 0; i < 20; i++ {
		s.typeText("msg" + string(rune('A'+i)))
		s.submit()
		s.respond("resp" + string(rune('A'+i)))
	}

	msgs := s.app.transcript.Messages()
	if len(msgs) != 40 {
		t.Fatalf("expected 40 messages, got %d", len(msgs))
	}

	s.press("pgup")
	if s.app.transcript.atBottom {
		t.Error("should not be at bottom after pgup")
	}

	s.press("pgdown")
	if !s.app.transcript.atBottom {
		t.Error("should be at bottom after pgdown to end")
	}
}

// ============================================================
// Test 12: Async events
// ============================================================

func TestSimAsyncEvents(t *testing.T) {
	s := newSimSession(t)

	s.app.Update(EventMsg{Icon: "email", Source: "weixin", Content: "new message"})
	s.app.Update(EventMsg{Icon: "clock", Source: "cron", Content: "job executed"})

	eventCount := 0
	for _, m := range s.app.transcript.Messages() {
		if m.Kind == MsgEvent {
			eventCount++
		}
	}
	if eventCount != 2 {
		t.Errorf("expected 2 events, got %d", eventCount)
	}

	v := s.viewContent()
	if !strings.Contains(v, "weixin") {
		t.Error("view should contain weixin event")
	}
}

// ============================================================
// Test 13: Tool call tree
// ============================================================

func TestSimToolCallTree(t *testing.T) {
	s := newSimSession(t)

	toolMsg := TranscriptMsg{
		Kind: MsgToolCall,
		Tool: &ToolCall{
			Name:   "Bash",
			Detail: "ls -la",
			Status: ToolStatusDone,
			Output: "file1.txt",
			Children: []ToolCall{
				{Name: "ReadFile", Detail: "file1.txt", Status: ToolStatusDone},
			},
		},
	}
	s.app.transcript.Append(toolMsg)

	// Check the raw Format output contains tool names (shortName: bash, read)
	tree := toolMsg.Tool.FormatTree(0)
	joined := strings.Join(tree, "\n")
	if !strings.Contains(joined, "bash") {
		t.Errorf("FormatTree should contain 'bash': %q", joined)
	}
	if !strings.Contains(joined, "read") {
		t.Errorf("FormatTree should contain 'read': %q", joined)
	}
	if !strings.Contains(joined, "┊") {
		t.Errorf("FormatTree should contain tree indent: %q", joined)
	}
}

// ============================================================
// Test 14: Markdown rendering
// ============================================================

func TestSimMarkdownRendering(t *testing.T) {
	s := newSimSession(t)

	s.respond("Here is **bold** and `code`.")
	msgs := s.app.transcript.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	v := s.viewContent()
	if v == "" {
		t.Error("view should not be empty after markdown response")
	}
}

// ============================================================
// Test 15: Status bar
// ============================================================

func TestSimStatusBar(t *testing.T) {
	s := newSimSession(t)

	v := s.viewContent()
	if !strings.Contains(v, "●") {
		t.Error("status bar should contain status dot")
	}

	s.typeText("test")
	s.submit()
	v = s.viewContent()
	if !strings.Contains(v, "busy") {
		t.Error("status bar should show busy state")
	}

	s.respond("ok")
	v = s.viewContent()
	if strings.Contains(v, "busy") {
		t.Error("status bar should not show busy after response")
	}
}

// ============================================================
// Test 16: All themes render
// ============================================================

func TestSimAllThemes(t *testing.T) {
	for _, name := range []string{"tokyo-night", "catppuccin-mocha", "light", "terminal"} {
		t.Run(name, func(t *testing.T) {
			th := LoadTheme(name)
			app := NewApp(Deps{Theme: th})
			app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

			for _, r := range "test" {
				app.Update(tea.KeyPressMsg{Text: string(r)})
			}
			app.Update(tea.KeyPressMsg{Text: "enter"})
			app.Update(agentResponseMsg{text: "ok"})

			v := app.View()
			if v.Content == "" {
				t.Errorf("theme %s: empty view", name)
			}
		})
	}
}

// ============================================================
// Test 17: Log buffer
// ============================================================

func TestSimLogBuffer(t *testing.T) {
	logBuf := NewLogBuffer(50)
	for i := 0; i < 10; i++ {
		logBuf.AppendLevel("INFO", "test message")
	}

	last := logBuf.Last(5)
	if len(last) != 5 {
		t.Errorf("Last(5) = %d, want 5", len(last))
	}
	last = logBuf.Last(100)
	if len(last) != 10 {
		t.Errorf("Last(100) with 10 entries = %d, want 10", len(last))
	}
}

// ============================================================
// Test 18: Send callback streaming
// ============================================================

func TestSimComposerWithSendCallback(t *testing.T) {
	th := LoadTheme("tokyo-night")
	app := NewApp(Deps{Theme: th})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	app.Update(streamChunkMsg{text: "chunk1"})
	app.Update(streamChunkMsg{text: "chunk2"})

	msgs := app.transcript.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "chunk1chunk2" {
		t.Errorf("streamed content: got %q", msgs[0].Content)
	}
}

// ============================================================
// Test 19: Full realistic session (English only for reliability)
// ============================================================

func TestSimRealisticSession(t *testing.T) {
	s := newSimSession(t)

	// 1. User asks a question
	s.typeText("write a hello world program")
	s.submit()

	// 2. Agent responds (use respond only, not streaming + respond which doubles)
	s.respond("Here is a Go hello world:\n```go\npackage main\nfunc main() { fmt.Println(\"hi\") }\n```")

	// 3. History recall
	s.press("up")
	if s.app.composer.Text() != "write a hello world program" {
		t.Errorf("history recall: got %q", s.app.composer.Text())
	}
	s.press("down")

	// 4. Another question
	s.typeText("change to python")
	s.submit()
	s.respond("```python\nprint('hello world')\n```")

	// 5. Slash command
	s.typeText("/status")
	s.submit()
	if len(s.events) != 1 || s.events[0] != "/status" {
		t.Errorf("expected /status event, got %v", s.events)
	}

	// 6. Async event
	s.app.Update(EventMsg{Icon: "msg", Source: "weixin", Content: "new msg"})

	// 7. Verify transcript: user + assistant + user + assistant + event = 5
	msgs := s.app.transcript.Messages()
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}

	expected := []MsgKind{MsgUser, MsgAssistant, MsgUser, MsgAssistant, MsgEvent}
	for i, m := range msgs {
		if m.Kind != expected[i] {
			t.Errorf("msg[%d]: got %v, want %v", i, m.Kind, expected[i])
		}
	}

	// 8. View renders
	v := s.viewContent()
	if v == "" {
		t.Error("final view should not be empty")
	}
	if !strings.Contains(v, "hello world") {
		t.Error("view should contain response content")
	}
	if !strings.Contains(v, "weixin") {
		t.Error("view should contain event")
	}

	// 9. Ctrl+L clears
	s.press("ctrl+l")
	if len(s.app.transcript.Messages()) != 0 {
		t.Error("ctrl+l should clear transcript")
	}

	// 10. /exit command
	s.typeText("/exit")
	s.submit()
	if len(s.events) != 2 || s.events[1] != "/exit" {
		t.Errorf("expected /exit, got %v", s.events)
	}
}

// ============================================================
// Test 20: Window resize
// ============================================================

func TestSimWindowResize(t *testing.T) {
	s := newSimSession(t)

	if s.app.width != 100 || s.app.height != 30 {
		t.Errorf("initial size: %dx%d", s.app.width, s.app.height)
	}

	s.app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if s.app.width != 120 || s.app.height != 40 {
		t.Errorf("after resize: %dx%d", s.app.width, s.app.height)
	}
	if s.app.transcript.height != 34 {
		t.Errorf("transcript height: %d, want 34", s.app.transcript.height)
	}

	s.typeText("test")
	s.submit()
	s.respond("response")
	if s.viewContent() == "" {
		t.Error("view after resize should not be empty")
	}
}

// ============================================================
// Test 21: Error response
// ============================================================

func TestSimErrorResponse(t *testing.T) {
	s := newSimSession(t)

	s.typeText("trigger error")
	s.submit()
	s.app.Update(agentResponseMsg{err: context.Canceled})

	found := false
	for _, m := range s.app.transcript.Messages() {
		if m.Kind == MsgAssistant && strings.Contains(m.Content, "Error:") {
			found = true
		}
	}
	if !found {
		t.Error("should have error message in transcript")
	}
}

// ============================================================
// Test 22: Completion engine
// ============================================================

func TestSimCompletionEngine(t *testing.T) {
	compEng := NewCompletionEngine(defaultCommands)

	results := compEng.Match("/he")
	if len(results) == 0 {
		t.Error("should match /he to /help")
	}
	found := false
	for _, r := range results {
		if r.Text == "/help" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected /help in results: %+v", results)
	}

	if len(compEng.Match("hello")) != 0 {
		t.Error("non-slash input should return no results")
	}
	if len(compEng.Match("")) != 0 {
		t.Error("empty input should return no results")
	}
}

// ============================================================
// Test 23: History search
// ============================================================

func TestSimHistorySearch(t *testing.T) {
	hist := NewHistory("", 100)
	hist.Add("git status")
	hist.Add("git commit")
	hist.Add("go build")
	hist.Add("go test")

	if len(hist.Search("go")) != 2 {
		t.Errorf("search 'go': expected 2")
	}
	if len(hist.Search("git")) != 2 {
		t.Errorf("search 'git': expected 2")
	}
	if len(hist.Search("xyz")) != 0 {
		t.Errorf("search 'xyz': expected 0")
	}
}

// ============================================================
// Test 24: Tick
// ============================================================

func TestSimTickUpdatesElapsed(t *testing.T) {
	s := newSimSession(t)
	s.app.Update(tickMsg{})
	if s.viewContent() == "" {
		t.Error("view should not be empty after tick")
	}
}

// ============================================================
// Test 25: Paste
// ============================================================

func TestSimPaste(t *testing.T) {
	s := newSimSession(t)

	s.app.Update(tea.PasteMsg{Content: "pasted text"})
	if s.app.composer.Text() != "pasted text" {
		t.Errorf("after paste: got %q", s.app.composer.Text())
	}

	s.app.Update(tea.PasteMsg{Content: " more"})
	if s.app.composer.Text() != "pasted text more" {
		t.Errorf("after second paste: got %q", s.app.composer.Text())
	}
}

// ============================================================
// Test 26: Chinese input
// ============================================================

func TestSimChineseInput(t *testing.T) {
	s := newSimSession(t)

	s.typeText("你好世界")
	s.submit()
	s.respond("你好！有什么可以帮你的？")

	msgs := s.app.transcript.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "你好世界" {
		t.Errorf("user msg: got %q", msgs[0].Content)
	}
	if msgs[1].Content != "你好！有什么可以帮你的？" {
		t.Errorf("assistant msg: got %q", msgs[1].Content)
	}

	// History recall with Chinese
	s.press("up")
	if s.app.composer.Text() != "你好世界" {
		t.Errorf("history with Chinese: got %q", s.app.composer.Text())
	}
}
