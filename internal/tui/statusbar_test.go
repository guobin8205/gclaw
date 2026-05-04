package tui

import "testing"

func TestStatusBarRender(t *testing.T) {
	sb := NewStatusBar(LoadTheme("tokyo-night"))
	sb.SetModel("deepseek-v4-flash")
	sb.SetContextUsage(1200, 200000)
	sb.SetCronActive(true)
	if sb.Render(80) == "" {
		t.Error("expected non-empty render")
	}
}

func TestStatusBarHideZero(t *testing.T) {
	sb := NewStatusBar(LoadTheme("tokyo-night"))
	sb.SetModel("test")
	sb.SetAgentCount(0)
	sb.SetBackgroundTasks(0)
	if sb.Render(80) == "" {
		t.Error("expected non-empty render")
	}
}
