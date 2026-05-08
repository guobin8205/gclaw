package tui

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/openclaw/gclaw/internal/agent"
	"github.com/openclaw/gclaw/internal/config"
)

var dbgLog *log.Logger

func init() {
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "gclaw-composer-debug.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		dbgLog = log.New(f, "", log.Lmicroseconds)
	}
}

type Deps struct {
	Config       *config.Config
	Agent        *agent.Agent
	Theme        Theme
	LogBuf       *LogBuffer
	History      *History
	CompEng      *CompletionEngine
	OnSubmit     func(ctx context.Context, input string, images []string) (string, error)
	OnSlash      func(cmd string)
	Send         func(msg tea.Msg) // tea.Program.Send wrapper
	GetAgentCount func() int
	GetBgTasks   func() int
	IsCronActive func() bool
}

type App struct {
	deps        Deps
	theme       Theme
	styles      Styles
	transcript  *Transcript
	composer    *Composer
	statusbar   *StatusBar
	approval    *ApprovalRequest
	width       int
	height      int
	busy        bool
	startTime   time.Time
	quitConfirm bool
	// Completion state
	compItems []CompletionItem
	compIdx   int
	compActive bool
	// Queue
	queue []string
	// Attachment input mode
	attaching   bool
	attachInput []rune
	cancelFn    context.CancelFunc
	// Selection state
	sel          *Selection
	rawContent   string
	rawLineCount int
}

func (a *App) SetSend(send func(msg tea.Msg)) {
	a.deps.Send = send
}

func NewApp(deps Deps) *App {
	th := deps.Theme
	st := th.Styles()
	sb := NewStatusBar(th)
	if deps.Config != nil {
		sb.SetModel(deps.Config.Model.Default)
	}
	return &App{
		deps:       deps,
		theme:      th,
		styles:     st,
		transcript: NewTranscript(st, th),
		composer:   NewComposer(),
		statusbar:  sb,
		startTime:  time.Now(),
		sel:        newSelection(),
	}
}

func (a *App) updateCompletions() {
	if a.deps.CompEng == nil {
		return
	}
	text := a.composer.Text()
	if strings.HasPrefix(text, "/") && !strings.Contains(text, "\n") {
		items := a.deps.CompEng.Match(text)
		if len(items) > 0 {
			a.compItems = items
			a.compActive = true
			if a.compIdx >= len(items) {
				a.compIdx = 0
			}
			return
		}
	}
	a.compItems = nil
	a.compActive = false
	a.compIdx = 0
}

func (a *App) applyCompletion() {
	if !a.compActive || len(a.compItems) == 0 {
		return
	}
	item := a.compItems[a.compIdx]
	a.composer.SetInput(item.Text + " ")
	a.compItems = nil
	a.compActive = false
	a.compIdx = 0
}

func (a *App) AppendWelcome(content string) {
	a.transcript.Append(TranscriptMsg{
		Kind:      MsgEvent,
		EventIcon: "gclaw",
		EventSrc:  "ready",
		Content:   content,
	})
}

func (a *App) AppendEvent(icon, source, content string) {
	a.transcript.Append(TranscriptMsg{
		Kind:      MsgEvent,
		EventIcon: icon,
		EventSrc:  source,
		Content:   content,
	})
}

func (a *App) SetBanner(version, model, mode string) {
	parts := []string{"gclaw " + version}
	if model != "" {
		parts = append(parts, model)
	}
	if mode != "" {
		parts = append(parts, mode)
	}
	a.transcript.SetBanner(strings.Join(parts, " │ "))
}

func (a *App) SetTheme(name string) bool {
	th := LoadTheme(name)
	if dbgLog != nil {
		dbgLog.Printf("THEME: SetTheme(%q) → loaded %q", name, th.Name)
	}
	if th.Name != name {
		return false
	}
	return a.SetThemeObj(th)
}

func (a *App) SetThemeObj(th Theme) bool {
	if _, ok := Themes[th.Name]; !ok {
		return false
	}
	a.theme = th
	a.styles = th.Styles()
	a.transcript.SetTheme(a.styles, a.theme)
	a.statusbar.SetTheme(a.theme)
	if dbgLog != nil {
		dbgLog.Printf("THEME: SetThemeObj(%q) done, styles.Accent=%v", th.Name, a.styles.Accent)
	}
	return true
}

