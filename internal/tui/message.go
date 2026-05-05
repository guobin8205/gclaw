package tui

import "strings"

type MsgKind int

const (
	MsgUser     MsgKind = iota
	MsgAssistant
	MsgToolCall
	MsgEvent
)

type ToolStatus int

const (
	ToolStatusRunning ToolStatus = iota
	ToolStatusDone
	ToolStatusError
)

type ToolCall struct {
	Name      string
	Detail    string
	Duration  string
	Status    ToolStatus
	Output    string
	Collapsed bool
	Children  []ToolCall
}

type TranscriptMsg struct {
	Kind         MsgKind
	Content      string
	Tool         *ToolCall
	EventIcon    string
	EventSrc     string
	Thinking     string
	ThinkingOpen bool
	Images       []string
	Streaming    bool // true when assistant message is receiving stream chunks
}

func ToolCallIcon(name string) string {
	switch name {
	case "Bash":
		return "$"
	case "delegate_task":
		return "⚡"
	case "skill_create", "skill_delete", "skill_list":
		return "★"
	case "vision":
		return "🖼"
	case "ReadFile", "WriteFile", "Grep", "Glob",
		"web_search", "web_extract", "Patch":
		return "⚙"
	default:
		return "⚙"
	}
}

func toolCallShortName(name string) string {
	switch name {
	case "ReadFile":
		return "read"
	case "WriteFile":
		return "write"
	case "web_search":
		return "search"
	case "web_extract":
		return "extract"
	default:
		return strings.ToLower(name)
	}
}

func (tc ToolCall) statusIcon() string {
	switch tc.Status {
	case ToolStatusRunning:
		return "⠋"
	case ToolStatusError:
		return "✕"
	case ToolStatusDone:
		return "✓"
	default:
		return ""
	}
}

func (tc ToolCall) Format() string {
	icon := ToolCallIcon(tc.Name)
	short := toolCallShortName(tc.Name)
	parts := []string{icon + " " + short}
	if tc.Detail != "" {
		parts = append(parts, tc.Detail)
	}
	if tc.Duration != "" {
		parts = append(parts, tc.Duration)
	}
	if si := tc.statusIcon(); si != "" {
		parts = append(parts, si)
	}
	return strings.Join(parts, " ")
}

func (tc ToolCall) FormatTree(depth int) []string {
	var lines []string
	indent := strings.Repeat("┊ ", depth)
	if depth == 0 {
		lines = append(lines, tc.Format())
	} else {
		lines = append(lines, indent+tc.Format())
	}
	for _, child := range tc.Children {
		lines = append(lines, child.FormatTree(depth+1)...)
	}
	return lines
}
