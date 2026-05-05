package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/openclaw/gclaw/internal/agent"
	"github.com/openclaw/gclaw/internal/config"
)

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
	attaching    bool
	attachInput  []rune
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

func (a *App) Init() tea.Cmd {
	return tickCmd()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		a.transcript.Resize(a.width, a.height-6)
		return a, nil

	case tea.KeyPressMsg:
		return a.handleKey(m)

	case tea.PasteMsg:
		a.composer.SetInput(a.composer.Text() + m.Content)
		return a, nil

	case agentResponseMsg:
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
		return a, nil

	case EventMsg:
		a.transcript.Append(TranscriptMsg{
			Kind: MsgEvent, EventIcon: m.Icon, EventSrc: m.Source, Content: m.Content,
		})
		return a, nil

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
	transcriptView := a.transcript.Render()
	divider := lipgloss.NewStyle().Foreground(lipgloss.Color(a.theme.Border)).Render(
		strings.Repeat("─", a.width),
	)
	statusView := a.statusbar.Render(a.width)
	var composerView string
	if a.approval != nil {
		composerView = a.approval.Render(a.theme)
	} else {
		composerView = a.renderComposer()
	}
	compView := a.renderCompletions()
	queueView := a.renderQueue()
	parts := []string{transcriptView, divider, statusView}
	if queueView != "" {
		parts = append(parts, queueView)
	}
	if compView != "" {
		parts = append(parts, compView)
	}
	parts = append(parts, composerView)
	if a.quitConfirm {
		hint := a.styles.Warning.Render("  再按 Ctrl+C 退出")
		parts = append(parts, hint)
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
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
				a.deps.Agent.Interrupt("user interrupt")
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

	if code == 'l' && mod == tea.ModCtrl {
		a.transcript = NewTranscript(a.styles, a.theme)
		a.transcript.Resize(a.width, a.height-6)
		return a, nil
	}
	if code == tea.KeyEnter && mod == tea.ModCtrl {
		a.composer.InsertNewLine()
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
		if a.deps.History != nil {
			if entry := a.deps.History.Older(); entry != "" {
				a.composer.SetInput(entry)
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
		if a.deps.History != nil {
			a.composer.SetInput(a.deps.History.Newer())
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
	return a, runAgentCmd(a.deps, text, images)
}

func (a *App) renderCompletions() string {
	if !a.compActive || len(a.compItems) == 0 {
		return ""
	}
	maxVisible := 8
	items := a.compItems
	if len(items) > maxVisible {
		items = items[:maxVisible]
	}
	var lines []string
	for i, item := range items {
		line := item.Display + " " + a.styles.Muted.Render("— "+item.Description)
		if i == a.compIdx {
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

func (a *App) renderComposer() string {
	var parts []string
	if a.busy {
		parts = append(parts, a.styles.Warning.Render("⏳ agent busy..."))
	}
	text := a.composer.Text()
	if text == "" {
		parts = append(parts, a.styles.Prompt.Render("❯ "))
	} else {
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			if i == 0 {
				parts = append(parts, a.styles.Prompt.Render("❯ ")+line)
			} else {
				parts = append(parts, "  "+line)
			}
		}
	}
	for _, att := range a.composer.Attachments() {
		icon := "📎"
		if att.IsImage {
			icon = "🖼"
		}
		parts = append(parts, a.styles.EventPrefix.Render(icon+" "+att.Path)+" "+a.styles.Error.Render("✕"))
	}
	// Attachment input mode
	if a.attaching {
		parts = append(parts, a.styles.Accent.Render("📎 文件路径: ")+string(a.attachInput))
	} else {
		parts = append(parts, a.styles.Muted.Render("Ctrl+Enter:换行 Enter:发送 Ctrl+I:附加文件 Esc:取消"))
	}
	return strings.Join(parts, "\n")
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

type tickMsg time.Time
type quitConfirmTimeoutMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func quitConfirmTimeout() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return quitConfirmTimeoutMsg(t) })
}

func runAgentCmd(deps Deps, input string, images []string) tea.Cmd {
	return func() tea.Msg {
		if deps.OnSubmit != nil {
			if deps.Agent != nil && deps.Send != nil {
				text, err := deps.Agent.RunStreaming(context.Background(), input, func(chunk string) {
					deps.Send(streamChunkMsg{text: chunk})
				})
				if err != nil {
					return agentResponseMsg{err: err}
				}
				return agentResponseMsg{text: text}
			}
			text, err := deps.OnSubmit(context.Background(), input, images)
			return agentResponseMsg{text: text, err: err}
		}
		return agentResponseMsg{}
	}
}
