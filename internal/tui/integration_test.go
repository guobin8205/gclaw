package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
)

// keyFor creates a KeyPressMsg with the correct Code for special keys.
func keyFor(name string) tea.KeyPressMsg {
	switch name {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "ctrl+enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+l":
		return tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "delete":
		return tea.KeyPressMsg{Code: tea.KeyDelete}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	default:
		return tea.KeyPressMsg{Text: name}
	}
}

type simSession struct {
	app    *App
	t      *testing.T
	events []string
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
	s.app.Update(keyFor(key))
}

func (s *simSession) submit()            { s.press("enter") }
func (s *simSession) respond(text string) { s.app.Update(agentResponseMsg{text: text}) }
func (s *simSession) streamChunk(text string) { s.app.Update(streamChunkMsg{text: text}) }
func (s *simSession) viewContent() string     { return s.app.View().Content }

// ============================================================
// Test 1: Basic typing and editing
// ============================================================

func TestSimBasicTyping(t *testing.T) {
	s := newSimSession(t)

	s.typeText("hello")
	if s.app.composer.Text() != "hello" {
		t.Errorf("after typing: got %q", s.app.composer.Text())
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
		t.Errorf("after esc: got %q", s.app.composer.Text())
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

	for i := 0; i < 5; i++ {
		s.press("backspace")
	}
	if s.app.composer.Text() != "line1\n" {
		t.Errorf("after deleting line2: got %q", s.app.composer.Text())
	}

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
		t.Fatalf("transcript: got %+v", msgs)
	}

	s.respond("Go is a compiled language.")
	if s.app.busy {
		t.Error("should not be busy after response")
	}

	msgs = s.app.transcript.Messages()
	if len(msgs) != 2 || msgs[1].Kind != MsgAssistant {
		t.Fatalf("expected 2 msgs: got %+v", msgs)
	}
}

// ============================================================
// Test 5: Streaming output
// ============================================================

func TestSimStreaming(t *testing.T) {
	s := newSimSession(t)

	s.streamChunk("Hello ")
	s.streamChunk("streaming!")

	msgs := s.app.transcript.Messages()
	if len(msgs) != 1 || msgs[0].Content != "Hello streaming!" {
		t.Fatalf("streaming: got %+v", msgs)
	}

	s.respond("")
	// Streaming continues on same assistant msg (real dialog has user msg between)
	s.typeText("next"); s.submit()
	s.streamChunk("Second response")

	msgs = s.app.transcript.Messages()
	if len(msgs) != 3 || msgs[2].Content != "Second response" {
		t.Fatalf("after respond+stream: got %+v", msgs)
	}
}

// ============================================================
// Test 6: Slash commands
// ============================================================

func TestSimSlashCommands(t *testing.T) {
	s := newSimSession(t)

	s.submit() // empty → no action
	if len(s.events) != 0 {
		t.Error("empty input should not trigger slash command")
	}

	s.typeText("/help")
	s.submit()
	if len(s.events) != 1 || s.events[0] != "/help" {
		t.Errorf("expected /help, got %v", s.events)
	}
	if !s.app.composer.IsEmpty() {
		t.Errorf("composer should be cleared, got %q", s.app.composer.Text())
	}
}

// ============================================================
// Test 7: History navigation
// ============================================================

func TestSimHistory(t *testing.T) {
	s := newSimSession(t)

	s.typeText("first"); s.submit(); s.respond("r1")
	s.typeText("second"); s.submit(); s.respond("r2")
	s.typeText("third"); s.submit(); s.respond("r3")

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
		t.Errorf("up 3x: got %q", s.app.composer.Text())
	}
	// After Up, cursor is at line start; first Down moves to line end
	s.press("down")
	if s.app.composer.Text() != "first" {
		t.Errorf("down to end: got %q", s.app.composer.Text())
	}
	s.press("down")
	if s.app.composer.Text() != "second" {
		t.Errorf("down 2x: got %q", s.app.composer.Text())
	}
	s.press("down")
	if s.app.composer.Text() != "third" {
		t.Errorf("down 3x: got %q", s.app.composer.Text())
	}
	s.press("down")
	if s.app.composer.Text() != "" {
		t.Errorf("down past end: got %q", s.app.composer.Text())
	}
}

// ============================================================
// Test 8: Approval flow
// ============================================================

