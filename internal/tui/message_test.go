package tui

import "testing"

func TestToolCallIcon(t *testing.T) {
	tests := []struct{ name, want string }{
		{"ReadFile", "⚙"}, {"Bash", "$"}, {"delegate_task", "⚡"},
		{"skill_create", "★"}, {"vision", "🖼"}, {"unknown", "⚙"},
	}
	for _, tt := range tests {
		got := ToolCallIcon(tt.name)
		if got != tt.want {
			t.Errorf("ToolCallIcon(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestToolCallFormat(t *testing.T) {
	tc := ToolCall{Name: "ReadFile", Detail: "config.go", Duration: "0.4s", Status: ToolStatusDone}
	got := tc.Format()
	if got != "⚙ read config.go 0.4s" {
		t.Errorf("unexpected: %q", got)
	}
}

func TestToolCallFormatTree(t *testing.T) {
	parent := ToolCall{
		Name: "delegate_task", Detail: "configure weixin", Status: ToolStatusDone,
		Children: []ToolCall{
			{Name: "Grep", Detail: "weixin.*setup", Duration: "0.4s", Status: ToolStatusDone},
			{Name: "ReadFile", Detail: "client.go", Duration: "0.8s", Status: ToolStatusDone},
		},
	}
	lines := parent.FormatTree(0)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "⚡ delegate_task configure weixin" {
		t.Errorf("unexpected: %q", lines[0])
	}
}
