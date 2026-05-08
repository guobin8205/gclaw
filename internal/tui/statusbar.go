package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type StatusBar struct {
	theme    Theme
	styles   Styles
	model    string
	state    string
	ctxInput int
	ctxTotal int
	agents   int
	bgTasks  int
	cronOn   bool
	elapsed  string
}

func NewStatusBar(th Theme) *StatusBar {
	return &StatusBar{theme: th, styles: th.Styles(), state: "ready"}
}

func (sb *StatusBar) SetModel(m string)      { sb.model = m }
func (sb *StatusBar) SetState(s string)       { sb.state = s }
func (sb *StatusBar) SetContextUsage(i, t int) { sb.ctxInput = i; sb.ctxTotal = t }
func (sb *StatusBar) SetAgentCount(n int)     { sb.agents = n }
func (sb *StatusBar) SetBackgroundTasks(n int) { sb.bgTasks = n }
func (sb *StatusBar) SetCronActive(on bool)   { sb.cronOn = on }
func (sb *StatusBar) SetElapsed(s string)     { sb.elapsed = s }

func (sb *StatusBar) SetTheme(th Theme) { sb.theme = th; sb.styles = th.Styles() }

func (sb *StatusBar) Render(width int) string {
	var parts []string

	var dotStyle lipgloss.Style
	switch {
	case sb.state == "busy":
		dotStyle = sb.styles.Warning
	case sb.state == "error":
		dotStyle = sb.styles.Error
	default:
		dotStyle = sb.styles.ToolName
	}
	dot := dotStyle.Render("●")
	modelName := sb.styles.Accent.Render(sb.model)
	parts = append(parts, dot+" "+modelName)

	if sb.ctxTotal > 0 {
		parts = append(parts, fmt.Sprintf("ctx: %s/%s", formatTokens(sb.ctxInput), formatTokens(sb.ctxTotal)))
	}
	if sb.agents > 0 {
		parts = append(parts, fmt.Sprintf("agents: %d", sb.agents))
	}
	if sb.bgTasks > 0 {
		parts = append(parts, fmt.Sprintf("bg: %d", sb.bgTasks))
	}
	if sb.cronOn {
		parts = append(parts, "cron: active")
	}
	if sb.elapsed != "" {
		parts = append(parts, sb.elapsed)
	}

	return sb.styles.Muted.Render(strings.Join(parts, " │ "))
}

func formatTokens(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
