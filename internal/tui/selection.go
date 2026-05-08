package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type selPoint struct {
	Col int
	Row int
}

type Selection struct {
	anchor *selPoint
	focus  *selPoint
	active bool
	done   bool
}

func newSelection() *Selection { return &Selection{} }

func (s *Selection) Start(col, row int) {
	s.anchor = &selPoint{Col: col, Row: row}
	s.focus = &selPoint{Col: col, Row: row}
	s.active = true
	s.done = false
}

func (s *Selection) Update(col, row int) {
	if s.active {
		s.focus = &selPoint{Col: col, Row: row}
	}
}

func (s *Selection) Finish() {
	s.active = false
	s.done = true
}

func (s *Selection) Clear() {
	s.anchor = nil
	s.focus = nil
	s.active = false
	s.done = false
}

func (s *Selection) HasSelection() bool {
	return (s.active || s.done) && s.anchor != nil && s.focus != nil
}

func (s *Selection) IsDragging() bool { return s.active }

func (s *Selection) Bounds() (start, end selPoint, ok bool) {
	if s.anchor == nil || s.focus == nil {
		return
	}
	ok = true
	a, b := *s.anchor, *s.focus
	if a.Row < b.Row || (a.Row == b.Row && a.Col <= b.Col) {
		start, end = a, b
	} else {
		start, end = b, a
	}
	return
}

// runeWidth returns the display width of a rune (1 for narrow, 2 for wide).
// Covers CJK, Hangul, Kana, fullwidth forms, and East Asian Ambiguous ranges
// that render as double-width on CJK terminals.
func runeWidth(r rune) int {
	switch {
	// Hangul Jamo
	case r >= 0x1100 && r <= 0x115F:
	// CJK Radicals through CJK Compatibility (includes Unified Ideographs 4E00-9FFF)
	case r >= 0x2E80 && r <= 0xA4CF:
	// Hangul Syllables
	case r >= 0xAC00 && r <= 0xD7A3:
	// CJK Compatibility Ideographs
	case r >= 0xF900 && r <= 0xFAFF:
	// CJK Compatibility Forms
	case r >= 0xFE30 && r <= 0xFE6F:
	// Fullwidth Forms (fullwidth digits, letters, punctuation)
	case r >= 0xFF01 && r <= 0xFF60:
	// Fullwidth Signs
	case r >= 0xFFE0 && r <= 0xFFE6:
	// Box Drawing (U+2500-U+257F)
	case r >= 0x2500 && r <= 0x257F:
	// Block Elements (U+2580-U+259F)
	case r >= 0x2580 && r <= 0x259F:
	// Geometric Shapes (U+25A0-U+25FF)
	case r >= 0x25A0 && r <= 0x25FF:
	// Miscellaneous Symbols (U+2600-U+26FF)
	case r >= 0x2600 && r <= 0x26FF:
	// Dingbats (U+2700-U+27BF) — includes ❯ (U+276F)
	case r >= 0x2700 && r <= 0x27BF:
	// Arrows (U+2190-U+21FF)
	case r >= 0x2190 && r <= 0x21FF:
	// Mathematical Operators ambiguous (U+2200-U+22FF)
	case r >= 0x2200 && r <= 0x22FF:
	// CJK Unified Ideographs Extension B-I and other SMP planes
	case r >= 0x20000 && r <= 0x3FFFD:
	// Zero-width characters
	case r >= 0x0300 && r <= 0x036F: // Combining Diacritical Marks
		return 0
	case r >= 0xFE00 && r <= 0xFE0F: // Variation Selectors
		return 0
	case r == 0x200B || r == 0x200C || r == 0x200D: // ZW space/joiner
		return 0
	case r == 0xFEFF: // BOM
		return 0
	default:
		return 1
	}
	return 2
}

// applyInverseRange wraps the visual column range [startCol, endCol] with
// ANSI inverse codes, correctly handling existing ANSI escape sequences.
// After each ANSI sequence within the selected range, inverse is re-applied
// to prevent color resets from breaking the highlight.
func applyInverseRange(line string, startCol, endCol int) string {
	visCol := 0
	var sb strings.Builder
	inverted := false
	i := 0
	for i < len(line) {
		// Enter inverse at start of selection
		if !inverted && visCol >= startCol {
			sb.WriteString("\x1b[7m")
			inverted = true
		}
		// Exit inverse past end of selection
		if inverted && visCol > endCol {
			sb.WriteString("\x1b[27m")
			inverted = false
		}
		// Handle ANSI escape sequences (they take no visual space)
		if line[i] == '\x1b' {
			j := i + 1
			if j < len(line) && line[j] == '[' {
				j++
				for j < len(line) && !((line[j] >= 'A' && line[j] <= 'Z') || (line[j] >= 'a' && line[j] <= 'z')) {
					j++
				}
				if j < len(line) {
					j++
				}
			}
			sb.WriteString(line[i:j])
			// Re-apply inverse after any ANSI sequence within selection,
			// because sequences like \x1b[0m (reset) clear the inverse attribute
			if inverted && visCol >= startCol {
				sb.WriteString("\x1b[7m")
			}
			i = j
		} else {
			r, size := utf8.DecodeRuneInString(line[i:])
			sb.WriteRune(r)
			visCol += runeWidth(r)
			i += size
		}
	}
	if inverted {
		sb.WriteString("\x1b[27m")
	}
	return sb.String()
}

