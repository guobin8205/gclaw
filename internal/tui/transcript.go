package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

type Transcript struct {
	msgs     []TranscriptMsg
	styles   Styles
	theme    Theme
	height   int
	width    int
	yOffset  int
	atBottom bool
}

func NewTranscript(styles Styles, theme Theme) *Transcript {
	return &Transcript{styles: styles, theme: theme, atBottom: true}
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

func (tr *Transcript) Messages() []TranscriptMsg { return tr.msgs }

func (tr *Transcript) Render() string {
	if tr.height <= 0 || tr.width <= 0 {
		return ""
	}
	var allLines []string
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
		return lipgloss.NewStyle().Width(tr.width).Render(strings.Join(visible, "\n"))
	}
	style := lipgloss.NewStyle().Width(tr.width - 2).Height(vis)
	content := style.Render(strings.Join(visible, "\n"))
	sb := tr.renderScrollbar(start, total, vis)
	return lipgloss.JoinHorizontal(lipgloss.Left, content, sb)
}

func (tr *Transcript) renderMessage(msg TranscriptMsg) []string {
	var lines []string
	switch msg.Kind {
	case MsgUser:
		lines = append(lines, tr.styles.UserPrefix.Render("❯ ")+tr.styles.UserText.Render(msg.Content))
		for _, img := range msg.Images {
			lines = append(lines, tr.styles.EventPrefix.Render("🖼 "+img))
		}
		lines = append(lines, "")
	case MsgAssistant:
		mdLines := RenderMarkdown(msg.Content, tr.theme)
		if len(mdLines) == 0 {
			mdLines = []string{msg.Content}
		}
		for _, l := range mdLines {
			lines = append(lines, tr.styles.Assistant.Render(l))
		}
		lines = append(lines, "")
	case MsgToolCall:
		if msg.Tool != nil {
			for _, tl := range msg.Tool.FormatTree(0) {
				lines = append(lines, "  "+tl)
			}
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
	// approximate: count rendered lines
	n := 0
	for _, msg := range tr.msgs {
		n += len(tr.renderMessage(msg))
	}
	return max(0, n-tr.height)
}
