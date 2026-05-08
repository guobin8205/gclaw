package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type Transcript struct {
	msgs          []TranscriptMsg
	styles        Styles
	theme         Theme
	height        int
	width         int
	yOffset       int
	atBottom      bool
	cursorVisible bool
	banner        string
}

func NewTranscript(styles Styles, theme Theme) *Transcript {
	return &Transcript{styles: styles, theme: theme, atBottom: true, cursorVisible: true}
}

func (tr *Transcript) Resize(w, h int) {
	tr.width = w
	tr.height = h
	if tr.atBottom {
		tr.scrollToBottom()
	}
}

func (tr *Transcript) Append(msg TranscriptMsg) {
	tr.msgs = append(tr.msgs, msg)
	if tr.atBottom {
		tr.scrollToBottom()
	}
}

func (tr *Transcript) UpdateLast(msg TranscriptMsg) {
	if len(tr.msgs) > 0 {
		tr.msgs[len(tr.msgs)-1] = msg
	}
}

func (tr *Transcript) ScrollUp(n int) {
	tr.yOffset -= n
	if tr.yOffset < 0 {
		tr.yOffset = 0
	}
	tr.atBottom = false
}

func (tr *Transcript) ScrollDown(n int) {
	tr.yOffset += n
	max := tr.maxScrollOffset()
	if tr.yOffset >= max {
		tr.yOffset = max
		tr.atBottom = true
	}
}

func (tr *Transcript) ScrollToBottom() {
	tr.atBottom = true
	tr.scrollToBottom()
}

func (tr *Transcript) ToggleCursor() {
	tr.cursorVisible = !tr.cursorVisible
}

func (tr *Transcript) ToggleFold() {
	for i := len(tr.msgs) - 1; i >= 0; i-- {
		msg := &tr.msgs[i]
		if msg.Kind == MsgAssistant && msg.Thinking != "" {
			msg.ThinkingOpen = !msg.ThinkingOpen
			return
		}
		if msg.Kind == MsgToolCall && msg.Tool != nil {
			if tc := tr.findLastCollapsed(msg.Tool); tc != nil {
				tc.Collapsed = !tc.Collapsed
				return
			}
		}
	}
}

func (tr *Transcript) findLastCollapsed(tc *ToolCall) *ToolCall {
	for i := len(tc.Children) - 1; i >= 0; i-- {
		if found := tr.findLastCollapsed(&tc.Children[i]); found != nil {
			return found
		}
	}
	if tc.Output != "" {
		return tc
	}
	return nil
}

func (tr *Transcript) Messages() []TranscriptMsg { return tr.msgs }
func (tr *Transcript) SetBanner(banner string)    { tr.banner = banner }

func (tr *Transcript) SetTheme(styles Styles, theme Theme) { tr.styles = styles; tr.theme = theme }

func (tr *Transcript) Render() string {
	if tr.height <= 0 || tr.width <= 0 {
		return ""
	}
	var allLines []string
	if tr.banner != "" {
		allLines = append(allLines, tr.styles.Accent.Render(tr.banner))
		allLines = append(allLines, "")
	}
	for _, msg := range tr.msgs {
		allLines = append(allLines, tr.renderMessage(msg)...)
	}
	total := len(allLines)
	vis := tr.height
	var start int
	var visible []string
	if total <= vis {
		visible = allLines
	} else {
		if tr.atBottom {
			start = total - vis
		} else {
			start = tr.yOffset
		}
		end := start + vis
		if end > total {
			end = total
		}
		visible = allLines[start:end]
	}
	if total <= vis {
		result := lipgloss.NewStyle().Width(tr.width).Render(strings.Join(visible, "\n"))
		// Width wrapping may add extra lines. When at bottom, keep the
		// last vis lines so newest content stays visible; otherwise keep top.
		resultLines := strings.Split(result, "\n")
		if len(resultLines) > vis {
			if tr.atBottom {
				result = strings.Join(resultLines[len(resultLines)-vis:], "\n")
			} else {
				result = strings.Join(resultLines[:vis], "\n")
			}
		}
		return result
	}
	style := lipgloss.NewStyle().Width(tr.width - 2).Height(vis)
	content := style.Render(strings.Join(visible, "\n"))
	sb := tr.renderScrollbar(start, total, vis)
	result := lipgloss.JoinHorizontal(lipgloss.Left, content, sb)
	// Width wrapping may add extra lines. When at bottom, keep the
	// last vis lines so newest content stays visible; otherwise keep top.
	resultLines := strings.Split(result, "\n")
	if len(resultLines) > vis {
		if tr.atBottom {
			result = strings.Join(resultLines[len(resultLines)-vis:], "\n")
		} else {
			result = strings.Join(resultLines[:vis], "\n")
		}
	}
	return result
}

