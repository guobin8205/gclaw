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
		Text: "#c0caf5", Accent: "#2ac3de", Green: "#9ece6a",
		Orange: "#ff9e64", Purple: "#bb9af7", Yellow: "#e0af68",
		Red: "#f7768e", Muted: "#565f89", Border: "#3b3d57",
	},
	"catppuccin-mocha": {
		Name: "catppuccin-mocha", BG: "#11111b",
		Text: "#cdd6f4", Accent: "#cba6f7", Green: "#a6e3a1",
		Orange: "#fab387", Purple: "#f5c2e7", Yellow: "#f9e2af",
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
		Skill:       fg(th.Purple),
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
		CodeComment: fg(th.Muted),
		CodeKeyword: fg(th.Purple),
	}
}
