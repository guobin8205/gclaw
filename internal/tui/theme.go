package tui

import (
	"charm.land/lipgloss/v2"
)

type Theme struct {
	Name   string
	BG     string
	Text   string
	Muted  string
	Dim    string
	Border string
	Accent string
	Green  string
	Yellow string
	Red    string
	Orange string
	Purple string
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
	Dim         lipgloss.Style
	Accent      lipgloss.Style
	Banner      lipgloss.Style
	StatusBar   lipgloss.Style
	Divider     lipgloss.Style
	Completion  lipgloss.Style
	CompActive  lipgloss.Style
	Error       lipgloss.Style
	Warning     lipgloss.Style
	Prompt      lipgloss.Style
	Background  lipgloss.Style
	AllowBtn    lipgloss.Style
	DenyBtn     lipgloss.Style
	AlwaysBtn   lipgloss.Style
	Heading1    lipgloss.Style
	Heading2    lipgloss.Style
	Heading     lipgloss.Style
	Bold        lipgloss.Style
	Italic      lipgloss.Style
	Table       lipgloss.Style
	CodeString  lipgloss.Style
	CodeComment lipgloss.Style
	CodeKeyword lipgloss.Style
}

var Themes = map[string]Theme{
	"tokyo-night": {
		Name: "tokyo-night", BG: "#1a1b26",
		Text: "#c0caf5", Muted: "#565f89", Dim: "#414868",
		Border: "#3b4261", Accent: "#2ac3de",
		Green: "#9ece6a", Yellow: "#e0af68", Red: "#f7768e",
		Orange: "#ff9e64", Purple: "#bb9af7",
	},
	"catppuccin-mocha": {
		Name: "catppuccin-mocha", BG: "#11111b",
		Text: "#cdd6f4", Muted: "#6c7086", Dim: "#585b70",
		Border: "#313244", Accent: "#cba6f7",
		Green: "#a6e3a1", Yellow: "#f9e2af", Red: "#f38ba8",
		Orange: "#fab387", Purple: "#f5c2e7",
	},
	"light": {
		Name: "light", BG: "#fafafa",
		Text: "#333333", Muted: "#888888", Dim: "#aaaaaa",
		Border: "#dddddd", Accent: "#0066cc",
		Green: "#008800", Yellow: "#cc9900", Red: "#cc0000",
		Orange: "#cc6600", Purple: "#6600cc",
	},
	"terminal": {
		Name: "terminal", BG: "",
		Text: "", Muted: "", Dim: "",
		Border: "", Accent: "",
		Green: "", Yellow: "", Red: "",
		Orange: "", Purple: "",
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
		Skill:       fg(th.Purple),
		EventPrefix: fg(th.Accent),
		Muted:       fg(th.Muted),
		Dim:         fg(th.Dim),
		Accent:      fg(th.Accent),
		Banner:      fgbg(th.Accent, th.Dim),
		StatusBar:   fg(th.Muted),
		Divider:     fg(th.Border),
		Completion:  fg(th.Text),
		CompActive:  fgbg(th.Purple, th.Border),
		Error:       fg(th.Red),
		Warning:     fg(th.Yellow),
		Prompt:      lipgloss.NewStyle().Foreground(lipgloss.Color(th.Orange)).Bold(true),
		Background:  fgbg(th.Text, th.BG),
		AllowBtn: lipgloss.NewStyle().
			Background(lipgloss.Color(th.Green)).
			Foreground(lipgloss.Color(th.BG)).
			Padding(0, 1),
		DenyBtn: lipgloss.NewStyle().
			Background(lipgloss.Color(th.Red)).
			Foreground(lipgloss.Color(th.BG)).
			Padding(0, 1),
		AlwaysBtn: lipgloss.NewStyle().
			Background(lipgloss.Color(th.Border)).
			Foreground(lipgloss.Color(th.Text)).
			Padding(0, 1),
		Heading1:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Accent)),
		Heading2:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Text)),
		Heading:     lipgloss.NewStyle().Bold(true),
		Bold:        lipgloss.NewStyle().Bold(true),
		Italic:      lipgloss.NewStyle().Italic(true),
		Table:       fg(th.Muted),
		CodeString:  fg(th.Green),
		CodeComment: fg(th.Dim),
		CodeKeyword: fg(th.Purple),
	}
}