func (a *App) Init() tea.Cmd {
	return tickCmd()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		a.sel.Clear()
		return a, nil

	case tea.MouseClickMsg:
		mm := m.Mouse()
		if mm.Button == tea.MouseRight && a.sel.HasSelection() {
			start, end, ok := a.sel.Bounds()
			if ok {
				text := SelectedText(a.rawContent, start, end)
				if text != "" {
					a.sel.Clear()
					return a, tea.SetClipboard(text)
				}
			}
			a.sel.Clear()
			return a, nil
		}
		if mm.Button == tea.MouseLeft {
			row := mm.Y
			if row < 0 {
				row = 0
			}
			if a.rawLineCount > 0 && row >= a.rawLineCount {
				row = a.rawLineCount - 1
			}
			a.sel.Clear()
			a.sel.Start(mm.X, row)
		}
		return a, nil

	case tea.MouseMotionMsg:
		if a.sel.IsDragging() {
			mm := m.Mouse()
			row := mm.Y
			if row < 0 {
				row = 0
			}
			if a.rawLineCount > 0 && row >= a.rawLineCount {
				row = a.rawLineCount - 1
			}
			a.sel.Update(mm.X, row)
		}
		return a, nil

	case tea.MouseReleaseMsg:
		if a.sel.IsDragging() {
			a.sel.Finish()
			// Only clear click-without-drag; keep valid selection for Ctrl+C / right-click
			if a.sel.HasSelection() {
				start, end, ok := a.sel.Bounds()
				if ok && start == end {
					a.sel.Clear()
				}
			}
		}
		return a, nil

	case tea.MouseWheelMsg:
		mm := m.Mouse()
		switch mm.Button {
		case tea.MouseWheelUp:
			a.transcript.ScrollUp(3)
		case tea.MouseWheelDown:
			a.transcript.ScrollDown(3)
		}
		return a, nil

	case tea.KeyPressMsg:
		// Selection-aware keys take priority
		if a.sel.HasSelection() {
			if m.Code == tea.KeyEscape {
				a.sel.Clear()
				return a, nil
			}
			if m.Code == 'c' && m.Mod == tea.ModCtrl {
				start, end, ok := a.sel.Bounds()
				if ok {
					text := SelectedText(a.rawContent, start, end)
					a.sel.Clear()
					if text != "" {
						return a, tea.SetClipboard(text)
					}
				}
				return a, nil
			}
		}
		prevText := a.composer.Text()
		m2, cmd := a.handleKey(m)
		if a.composer.Text() != prevText {
			w, h := a.width, a.height
			return m2, tea.Batch(cmd, func() tea.Msg {
				return tea.WindowSizeMsg{Width: w, Height: h}
			})
		}
		return m2, cmd

	case tea.PasteMsg:
		a.composer.InsertText(m.Content)
		if dbgLog != nil {
			dbgLog.Printf("PASTE: len=%d lines=%d composerLines=%d", len(m.Content), strings.Count(m.Content, "\n")+1, len(a.composer.lines))
		}
		w, h := a.width, a.height
		return a, func() tea.Msg {
			return tea.WindowSizeMsg{Width: w, Height: h}
		}

	case agentResponseMsg:
		a.cancelFn = nil
		a.busy = false
		a.statusbar.SetState("ready")
		if m.err != nil {
			a.transcript.Append(TranscriptMsg{Kind: MsgAssistant, Content: "Error: " + m.err.Error()})
		} else if m.text != "" {
			msgs := a.transcript.Messages()
			if len(msgs) == 0 || msgs[len(msgs)-1].Kind != MsgAssistant {
				a.transcript.Append(TranscriptMsg{Kind: MsgAssistant, Content: m.text})
			} else {
				msgs[len(msgs)-1].Streaming = false
				a.transcript.UpdateLast(msgs[len(msgs)-1])
			}
		}
		// Process queued slash commands
		if len(a.queue) > 0 {
			cmd := a.queue[0]
			a.queue = a.queue[1:]
			if a.deps.OnSlash != nil {
				a.deps.OnSlash(cmd)
			}
		}
		return a, a.fullRedraw()

	case EventMsg:
		a.transcript.Append(TranscriptMsg{
			Kind: MsgEvent, EventIcon: m.Icon, EventSrc: m.Source, Content: m.Content,
		})
		return a, a.fullRedraw()

	case streamChunkMsg:
		msgs := a.transcript.Messages()
		if len(msgs) > 0 && msgs[len(msgs)-1].Kind == MsgAssistant {
			msgs[len(msgs)-1].Content += m.text
			msgs[len(msgs)-1].Streaming = true
			a.transcript.UpdateLast(msgs[len(msgs)-1])
		} else {
			a.transcript.Append(TranscriptMsg{Kind: MsgAssistant, Content: m.text, Streaming: true})
		}
		return a, nil

	case ToolStartEvent:
		a.transcript.Append(TranscriptMsg{
			Kind: MsgToolCall,
			Tool: &ToolCall{
				Name:   m.Name,
				Detail: m.Detail,
				Status: ToolStatusRunning,
			},
		})
		return a, a.fullRedraw()

	case ToolEndEvent:
		tmsgs := a.transcript.Messages()
		for i := len(tmsgs) - 1; i >= 0; i-- {
			if tmsgs[i].Kind == MsgToolCall && tmsgs[i].Tool != nil {
				tc := tmsgs[i].Tool
				if tc.Name == m.Name && tc.Status == ToolStatusRunning {
					if m.Err != nil {
						tc.Status = ToolStatusError
						tc.Output = m.Err.Error()
					} else {
						tc.Status = ToolStatusDone
						tc.Output = m.Output
					}
					tc.Duration = m.Duration.Truncate(time.Millisecond).String()
					tc.Collapsed = true
					break
				}
			}
		}
		return a, a.fullRedraw()

	case tickMsg:
		a.statusbar.SetElapsed(time.Since(a.startTime).Truncate(time.Second).String())
		if a.quitConfirm {
			a.quitConfirm = false
		}
		a.transcript.ToggleCursor()
		// Update status bar with live data from agent
		if a.deps.Agent != nil {
			usage := a.deps.Agent.Usage()
			a.statusbar.SetContextUsage(usage.InputTokens+usage.OutputTokens, a.deps.Agent.MaxTokens())
		}
		if a.deps.GetAgentCount != nil {
			a.statusbar.SetAgentCount(a.deps.GetAgentCount())
		}
		if a.deps.GetBgTasks != nil {
			a.statusbar.SetBackgroundTasks(a.deps.GetBgTasks())
		}
		if a.deps.IsCronActive != nil {
			a.statusbar.SetCronActive(a.deps.IsCronActive())
		}
		return a, tickCmd()

	case quitConfirmTimeoutMsg:
		a.quitConfirm = false
		return a, nil
	}
	return a, nil
}

