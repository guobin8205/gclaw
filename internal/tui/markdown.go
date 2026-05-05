package tui

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	reCodeBlock  = regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```")
	reBold       = regexp.MustCompile("\\*\\*(.+?)\\*\\*")
	reItalic     = regexp.MustCompile("\\*(.+?)\\*")
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reHeading    = regexp.MustCompile("^(#{1,6})\\s+(.+)$")
	reLink       = regexp.MustCompile("\\[(.+?)\\]\\((.+?)\\)")
	reTableLine  = regexp.MustCompile("^\\|(.+)\\|$")
	reTableSep   = regexp.MustCompile("^\\|[-:| ]+\\|$")
	// Keyword highlighting for code blocks
	reKeyword = regexp.MustCompile(`\b(func|return|if|else|for|range|var|const|type|struct|interface|map|chan|go|defer|select|switch|case|default|break|continue|package|import|nil|true|false|error|string|int|bool|byte|make|append|len|cap|fmt|err|ctx)\b`)
	reComment = regexp.MustCompile(`(//.*$|/\*.*?\*/)`)
	reString  = regexp.MustCompile(`"([^"\\]|\\.)*"`)
)

func RenderMarkdown(text string, th Theme) []string {
	styles := th.Styles()

	// Extract and replace code blocks first
	var codeBlocks []string
	text = reCodeBlock.ReplaceAllStringFunc(text, func(match string) string {
		sub := reCodeBlock.FindStringSubmatch(match)
		lang := sub[1]
		code := strings.TrimRight(sub[2], "\n")
		var block strings.Builder
		// Language label with decorative border
		if lang != "" {
			block.WriteString(styles.Accent.Render(fmt.Sprintf("┌ %s ", lang)))
			block.WriteString("\n")
		}
		for _, line := range strings.Split(code, "\n") {
			highlighted := highlightCode(line, th)
			block.WriteString(styles.Muted.Render("│ ") + highlighted)
			block.WriteString("\n")
		}
		if lang != "" {
			block.WriteString(styles.Accent.Render("└"))
			block.WriteString("\n")
		}
		idx := len(codeBlocks)
		codeBlocks = append(codeBlocks, block.String())
		return "\x00CODEBLOCK_" + strings.Repeat(" ", idx) + "\x00"
	})

	var result []string
	var tableRows [][]string
	var inTable bool

	for _, line := range strings.Split(text, "\n") {
		// Code block placeholder
		if strings.HasPrefix(line, "\x00CODEBLOCK_") {
			if inTable {
				result = append(result, renderTable(tableRows, th)...)
				tableRows = nil
				inTable = false
			}
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

		// Table row
		if reTableLine.MatchString(line) {
			if reTableSep.MatchString(line) {
				continue // skip separator rows
			}
			cells := parseTableRow(line)
			tableRows = append(tableRows, cells)
			inTable = true
			continue
		}

		// Flush table if we exit table mode
		if inTable {
			result = append(result, renderTable(tableRows, th)...)
			tableRows = nil
			inTable = false
		}

		// Heading
		if m := reHeading.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			headingText := applyInlineStyles(m[2], styles)
			switch level {
			case 1:
				result = append(result, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Accent)).Render(headingText))
			case 2:
				result = append(result, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Text)).Render(headingText))
			default:
				result = append(result, lipgloss.NewStyle().Bold(true).Render(headingText))
			}
			continue
		}

		// Link
		line = reLink.ReplaceAllStringFunc(line, func(match string) string {
			parts := reLink.FindStringSubmatch(match)
			return styles.Accent.Render(parts[1]) + styles.Muted.Render("("+parts[2]+")")
		})

		styled := applyInlineStyles(line, styles)
		result = append(result, styled)
	}

	// Flush remaining table
	if inTable {
		result = append(result, renderTable(tableRows, th)...)
	}

	return result
}

func highlightCode(line string, th Theme) string {
	// Simple keyword highlighting: keywords → accent color, strings → green, comments → muted
	// Process in order: strings first (to avoid highlighting keywords inside strings)
	result := reString.ReplaceAllStringFunc(line, func(match string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(th.Green)).Render(match)
	})
	result = reComment.ReplaceAllStringFunc(result, func(match string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(th.Muted)).Render(match)
	})
	result = reKeyword.ReplaceAllStringFunc(result, func(match string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(th.Purple)).Render(match)
	})
	return result
}

func parseTableRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func renderTable(rows [][]string, th Theme) []string {
	if len(rows) == 0 {
		return nil
	}
	// Calculate column widths
	colCount := 0
	for _, row := range rows {
		if len(row) > colCount {
			colCount = len(row)
		}
	}
	widths := make([]int, colCount)
	for _, row := range rows {
		for i, cell := range row {
			// Strip ANSI for width calculation
			stripped := stripANSI(cell)
			if len(stripped) > widths[i] {
				widths[i] = len(stripped)
			}
		}
	}

	var lines []string
	for _, row := range rows {
		var cells []string
		for i := 0; i < colCount; i++ {
			var cell string
			if i < len(row) {
				cell = row[i]
			}
			stripped := stripANSI(cell)
			pad := widths[i] - len(stripped)
			if pad < 0 {
				pad = 0
			}
			cells = append(cells, cell+strings.Repeat(" ", pad))
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color(th.Muted)).Render(
			"│ "+strings.Join(cells, " │ ")+" │",
		))
	}
	return lines
}

func stripANSI(s string) string {
	// Simple ANSI escape sequence remover
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			// Skip escape sequence
			i++
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) && !((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
					i++
				}
				if i < len(s) {
					i++
				}
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
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