func (tr *Transcript) renderMessage(msg TranscriptMsg) []string {
	var lines []string
	switch msg.Kind {
	case MsgUser:
		lines = append(lines, tr.styles.UserPrefix.Render("> ")+tr.styles.UserText.Render(msg.Content))
		for _, img := range msg.Images {
			lines = append(lines, tr.styles.EventPrefix.Render("🖼 "+img))
		}
		lines = append(lines, "")
	case MsgAssistant:
		// Thinking block (collapsed by default)
		if msg.Thinking != "" {
			if msg.ThinkingOpen {
				thinkLines := strings.Split(msg.Thinking, "\n")
				lines = append(lines, tr.styles.Muted.Render("▾ 💭 思考过程 · Ctrl+O 折叠"))
				for _, tl := range thinkLines {
					lines = append(lines, tr.styles.Muted.Render("  "+tl))
				}
			} else {
				lines = append(lines, tr.styles.Muted.Render("▸ 💭 思考过程 · Ctrl+O 展开"))
			}
		}
		mdLines := RenderMarkdown(msg.Content, tr.theme)
		if len(mdLines) == 0 {
			mdLines = []string{msg.Content}
		}
		for i, l := range mdLines {
			if msg.Streaming && tr.cursorVisible && i == len(mdLines)-1 {
				lines = append(lines, tr.styles.Assistant.Render(l)+"▌")
			} else {
				lines = append(lines, tr.styles.Assistant.Render(l))
			}
		}
		lines = append(lines, "")
	case MsgToolCall:
		if msg.Tool != nil {
			lines = append(lines, tr.renderToolCall(msg.Tool, 0)...)
		}
		lines = append(lines, "")
	case MsgEvent:
		lines = append(lines,
			tr.styles.Muted.Render("--- ")+
				tr.styles.EventPrefix.Render(msg.EventIcon+" "+msg.EventSrc)+
				tr.styles.Muted.Render("  "+msg.Content+" ---"),
		)
		lines = append(lines, "")
	}
	return lines
}

func (tr *Transcript) renderToolCall(tc *ToolCall, depth int) []string {
	var lines []string
	indent := strings.Repeat("┊ ", depth)
	if depth == 0 {
		lines = append(lines, "  "+tc.Format())
	} else {
		lines = append(lines, "  "+indent+tc.Format())
	}
	// Output folding
	if tc.Output != "" {
		outLines := strings.Split(tc.Output, "\n")
		if tc.Collapsed {
			lines = append(lines, "  "+indent+"  "+tr.styles.Muted.Render(fmt.Sprintf("▸ 输出 (%d行) · Ctrl+O 展开", len(outLines))))
		} else {
			for _, ol := range outLines {
				if ol != "" {
					lines = append(lines, "  "+indent+"  "+tr.styles.Muted.Render(ol))
				}
			}
		}
	}
	for _, child := range tc.Children {
		lines = append(lines, tr.renderToolCall(&child, depth+1)...)
	}
	return lines
}

func (tr *Transcript) renderScrollbar(offset, total, visible int) string {
	thumbSize := max(1, visible*visible/total)
	thumbPos := offset * visible / total
	var sb strings.Builder
	for i := 0; i < visible; i++ {
		if i >= thumbPos && i < thumbPos+thumbSize {
			sb.WriteString("█")
		} else {
			sb.WriteString("│")
		}
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(tr.theme.Muted)).Render(sb.String())
}

func (tr *Transcript) scrollToBottom() { tr.yOffset = tr.maxScrollOffset() }
func (tr *Transcript) maxScrollOffset() int {
	n := 0
	if tr.banner != "" {
		n += 2 // banner line + separator
	}
	for _, msg := range tr.msgs {
		n += len(tr.renderMessage(msg))
	}
	return max(0, n-tr.height)
}