func (a *App) View() tea.View {
	if a.width == 0 {
		return tea.NewView("Loading...")
	}
	divider := a.styles.Divider.Render(
		strings.Repeat("─", a.width),
	)
	statusView := a.statusbar.Render(a.width)
	var composerView string
	var compScrollOff int
	if a.approval != nil {
		composerView = a.approval.Render(a.theme)
	} else {
		composerView, compScrollOff = a.renderComposer(maxComposerLines)
	}
	compView := a.renderCompletions()
	queueView := a.renderQueue()
	hintView := ""
	if a.approval == nil {
		hintView = a.renderHints()
	}
	// Build non-transcript parts and measure their actual joined height
	var belowParts []string
	if a.busy {
		belowParts = append(belowParts, a.styles.Warning.Render("⏳ agent busy..."))
	}
	belowParts = append(belowParts, divider)
	if queueView != "" {
		belowParts = append(belowParts, queueView)
	}
	if compView != "" {
		belowParts = append(belowParts, compView)
	}
	belowParts = append(belowParts, composerView, divider, statusView)
	if hintView != "" {
		belowParts = append(belowParts, hintView)
	}
	if a.quitConfirm {
		belowParts = append(belowParts, a.styles.Warning.Render("  再按 Ctrl+C 退出"))
	}
	belowJoined := lipgloss.JoinVertical(lipgloss.Left, belowParts...)
	bottomLines := strings.Count(belowJoined, "\n") + 1
	transcriptHeight := a.height - bottomLines
	if transcriptHeight < 3 {
		transcriptHeight = 3
	}
	a.transcript.Resize(a.width, transcriptHeight)
	transcriptView := a.transcript.Render()
	transLines := strings.Count(transcriptView, "\n") + 1
	// Safety: clip transcript if it exceeds allocated height (Width wrapping may add lines)
	if transLines > transcriptHeight {
		lines := strings.Split(transcriptView, "\n")
		transcriptView = strings.Join(lines[:transcriptHeight], "\n")
		transLines = transcriptHeight
	}
	parts := []string{transcriptView}
	parts = append(parts, belowParts...)
	joined := lipgloss.JoinVertical(lipgloss.Left, parts...)
	joinedLines := strings.Count(joined, "\n") + 1
	// Clip if the joined content exceeds terminal height
	clipped := false
	if joinedLines > a.height {
		lines := strings.Split(joined, "\n")
		joined = strings.Join(lines[:a.height], "\n")
		clipped = true
	}
	if dbgLog != nil {
		qLines := 0
		if queueView != "" { qLines = strings.Count(queueView, "\n") + 1 }
		cLines := 0
		if compView != "" { cLines = strings.Count(compView, "\n") + 1 }
		hLines := 0
		if hintView != "" { hLines = strings.Count(hintView, "\n") + 1 }
		qi := 0; if a.quitConfirm { qi = 1 }
		dbgLog.Printf("LAYOUT: theme=%s term=%dx%d busy=%v comp=(total=%d,vis=%d,scroll=%d) queue=%d compH=%d hint=%d quit=%d => bottom=%d transH=%d transRendered=%d joined=%d clipped=%v",
			a.theme.Name, a.width, a.height, a.busy,
			len(a.composer.lines), strings.Count(composerView, "\n")+1,
			compScrollOff,
			qLines, cLines, hLines, qi,
			bottomLines, transcriptHeight, transLines, joinedLines, clipped)
	}
	a.rawContent = joined
	a.rawLineCount = strings.Count(joined, "\n") + 1
	if a.sel.HasSelection() {
		start, end, ok := a.sel.Bounds()
		if ok {
			joined = applySelectionToContent(joined, start, end)
		}
	}
	if dbgLog != nil {
		dbgLog.Printf("VIEW: width=%d height=%d joinedLen=%d", a.width, a.height, len(joined))
	}
	v := tea.NewView(joined)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	if a.approval == nil {
		cursorX, cursorY := a.computeCursorPos(transcriptView, queueView, compView, compScrollOff)
		if a.busy {
			cursorY++
		}
		if dbgLog != nil {
			dbgLog.Printf("CURSOR: x=%d y=%d", cursorX, cursorY)
		}
		v.Cursor = tea.NewCursor(cursorX, cursorY)
		v.Cursor.Blink = true
	}
	return v
}

