package tui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/openclaw/gclaw/internal/agent"
	"github.com/openclaw/gclaw/internal/config"
)

type Deps struct {
	Config   *config.Config
	Agent    *agent.Agent
	Theme    Theme
	LogBuf   *LogBuffer
	History  *History
	CompEng  *CompletionEngine
	OnSubmit func(ctx context.Context, input string, images []string) (string, error)
	OnSlash  func(cmd string)
	Send     func(msg tea.Msg) // tea.Program.Send wrapper
}

type App struct {
	deps       Deps
	theme      Theme
	styles     Styles
	transcript *Transcript
	composer   *Composer
	statusbar  *StatusBar
	approval   *ApprovalRequest
	width      int
	height     int
	busy       bool
	startTime  time.Time
}

func (a *App) SetSend(send func(msg tea.Msg)) {
	a.deps.Send = send
}

func NewApp(deps Deps) *App {
	th := deps.Theme
	st := th.Styles()
	return &App{
		deps:       deps,
		theme:      th,
		styles:     st,
		transcript: NewTranscript(st, th),
		composer:   NewComposer(),
		statusbar:  NewStatusBar(th),
		startTime:  time.Now(),
	}
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
		content := m.text
		if m.err != nil {
			content = "Error: " + m.err.Error()
		}
		a.transcript.Append(TranscriptMsg{Kind: MsgAssistant, Content: content})
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
			a.transcript.UpdateLast(msgs[len(msgs)-1])
		} else {
			a.transcript.Append(TranscriptMsg{Kind: MsgAssistant, Content: m.text})
		}
		return a, nil

	case tickMsg:
		a.statusbar.SetElapsed(time.Since(a.startTime).Truncate(time.Second).String())
		return a, tickCmd()
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
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, transcriptView, divider, statusView, composerView))
}

func (a *App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.approval != nil {
		result := a.approval.HandleKey(msg.Text)
		if result != ApprovalPending {
			a.approval = nil
		}
		return a, nil
	}

	text := msg.Text
	switch {
	case text == "ctrl+c":
		if a.composer.IsEmpty() {
			a.deps.Agent.Interrupt("user interrupt")
			return a, tea.Quit
		}
		a.composer.Clear()
		return a, nil
	case text == "ctrl+l":
		a.transcript = NewTranscript(a.styles, a.theme)
		a.transcript.Resize(a.width, a.height-6)
		return a, nil
	case text == "esc":
		a.composer.Clear()
		return a, nil
	case text == "enter":
		if a.busy {
			return a, nil
		}
		return a.submitInput()
	case text == "ctrl+enter":
		a.composer.InsertNewLine()
		return a, nil
	case text == "backspace":
		a.composer.Backspace()
		return a, nil
	case text == "delete":
		a.composer.Delete()
		return a, nil
	case text == "left":
		a.composer.MoveLeft()
		return a, nil
	case text == "right":
		a.composer.MoveRight()
		return a, nil
	case text == "home":
		a.composer.MoveHome()
		return a, nil
	case text == "end":
		a.composer.MoveEnd()
		return a, nil
	case text == "up":
		if a.deps.History != nil {
			if entry := a.deps.History.Older(); entry != "" {
				a.composer.SetInput(entry)
			}
		}
		return a, nil
	case text == "down":
		if a.deps.History != nil {
			a.composer.SetInput(a.deps.History.Newer())
		}
		return a, nil
	case text == "pgup":
		a.transcript.ScrollUp(a.transcript.height)
		return a, nil
	case text == "pgdown":
		a.transcript.ScrollDown(a.transcript.height)
		return a, nil
	default:
		if msg.Text != "" && len(msg.Text) > 0 {
			for _, r := range msg.Text {
				a.composer.InsertRune(r)
			}
		}
		return a, nil
	}
}

func (a *App) submitInput() (tea.Model, tea.Cmd) {
	text := a.composer.Text()
	if text == "" {
		return a, nil
	}
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

func (a *App) renderComposer() string {
	var parts []string
	if a.busy {
		parts = append(parts, a.styles.Warning.Render("⏳ agent busy..."))
	}
	text := a.composer.Text()
	if text == "" {
		parts = append(parts, a.styles.Prompt.Render("❯ ")+"_|")
	} else {
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			if i == 0 {
				parts = append(parts, a.styles.Prompt.Render("❯ ")+line)
			} else {
				parts = append(parts, "  "+line)
			}
		}
		parts[len(parts)-1] += "▌"
	}
	for _, att := range a.composer.Attachments() {
		icon := "📎"
		if att.IsImage {
			icon = "🖼"
		}
		parts = append(parts, a.styles.EventPrefix.Render(icon+" "+att.Path)+" "+a.styles.Error.Render("✕"))
	}
	parts = append(parts, a.styles.Muted.Render("Ctrl+Enter:换行 Enter:发送 Esc:取消"))
	return strings.Join(parts, "\n")
}

// Messages

type agentResponseMsg struct {
	text string
	err  error
}

type EventMsg struct {
	Icon   string
	Source string
	Content string
}

type streamChunkMsg struct {
	text string
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
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
