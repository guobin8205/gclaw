package tui

import (
	"sort"
	"strings"
)

type CompletionItem struct {
	Text        string
	Display     string
	Description string
}

type CompletionEngine struct {
	commands []CompletionItem
}

var defaultCommands = []CompletionItem{
	{"/clear", "/clear", "Clear conversation history"},
	{"/compact", "/compact", "Force context compaction"},
	{"/interrupt", "/interrupt", "Interrupt running agent"},
	{"/help", "/help", "Show available commands"},
	{"/version", "/version", "Show version"},
	{"/status", "/status", "Show system status"},
	{"/stats", "/stats", "Show usage statistics"},
	{"/config", "/config", "Show configuration"},
	{"/model", "/model", "Switch model"},
	{"/fallback", "/fallback", "Set fallback models"},
	{"/tools", "/tools", "List available tools"},
	{"/skills", "/skills", "List skills"},
	{"/memory", "/memory", "Memory management"},
	{"/sessions", "/sessions", "Session management"},
	{"/mcp", "/mcp", "MCP server status"},
	{"/cron", "/cron", "Cron job management"},
	{"/tasks", "/tasks", "Task list"},
	{"/weixin", "/weixin", "WeChat channel"},
	{"/gateway", "/gateway", "Gateway status"},
	{"/doctor", "/doctor", "Run diagnostics"},
	{"/debug", "/debug", "Toggle debug mode"},
	{"/dump", "/dump", "Dump internal state"},
	{"/backup", "/backup", "Create backup"},
	{"/logs", "/logs", "View recent logs [N]"},
	{"/theme", "/theme", "Switch TUI theme"},
	{"/autonomy", "/autonomy", "Autonomous mode"},
	{"/exit", "/exit", "Exit gclaw"},
}

func NewCompletionEngine(commands []CompletionItem) *CompletionEngine {
	return &CompletionEngine{commands: commands}
}

func DefaultCompletionEngine() *CompletionEngine {
	return NewCompletionEngine(defaultCommands)
}

func (e *CompletionEngine) Match(input string) []CompletionItem {
	if !strings.HasPrefix(input, "/") {
		return nil
	}
	prefix := strings.ToLower(input)
	var matches []CompletionItem
	for _, cmd := range e.commands {
		if strings.HasPrefix(strings.ToLower(cmd.Text), prefix) {
			matches = append(matches, cmd)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Text < matches[j].Text
	})
	return matches
}