func (a *App) computeCursorPos(transcriptView, queueView, compView string, compScrollOff int) (int, int) {
	transLines := strings.Count(transcriptView, "\n") + 1
	if transcriptView == "" {
		transLines = 0
	}
	compStartLine := transLines + 1 // divider line
	if queueView != "" {
		compStartLine += strings.Count(queueView, "\n") + 1
	}
	if compView != "" {
		compStartLine += strings.Count(compView, "\n") + 1
	}
	curRow, curCol := a.composer.CursorPos()
	compText := a.composer.Text()
	if compText == "" {
		cursorX := lipgloss.Width(a.styles.Prompt.Render("❯ "))
		if dbgLog != nil {
			dbgLog.Printf("CURSOR: empty comp transLines=%d compStart=%d => (%d,%d)", transLines, compStartLine, cursorX, compStartLine)
		}
		return cursorX, compStartLine
	}
	// Account for truncated composer lines hint
	if compScrollOff > 0 {
		compStartLine++ // "↑ N more lines" hint
	}
	if curRow >= compScrollOff {
		curRow -= compScrollOff
	} else {
		curRow = 0
	}
	clines := strings.Split(compText, "\n")
	if curRow >= 0 && curRow < maxComposerLines {
		prefix := "  "
		if curRow+compScrollOff == 0 {
			prefix = a.styles.Prompt.Render("❯ ")
		}
		prefixW := lipgloss.Width(prefix)
		idx := curRow + compScrollOff
		if idx < len(clines) {
			runes := []rune(clines[idx])
			if curCol > len(runes) {
				curCol = len(runes)
			}
			textBefore := string(runes[:curCol])
			cursorX := prefixW + lipgloss.Width(textBefore)
			if dbgLog != nil {
				dbgLog.Printf("CURSOR: transLines=%d compStart=%d scrollOff=%d visRow=%d visCol=%d idx=%d => (%d,%d)", transLines, compStartLine, compScrollOff, curRow, curCol, idx, cursorX, compStartLine+curRow)
			}
			return cursorX, compStartLine + curRow
		}
	}
	if dbgLog != nil {
		dbgLog.Printf("CURSOR: fallback compStart=%d => (0,%d)", compStartLine, compStartLine)
	}
	return 0, compStartLine
}

