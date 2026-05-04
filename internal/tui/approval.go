package tui

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

type ApprovalResult int

const (
	ApprovalPending ApprovalResult = iota
	ApprovalAllow
	ApprovalDeny
	ApprovalAlways
	ApprovalCancel
)

type ApprovalRequest struct {
	ToolName string
	Detail   string
}

func (a ApprovalRequest) HandleKey(key string) ApprovalResult {
	switch key {
	case "y", "Y":
		return ApprovalAllow
	case "n", "N":
		return ApprovalDeny
	case "a", "A":
		return ApprovalAlways
	case "esc":
		return ApprovalCancel
	default:
		return ApprovalPending
	}
}

func (a ApprovalRequest) Render(th Theme) string {
	s := th.Styles()
	warn := s.Warning.Render("⚠ Approval Required")
	detail := s.BashPrefix.Render("$ ") + s.UserText.Render(a.Detail)
	allowBtn := lipgloss.NewStyle().
		Background(lipgloss.Color(th.Green)).
		Foreground(lipgloss.Color(th.BG)).
		Padding(0, 1).
		Render("Y 允许")
	denyBtn := lipgloss.NewStyle().
		Background(lipgloss.Color(th.Red)).
		Foreground(lipgloss.Color(th.BG)).
		Padding(0, 1).
		Render("N 拒绝")
	alwaysBtn := lipgloss.NewStyle().
		Background(lipgloss.Color(th.Border)).
		Foreground(lipgloss.Color(th.Text)).
		Padding(0, 1).
		Render("A 总是允许")
	escBtn := s.Muted.Render("Esc 取消")
	return fmt.Sprintf("%s\n%s\n%s %s %s %s", warn, detail, allowBtn, denyBtn, alwaysBtn, escBtn)
}
