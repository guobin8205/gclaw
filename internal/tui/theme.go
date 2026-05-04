package tui

import (
	"charm.land/lipgloss/v2"
)

type Theme struct {
	Name   string
	BG     string
	Text   string
	Accent string
	Green  string
	Orange string
	Purple string
	Yellow string
	Red    string
	Muted  string
	Border string
}

type Styles struct {
	UserText    lipgloss.Style
	UserPrefix  lipgloss.Style
	Assistant   lipgloss.Style
	ToolName    lipgloss.Style
	BashPrefix  lipgloss.Style
	Delegate    lipgloss.Style
	Skill       lipgloss.Style
	EventPrefix lipgloss.Style
	Muted       lipgloss.Style
	Accent      lipgloss.Style
	StatusBar   lipgloss.Style
	Divider     lipgloss.Style
	Completion  lipgloss.Style
	CompActive  lipgloss.Style
	Error       lipgloss.Style
	Warning     lipgloss.Style
	Prompt      lipgloss.Style
}

var Themes = map[string]Theme{
	"tokyo-night": {
		Name: "tokyo-night", BG: "#1a1b26",
		Text: "#c0caf5", Accent: "#7aa2f7", Green: "#9ece6a",
		Orange: "#ff9e64", Purple: "#bb9af7", Yellow: "#e0af68",
		Red: "#f7768e", Muted: "#565f89", Border: "#3b3d57",
	},
	"catppuccin-mocha": {
		Name: "catppuccin-mocha", BG: "#1e1e2e",
		Text: "#cdd6f4", Accent: "#89b4fa", Green: "#a6e3a1",
		Orange: "#fab387", Purple: "#cba6f7", Yellow: "#f9e2af",
		Red: "#f38ba8", Muted: "#6c7086", Border: "#313244",
	},
	"light": {
		Name: "light", BG: "#fafafa",
		Text: "#333333", Accent: "#0066cc", Green: "#008800",
		Orange: "#cc6600", Purple: "#6600cc", Yellow: "#cc9900",
		Red: "#cc0000", Muted: "#888888", Border: "#dddddd",
	},
	"terminal": {
		Name: "terminal", BG: "",
		Text: "", Accent: "", Green: "",
		Orange: "", Purple: "", Yellow: "",
		Red: "", Muted: "", Border: "",
	},
}

func LoadTheme(name string) Theme {
	th, ok := Themes[name]
	if !ok {
		return Themes["tokyo-night"]
	}
	return th
}

func (th Theme) Styles() Styles {
	fg := func(hex string) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
	}
	fgbg := func(fgHex, bgHex string) lipgloss.Style {
		s := lipgloss.NewStyle()
		if fgHex != "" {
			s = s.Foreground(lipgloss.Color(fgHex))
		}
		if bgHex != "" {
			s = s.Background(lipgloss.Color(bgHex))
		}
		return s
	}
	return Styles{
		UserText:    fg(th.Text),
		UserPrefix:  lipgloss.NewStyle().Foreground(lipgloss.Color(th.Green)).Bold(true),
		Assistant:   lipgloss.NewStyle().Foreground(lipgloss.Color(th.Text)).MarginLeft(2),
		ToolName:    fg(th.Green),
		BashPrefix:  fg(th.Orange),
		Delegate:    fg(th.Purple),
		Skill:       fg(th.Yellow),
		EventPrefix: fg(th.Accent),
		Muted:       fg(th.Muted),
		Accent:      fg(th.Accent),
		StatusBar:   fg(th.Muted),
		Divider:     fg(th.Border),
		Completion:  fg(th.Text),
		CompActive:  fgbg(th.Purple, th.Border),
		Error:       fg(th.Red),
		Warning:     fg(th.Yellow),
		Prompt:      lipgloss.NewStyle().Foreground(lipgloss.Color(th.Orange)).Bold(true),
	}
}