func (a *App) handleAttachKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	code := msg.Code
	switch code {
	case tea.KeyEnter:
		path := string(a.attachInput)
		if path != "" {
			isImg := strings.HasSuffix(strings.ToLower(path), ".png") ||
				strings.HasSuffix(strings.ToLower(path), ".jpg") ||
				strings.HasSuffix(strings.ToLower(path), ".jpeg") ||
				strings.HasSuffix(strings.ToLower(path), ".gif") ||
				strings.HasSuffix(strings.ToLower(path), ".webp")
			a.composer.AddAttachment(path, isImg)
		}
		a.attaching = false
		a.attachInput = nil
		return a, nil
	case tea.KeyEscape:
		a.attaching = false
		a.attachInput = nil
		return a, nil
	case tea.KeyBackspace:
		if len(a.attachInput) > 0 {
			a.attachInput = a.attachInput[:len(a.attachInput)-1]
		} else {
			a.attaching = false
		}
		return a, nil
	default:
		if msg.Text != "" {
			a.attachInput = append(a.attachInput, []rune(msg.Text)...)
		}
		return a, nil
	}
}

func (a *App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if dbgLog != nil {
		cr, cc := a.composer.CursorPos()
		dbgLog.Printf("KEY: code=%d mod=%d text=%q composerLines=%d curRow=%d curCol=%d",
			msg.Code, msg.Mod, msg.Text, len(a.composer.lines), cr, cc)
	}
	if a.approval != nil {
		result := a.approval.HandleKey(msg.String())
		if result != ApprovalPending {
			a.approval = nil
		}
		return a, nil
	}

	code := msg.Code
	mod := msg.Mod


	if code == 'c' && mod == tea.ModCtrl {
		if a.busy {
			if a.deps.Agent != nil {
				if a.cancelFn != nil {
					a.deps.Agent.InterruptAndStop("user interrupt", a.cancelFn)
					a.cancelFn = nil
				} else {
					a.deps.Agent.Interrupt("user interrupt")
				}
			}
			a.busy = false
			a.statusbar.SetState("ready")
			a.transcript.Append(TranscriptMsg{Kind: MsgEvent, EventIcon: "⚡", EventSrc: "interrupt", Content: "已中断"})
			return a, nil
		}
		if !a.composer.IsEmpty() {
			a.composer.Clear()
			return a, nil
		}
		if a.quitConfirm {
			return a, tea.Quit
		}
		a.quitConfirm = true
		return a, quitConfirmTimeout()
	}
	a.quitConfirm = false
	if code == tea.KeyEnter && mod == tea.ModCtrl {
		a.composer.InsertNewLine()
		return a, nil
	}
	// Ctrl+J (non-Kitty Ctrl+Enter fallback)
	if code == 'j' && mod == tea.ModCtrl {
		a.composer.InsertNewLine()
		return a, nil
	}
	// Ctrl+O: toggle thinking/tool fold
	if code == 'o' && mod == tea.ModCtrl {
		a.transcript.ToggleFold()
		return a, nil
	}
	// Ctrl+L: clear transcript (preserve banner)
	if (code == 'l' && mod == tea.ModCtrl) || code == 12 {
		a.transcript.msgs = nil
		a.transcript.yOffset = 0
		a.transcript.atBottom = true
		return a, nil
	}

	// Attachment input mode
	if a.attaching {
		return a.handleAttachKey(msg)
	}

	// Ctrl+I: enter attachment mode
	if code == 'i' && mod == tea.ModCtrl {
		a.attaching = true
		a.attachInput = nil
		return a, nil
	}

	switch code {
	case tea.KeyEnter:
		if a.busy {
			// Queue slash commands when busy
			text := a.composer.Text()
			if text != "" && strings.HasPrefix(text, "/") {
				a.queue = append(a.queue, text)
				a.composer.Clear()
				a.compItems = nil
				a.compActive = false
			}
			return a, nil
		}
		return a.submitInput()
	case tea.KeyEscape:
		if a.compActive {
			a.compItems = nil
			a.compActive = false
			a.compIdx = 0
			return a, nil
		}
		a.composer.Clear()
		return a, nil
	case tea.KeyTab:
		a.applyCompletion()
		return a, nil
	case tea.KeyBackspace:
		a.composer.Backspace()
		a.updateCompletions()
		return a, nil
	case tea.KeyDelete:
		a.composer.Delete()
		a.updateCompletions()
		return a, nil
	case tea.KeyLeft:
		a.composer.MoveLeft()
		return a, nil
	case tea.KeyRight:
		a.composer.MoveRight()
		return a, nil
	case tea.KeyHome:
		a.composer.MoveHome()
		return a, nil
	case tea.KeyEnd:
		a.composer.MoveEnd()
		return a, nil
	case tea.KeyUp:
		if a.compActive && len(a.compItems) > 0 {
			a.compIdx--
			if a.compIdx < 0 {
				a.compIdx = len(a.compItems) - 1
			}
			return a, nil
		}
		if a.composer.MoveUp() {
			return a, nil
		}
		if !a.composer.AtFirstLineStart() {
			a.composer.MoveHome()
			return a, nil
		}
		if a.deps.History != nil {
			if entry := a.deps.History.Older(); entry != "" {
				a.composer.SetInput(entry)
				a.composer.MoveToStart()
			}
		}
		return a, nil
	case tea.KeyDown:
		if a.compActive && len(a.compItems) > 0 {
			a.compIdx++
			if a.compIdx >= len(a.compItems) {
				a.compIdx = 0
			}
			return a, nil
		}
		if a.composer.MoveDown() {
			return a, nil
		}
		if !a.composer.AtLastLineEnd() {
			a.composer.MoveEnd()
			return a, nil
		}
		if a.deps.History != nil {
			if entry := a.deps.History.Newer(); entry != "" {
				a.composer.SetInput(entry)
			} else {
				a.composer.Clear()
			}
		}
		return a, nil
	case tea.KeyPgUp:
		a.transcript.ScrollUp(a.transcript.height)
		return a, nil
	case tea.KeyPgDown:
		a.transcript.ScrollDown(a.transcript.height)
		return a, nil
	default:
		if msg.Text != "" {
			for _, r := range msg.Text {
				a.composer.InsertRune(r)
			}
			a.updateCompletions()
		} else if code != 0 && code < 256 && code >= 32 {
			a.composer.InsertRune(code)
		}
		return a, nil
	}
}