// stringVisualWidth returns the total display width of a string using runeWidth.
func stringVisualWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// visualColToRuneIdx maps a visual column to a rune index in plain text.
func visualColToRuneIdx(s string, col int) int {
	visCol := 0
	for idx, r := range s {
		if visCol >= col {
			return idx
		}
		visCol += runeWidth(r)
	}
	return len(s)
}

// stripAnsi removes all ANSI escape sequences, returning only visible text.
func stripAnsi(s string) string {
	var sb strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
					j++
				}
				if j < len(s) {
					j++
				}
			}
			i = j
		} else {
			r, size := utf8.DecodeRuneInString(s[i:])
			sb.WriteRune(r)
			i += size
		}
	}
	return sb.String()
}

// SelectedText extracts plain text from content between start and end visual
// coordinates. ANSI escape sequences are stripped before slicing.
func SelectedText(content string, start, end selPoint) string {
	lines := strings.Split(content, "\n")
	if start.Col > end.Col && start.Row == end.Row {
		start, end = end, start
	}
	if start.Row > end.Row {
		start, end = end, start
	}
	if start.Row >= len(lines) {
		return ""
	}
	if end.Row >= len(lines) {
		end.Row = len(lines) - 1
	}
	if start.Row == end.Row {
		plain := stripAnsi(lines[start.Row])
		a := visualColToRuneIdx(plain, start.Col)
		b := visualColToRuneIdx(plain, end.Col+1)
		if a > len(plain) {
			a = len(plain)
		}
		if b > len(plain) {
			b = len(plain)
		}
		if b <= a {
			return ""
		}
		return plain[a:b]
	}
	var parts []string
	// First line: from start.Col to end, trim trailing spaces
	plain := stripAnsi(lines[start.Row])
	a := visualColToRuneIdx(plain, start.Col)
	if a < len(plain) {
		parts = append(parts, strings.TrimRight(plain[a:], " \t"))
	}
	// Middle lines: strip ANSI, trim trailing spaces
	for r := start.Row + 1; r < end.Row; r++ {
		parts = append(parts, strings.TrimRight(stripAnsi(lines[r]), " \t"))
	}
	// Last line: from 0 to end.Col, trim trailing spaces
	plain = stripAnsi(lines[end.Row])
	b := visualColToRuneIdx(plain, end.Col+1)
	if b > len(plain) {
		b = len(plain)
	}
	if b > 0 {
		parts = append(parts, strings.TrimRight(plain[:b], " \t"))
	}
	return strings.Join(parts, "\n")
}

// applySelectionToContent applies inverse-video highlighting to the joined
// view content within the selection range.
func applySelectionToContent(content string, start, end selPoint) string {
	if start.Col > end.Col && start.Row == end.Row {
		start, end = end, start
	}
	if start.Row > end.Row {
		start, end = end, start
	}
	lines := strings.Split(content, "\n")
	if start.Row >= len(lines) {
		return content
	}
	if end.Row >= len(lines) {
		end.Row = len(lines) - 1
	}
	if start.Row == end.Row {
		lines[start.Row] = applyInverseRange(lines[start.Row], start.Col, end.Col)
		return strings.Join(lines, "\n")
	}
	// First line: highlight from start.Col to end of line
	lineStart := stripAnsi(lines[start.Row])
	endVis := stringVisualWidth(lineStart) - 1
	if endVis < start.Col {
		endVis = start.Col
	}
	lines[start.Row] = applyInverseRange(lines[start.Row], start.Col, endVis)
	// Middle lines: full highlight
	for r := start.Row + 1; r < end.Row; r++ {
		plain := stripAnsi(lines[r])
		w := stringVisualWidth(plain) - 1
		if w < 0 {
			w = 0
		}
		lines[r] = applyInverseRange(lines[r], 0, w)
	}

	
	// Last line: highlight from 0 to end.Col
	lines[end.Row] = applyInverseRange(lines[end.Row], 0, end.Col)
	return strings.Join(lines, "\n")
}

	// themeBgAnsi converts a hex color (e.g. "#1a1b26") to raw ANSI background
	// escape sequences: open (\x1b[48;2;R;G;Bm) and reset (\x1b[0m).
	func themeBgAnsi(hex string) (open, reset string) {
		hex = strings.TrimPrefix(hex, "#")
		var r, g, b int
		fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
		return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b), "\x1b[0m"
	}