func TestSimApprovalFlow(t *testing.T) {
	s := newSimSession(t)

	s.app.approval = &ApprovalRequest{ToolName: "Bash", Detail: "rm -rf /tmp"}
	v := s.viewContent()
	if !strings.Contains(v, "Approval") {
		t.Error("view should show approval")
	}

	// Unknown key keeps approval pending
	s.typeText("x")
	if s.app.composer.Text() != "" {
		t.Error("unknown key during approval should not go to composer")
	}
	if s.app.approval == nil {
		t.Error("approval should stay pending on unknown key")
	}

	// 'n' denies
	s.typeText("n") // printable 'n' → msg.String() = "n" → HandleKey("n") → Deny
	if s.app.approval != nil {
		t.Error("approval should be cleared after deny")
	}

	// 'y' allows
	s.app.approval = &ApprovalRequest{ToolName: "WriteFile", Detail: "/tmp/test.go"}
	s.typeText("y")
	if s.app.approval != nil {
		t.Error("approval should be cleared after allow")
	}

	// 'a' always allow
	s.app.approval = &ApprovalRequest{ToolName: "ReadFile", Detail: "main.go"}
	s.typeText("a")
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
		t.Fatal("should be busy")
	}

	s.typeText("second")
	s.submit()
	if !s.app.busy {
		t.Error("should still be busy")
	}
	if s.app.composer.Text() != "second" {
		t.Errorf("composer should keep text, got %q", s.app.composer.Text())
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

	s.typeText("msg1"); s.submit(); s.respond("resp1")
	if len(s.app.transcript.Messages()) == 0 {
		t.Fatal("should have messages")
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

	if len(s.app.transcript.Messages()) != 40 {
		t.Fatalf("expected 40 messages, got %d", len(s.app.transcript.Messages()))
	}

	s.press("pgup")
	if s.app.transcript.atBottom {
		t.Error("should not be at bottom after pgup")
	}
	s.press("pgdown")
	if !s.app.transcript.atBottom {
		t.Error("should be at bottom after pgdown")
	}
}

// ============================================================
// Test 12: Async events
// ============================================================

func TestSimAsyncEvents(t *testing.T) {
	s := newSimSession(t)

	s.app.Update(EventMsg{Icon: "email", Source: "weixin", Content: "msg"})
	eventCount := 0
	for _, m := range s.app.transcript.Messages() {
		if m.Kind == MsgEvent {
			eventCount++
		}
	}
	if eventCount != 1 {
		t.Errorf("expected 1 event, got %d", eventCount)
	}
}

// ============================================================
// Test 13: Tool call tree
// ============================================================

func TestSimToolCallTree(t *testing.T) {
	tc := &ToolCall{
		Name:   "Bash", Detail: "ls -la", Status: ToolStatusDone,
		Children: []ToolCall{
			{Name: "ReadFile", Detail: "file1.txt", Status: ToolStatusDone},
		},
	}
	tree := tc.FormatTree(0)
	joined := strings.Join(tree, "\n")
	if !strings.Contains(joined, "bash") {
		t.Errorf("should contain bash: %q", joined)
	}
	if !strings.Contains(joined, "read") {
		t.Errorf("should contain read: %q", joined)
	}
}

// ============================================================
// Test 14: Markdown rendering
// ============================================================

func TestSimMarkdownRendering(t *testing.T) {
	s := newSimSession(t)
	s.respond("**bold** and `code`.")
	if len(s.app.transcript.Messages()) != 1 {
		t.Fatalf("expected 1, got %d", len(s.app.transcript.Messages()))
	}
}

// ============================================================
// Test 15: Status bar
// ============================================================

func TestSimStatusBar(t *testing.T) {
	s := newSimSession(t)
	v := s.viewContent()
	if !strings.Contains(v, "●") {
		t.Error("should have status dot")
	}

	s.typeText("test"); s.submit()
	v = s.viewContent()
	if !strings.Contains(v, "busy") {
		t.Error("should show busy")
	}

	s.respond("ok")
	v = s.viewContent()
	if strings.Contains(v, "busy") {
		t.Error("should not show busy after response")
	}
}

// ============================================================
// Test 16: All themes
// ============================================================

func TestSimAllThemes(t *testing.T) {
	for _, name := range []string{"tokyo-night", "catppuccin-mocha", "light", "terminal"} {
		t.Run(name, func(t *testing.T) {
			app := NewApp(Deps{Theme: LoadTheme(name)})
			app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			if app.View().Content == "" {
				t.Errorf("theme %s: empty view", name)
			}
		})
	}
}

// ============================================================
// Test 17: Log buffer
// ============================================================

func TestSimLogBuffer(t *testing.T) {
	lb := NewLogBuffer(50)
	for i := 0; i < 10; i++ {
		lb.AppendLevel("INFO", "msg")
	}
	if len(lb.Last(5)) != 5 {
		t.Errorf("Last(5) = %d", len(lb.Last(5)))
	}
	if len(lb.Last(100)) != 10 {
		t.Errorf("Last(100) = %d", len(lb.Last(100)))
	}
}

// ============================================================
// Test 18: Send callback streaming
// ============================================================

func TestSimSendCallbackStreaming(t *testing.T) {
	app := NewApp(Deps{Theme: LoadTheme("tokyo-night")})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app.Update(streamChunkMsg{text: "chunk1"})
	app.Update(streamChunkMsg{text: "chunk2"})
	msgs := app.transcript.Messages()
	if len(msgs) != 1 || msgs[0].Content != "chunk1chunk2" {
		t.Fatalf("got %+v", msgs)
	}
}

// ============================================================
// Test 19: Realistic session
// ============================================================

func TestSimRealisticSession(t *testing.T) {
	s := newSimSession(t)

	s.typeText("write a hello world program")
	s.submit()
	s.respond("Here is a Go hello world:\n```go\npackage main\nfunc main() {}\n```")

	s.press("up")
	if s.app.composer.Text() != "write a hello world program" {
		t.Errorf("history: got %q", s.app.composer.Text())
	}
	s.press("down")

	s.typeText("change to python")
	s.submit()
	s.respond("```python\nprint('hello')\n```")

	s.typeText("/status")
	s.submit()
	if len(s.events) != 1 || s.events[0] != "/status" {
		t.Errorf("expected /status, got %v", s.events)
	}

	s.app.Update(EventMsg{Icon: "msg", Source: "weixin", Content: "new"})

	msgs := s.app.transcript.Messages()
	// user + assistant + user + assistant + event = 5
	if len(msgs) != 5 {
		t.Fatalf("expected 5 msgs, got %d: %+v", len(msgs), msgs)
	}

	v := s.viewContent()
	if v == "" || !strings.Contains(v, "hello world") {
		t.Error("view should contain response")
	}

	s.press("ctrl+l")
	if len(s.app.transcript.Messages()) != 0 {
		t.Error("ctrl+l should clear")
	}
}

// ============================================================
// Test 20: Window resize
// ============================================================

func TestSimWindowResize(t *testing.T) {
	s := newSimSession(t)
	s.app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if s.app.width != 120 || s.app.height != 40 {
		t.Errorf("after resize: %dx%d", s.app.width, s.app.height)
	}
}

// ============================================================
// Test 21: Error response
// ============================================================

func TestSimErrorResponse(t *testing.T) {
	s := newSimSession(t)
	s.typeText("err"); s.submit()
	s.app.Update(agentResponseMsg{err: context.Canceled})
	found := false
	for _, m := range s.app.transcript.Messages() {
		if m.Kind == MsgAssistant && strings.Contains(m.Content, "Error:") {
			found = true
		}
	}
	if !found {
		t.Error("should have error msg")
	}
}

// ============================================================
// Test 22: Completion engine
// ============================================================

func TestSimCompletionEngine(t *testing.T) {
	ce := NewCompletionEngine(defaultCommands)
	results := ce.Match("/he")
	if len(results) == 0 {
		t.Fatal("should match /he")
	}
	found := false
	for _, r := range results {
		if r.Text == "/help" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected /help: %+v", results)
	}
}

// ============================================================
// Test 23: History search
// ============================================================

func TestSimHistorySearch(t *testing.T) {
	h := NewHistory("", 100)
	h.Add("git status"); h.Add("go build")
	if len(h.Search("go")) != 1 {
		t.Errorf("search 'go': expected 1")
	}
}

// ============================================================
// Test 24: Tick
// ============================================================

func TestSimTick(t *testing.T) {
	s := newSimSession(t)
	s.app.Update(tickMsg{})
	if s.viewContent() == "" {
		t.Error("should not be empty after tick")
	}
}

// ============================================================
// Test 25: Paste
// ============================================================

func TestSimPaste(t *testing.T) {
	s := newSimSession(t)
	s.app.Update(tea.PasteMsg{Content: "pasted"})
	if s.app.composer.Text() != "pasted" {
		t.Errorf("got %q", s.app.composer.Text())
	}
}

// ============================================================
// Test 26: Chinese input
// ============================================================

func TestSimChineseInput(t *testing.T) {
	s := newSimSession(t)
	s.typeText("你好世界")
	s.submit()
	s.respond("你好！")
	msgs := s.app.transcript.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2, got %d", len(msgs))
	}
	if msgs[0].Content != "你好世界" {
		t.Errorf("user: got %q", msgs[0].Content)
	}
	s.press("up")
	if s.app.composer.Text() != "你好世界" {
		t.Errorf("history: got %q", s.app.composer.Text())
	}
}

// ============================================================
// Test 22: Theme switch triggers OnThemeChange callback
// ============================================================

func TestSimThemeSwitchCallback(t *testing.T) {
	var themeChanged string
	th := LoadTheme("tokyo-night")
	app := NewApp(Deps{
		Theme:  th,
		LogBuf: NewLogBuffer(200),
		OnThemeChange: func(name string) {
			themeChanged = name
		},
	})
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Type and submit /theme catppuccin-mocha
	for _, r := range "/theme catppuccin-mocha" {
		app.Update(tea.KeyPressMsg{Text: string(r)})
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if themeChanged != "catppuccin-mocha" {
		t.Fatalf("OnThemeChange not called or wrong value: got %q", themeChanged)
	}
	if app.theme.Name != "catppuccin-mocha" {
		t.Fatalf("theme not switched: got %q", app.theme.Name)
	}

	// Unknown theme should NOT trigger callback
	themeChanged = ""
	for _, r := range "/theme nonexistent" {
		app.Update(tea.KeyPressMsg{Text: string(r)})
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if themeChanged != "" {
		t.Fatalf("OnThemeChange should not be called for unknown theme, got %q", themeChanged)
	}
}