func (a *App) submitInput() (tea.Model, tea.Cmd) {
	text := a.composer.Text()
	if text == "" {
		return a, nil
	}
	// Close completions on submit
	a.compItems = nil
	a.compActive = false
	a.compIdx = 0
	if strings.HasPrefix(text, "/") {
		// /theme is handled locally (needs App state)
		if strings.HasPrefix(text, "/theme") {
			a.handleThemeCmd(text)
			a.composer.Clear()
			return a, a.fullRedraw()
		}
		if a.deps.OnSlash != nil {
			a.deps.OnSlash(text)
		}
		a.composer.Clear()
		return a, nil
	}
	if a.deps.History != nil {
		a.deps.History.Add(text)
	}
	a.transcript.Append(TranscriptMsg{Kind: MsgUser, Content: text})
	var images []string
	for _, att := range a.composer.Attachments() {
		if att.IsImage {
			images = append(images, att.Path)
		}
	}
		a.composer.Clear()
		a.busy = true
		a.statusbar.SetState("busy")
		ctx, cancel := context.WithCancel(context.Background())
		a.cancelFn = cancel
		return a, runAgentCmd(a.deps, ctx, text, images)
	}

func (a *App) handleThemeCmd(text string) {
	if dbgLog != nil {
		dbgLog.Printf("THEME: handleThemeCmd(%q)", text)
	}
	parts := strings.Fields(text)
	if len(parts) == 1 {
		a.transcript.Append(TranscriptMsg{Kind: MsgEvent, EventIcon: "🎨", EventSrc: "theme", Content: fmt.Sprintf("current: %s | tokyo-night, catppuccin-mocha, light, terminal", a.theme.Name)})
		return
	}
	name := parts[1]
	if a.SetTheme(name) {
		a.transcript.Append(TranscriptMsg{Kind: MsgEvent, EventIcon: "🎨", EventSrc: "theme", Content: fmt.Sprintf("switched to %s", name)})
	} else {
		a.transcript.Append(TranscriptMsg{Kind: MsgEvent, EventIcon: "⚠", EventSrc: "theme", Content: fmt.Sprintf("unknown theme: %s (tokyo-night, catppuccin-mocha, light, terminal)", name)})
	}
}

