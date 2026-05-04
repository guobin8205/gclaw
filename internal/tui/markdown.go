package tui

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	reCodeBlock  = regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```")
	reBold       = regexp.MustCompile("\\*\\*(.+?)\\*\\*")
	reItalic     = regexp.MustCompile("\\*(.+?)\\*")
	reInlineCode = regexp.MustCompile("`([^`]+)`")
)

func RenderMarkdown(text string, th Theme) []string {
	styles := th.Styles()

	var codeBlocks []string
	text = reCodeBlock.ReplaceAllStringFunc(text, func(match string) string {
		sub := reCodeBlock.FindStringSubmatch(match)
		lang := sub[1]
		code := strings.TrimRight(sub[2], "\n")
		var block strings.Builder
		if lang != "" {
			block.WriteString(styles.Muted.Render(lang))
			block.WriteString("\n")
		}
		for _, line := range strings.Split(code, "\n") {
			block.WriteString(styles.Muted.Render(line))
			block.WriteString("\n")
		}
		idx := len(codeBlocks)
		codeBlocks = append(codeBlocks, block.String())
		return "\x00CODEBLOCK_" + strings.Repeat(" ", idx) + "\x00"
	})

	var result []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "\x00CODEBLOCK_") {
			trimmed := strings.TrimPrefix(line, "\x00CODEBLOCK_")
			trimmed = strings.TrimRight(trimmed, "\x00")
			idx := 0
			for _, ch := range trimmed {
				if ch == ' ' {
					idx++
				}
			}
			if idx < len(codeBlocks) {
				for _, bline := range strings.Split(codeBlocks[idx], "\n") {
					if bline != "" {
						result = append(result, bline)
					}
				}
			}
			continue
		}
		styled := applyInlineStyles(line, styles)
		result = append(result, styled)
	}
	return result
}

func applyInlineStyles(text string, s Styles) string {
	text = reBold.ReplaceAllStringFunc(text, func(match string) string {
		inner := reBold.FindStringSubmatch(match)[1]
		return lipgloss.NewStyle().Bold(true).Render(inner)
	})
	text = reInlineCode.ReplaceAllStringFunc(text, func(match string) string {
		inner := reInlineCode.FindStringSubmatch(match)[1]
		return s.Muted.Render(inner)
	})
	text = reItalic.ReplaceAllStringFunc(text, func(match string) string {
		inner := reItalic.FindStringSubmatch(match)[1]
		return lipgloss.NewStyle().Italic(true).Render(inner)
	})
	return text
}