func (a *App) fullRedraw() tea.Cmd {
	return tickCmd()
}

func (a *App) renderCompletions() string {
	if !a.compActive || len(a.compItems) == 0 {
		return ""
	}
	items := a.compItems
	maxVisible := 8
	start := a.compIdx - maxVisible/2
	if start < 0 {
		start = 0
	}
	if len(items) > maxVisible && start > len(items)-maxVisible {
		start = len(items) - maxVisible
	}
	end := start + maxVisible
	if end > len(items) {
		end = len(items)
	}
	visible := items[start:end]
	var lines []string
	for i, item := range visible {
		realIdx := start + i
		line := item.Display + " " + a.styles.Muted.Render("— "+item.Description)
		if realIdx == a.compIdx {
			line = a.styles.CompActive.Render(line)
		} else {
			line = a.styles.Completion.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (a *App) renderQueue() string {
	if !a.busy || len(a.queue) == 0 {
		return ""
	}
	summary := fmt.Sprintf("⏳ %d 条排队 ", len(a.queue))
	items := a.queue
	if len(items) > 3 {
		items = items[:3]
	}
	summary += strings.Join(items, " · ")
	if len(a.queue) > 3 {
		summary += fmt.Sprintf(" · +%d", len(a.queue)-3)
	}
	return a.styles.Warning.Render(summary)
}

const maxComposerLines = 5

func (a *App) renderComposer(maxLines int) (view string, scrollOff int) {
	var parts []string
	text := a.composer.Text()
	if text == "" {
		parts = append(parts, a.styles.Prompt.Render("❯ "))
		return strings.Join(parts, "\n"), 0
	}
	lines := strings.Split(text, "\n")
	start, _ := a.composer.VisibleRange(maxLines)
	end := start + maxLines
	if end > len(lines) {
		end = len(lines)
	}
	if start > 0 {
		parts = append(parts, a.styles.Muted.Render(fmt.Sprintf("↑ %d more lines", start)))
	}
	for i := start; i < end; i++ {
		line := lines[i]
		if i == 0 {
			parts = append(parts, a.styles.Prompt.Render("❯ ")+line)
		} else {
			parts = append(parts, "  "+line)
		}
	}
	if end < len(lines) {
		parts = append(parts, a.styles.Muted.Render(fmt.Sprintf("↓ %d more lines", len(lines)-end)))
	}
	for _, att := range a.composer.Attachments() {
		icon := "📎"
		if att.IsImage {
			icon = "🖼"
		}
		parts = append(parts, a.styles.EventPrefix.Render(icon+" "+att.Path)+" "+a.styles.Error.Render("✕"))
	}
	return strings.Join(parts, "\n"), start
}

func (a *App) renderHints() string {
	if a.attaching {
		return a.styles.Accent.Render("📎 文件路径: ") + string(a.attachInput)
	}
	return a.styles.Muted.Render("Ctrl+Enter:换行 Enter:发送 Ctrl+I:附加文件 Esc:取消")
}

type agentResponseMsg struct {
	text string
	err  error
}

type EventMsg struct {
	Icon    string
	Source  string
	Content string
}

type streamChunkMsg struct {
	text string
}

type ToolStartEvent struct {
	Name   string
	Detail string
}

type ToolEndEvent struct {
	Name     string
	Output   string
	Err      error
	Duration time.Duration
}

type tickMsg time.Time
type quitConfirmTimeoutMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func quitConfirmTimeout() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return quitConfirmTimeoutMsg(t) })
}

func runAgentCmd(deps Deps, ctx context.Context, input string, images []string) tea.Cmd {
	return func() tea.Msg {
		if deps.OnSubmit != nil {
			if deps.Agent != nil && deps.Send != nil {
				text, err := deps.Agent.RunStreaming(ctx, input, func(chunk string) {
					deps.Send(streamChunkMsg{text: chunk})
				})
				if err != nil {
					return agentResponseMsg{err: err}
				}
				return agentResponseMsg{text: text}
			}
			text, err := deps.OnSubmit(ctx, input, images)
			return agentResponseMsg{text: text, err: err}
		}
		return agentResponseMsg{}
	}
}
