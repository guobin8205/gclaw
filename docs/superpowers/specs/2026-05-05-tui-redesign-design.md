# gclaw TUI Redesign — Design Spec

Date: 2026-05-05

## Overview

Replace the current `bufio.Scanner` REPL with a full Bubble Tea TUI, adding interactive input (completion, history, multi-line editing), tool call visualization, streaming output, image/file support, and a log ring buffer.

Framework: **Bubble Tea + Lip Gloss + Bubbles** (Charm stack).

## Layout

```
┌─ Transcript (flex-grow, virtual scroll) ─────────────┐
│  Banner (version | model | mode)                      │
│  Message list (user / assistant / tool / event)       │
│  Scrollbar (1 col, right edge)                        │
├─ Divider ─────────────────────────────────────────────┤
│  Status Rule (left-aligned)                           │
│  Queue Preview (when busy)                            │
│  Completions Dropdown (floating, / triggered)         │
│  Multi-line Input Buffer                              │
│  Attachment Tags (image / file)                       │
│  Help Hint                                            │
└───────────────────────────────────────────────────────┘
```

## Components

### Transcript

Virtual scrolling: render only visible rows. Auto-scroll to bottom; pause when user scrolls up, resume on new user message.

Message types:

| Type | Prefix | Style |
|------|--------|-------|
| user | `❯` | green bold |
| assistant | indented | normal text, markdown rendered |
| tool_call | `⚙` / `$` / `⚡` / `★` | tree structure (see Tool Call Tree) |
| event | dashed border | system notification (weixin, cron, checkpoint) |

### Tool Call Tree

Nested indentation with `┊` vertical bars. Each level adds 16px indent + 1px left border.

```
⚡ delegate_task 配置微信公众号接入
┊ ★ skill: weixin_setup
┊ ┊ ⚙ grep weixin.*setup 0.4s
┊ ┊ $ grep -n "weixin" config.go 0.2s
┊ ┊ ⚙ read client.go 0.8s
┊ ⚙ write config.yaml ... ⠋
```

Icon convention:

- `⚙` — file/search tools (read, grep, glob, web_search, web_extract)
- `$` — Bash commands
- `⚡` — delegate_task (sub-agent)
- `★` — skill invocation
- `🖼` — image/vision
- `✓` — success
- `✕` — failure
- `⠋` — in progress (spinner animation)

Tool output is collapsed by default. Ctrl+O or click to expand. Shows line count when collapsed: `▸ 输出 (47行) · Ctrl+O 展开`.

### Composer

Multi-line input buffer:

- **Enter** — send message
- **Ctrl+Enter** — insert new line
- **Backspace** at line start — merge with previous line
- **Delete** — delete character after cursor
- **← → Home End** — cursor movement across lines
- **Ctrl+V** — paste text; if clipboard contains an image, save to `~/.gclaw/tmp/clipboard-{timestamp}.png` and attach
- **Ctrl+I** — open file picker to attach file
- **Tab** — apply current completion
- **↑ ↓** — cycle history (no completion) or cycle completions (completion active)
- **Ctrl+C** — interrupt agent (empty input) or clear input (has input)
- **Esc** — cancel input / close completion dropdown
- **Ctrl+L** — clear screen

### Completions

Triggered by `/` prefix. Debounced 60ms. Shows dropdown above input line:

```
/help — Show available commands     ← highlighted (active)
/logs — View recent logs [N]
/model — Switch model
/status /tools /skills ...
```

Tab applies active item. ↑↓ cycles. Esc closes. Max 16 items visible.

### Status Rule

Left-aligned, single line, 11px font:

```
● deepseek-v4-flash │ ctx: 1.2k/200k │ agents: 2 │ bg: 1 │ cron: active │ 5:36:20
```

Fields:

- Model name with colored dot (green=ready, yellow=busy, red=error)
- Context usage (input/total tokens)
- Active sub-agents count (hidden when 0)
- Background tasks count (hidden when 0)
- Cron status
- Session elapsed time

### Queue Preview

When agent is busy, queued messages show above input:

```
⏳ 2 queued  /model gpt-4o · /status
```

Up/Down arrows edit queued messages. Esc cancels edit.

### Image & File Attachments

Attachment flow:

1. **Ctrl+V with image clipboard** → save to `~/.gclaw/tmp/clipboard-{ts}.png` → show tag `🖼 clipboard-image.png ✕`
2. **Ctrl+I or drag file** → `.png/.jpg/.jpeg` attaches as image (sent to vision tool), `.txt/.log/.md` injects text into prompt
3. Tags shown below input line with ✕ to remove
4. On send, images are passed to vision tool automatically

In transcript, image attachments render as a card:

```
🖼 clipboard-image.png
[image preview: 800×600]
```

## Streaming & Thinking

- Assistant replies stream into transcript with blinking cursor `▌`
- DeepSeek ReasoningContent renders as collapsed block:

```
▸ 💭 思考过程 (2.1s · Ctrl+O 展开)
```

- **Ctrl+O** toggles fold/unfold for the current thinking block
- Thinking is collapsed by default; response body is always visible

## Tool Approval

Tools requiring approval show a popup in the composer area:

```
⚠ Approval Required
$ rm -rf /tmp/test-output
[Y 允许] [N 拒绝] [A 总是允许] [Esc 取消]
```

Keys: Y (allow once), N (deny), A (always allow this pattern), Esc (cancel). Left-aligned, replaces input area until resolved.

## Markdown Rendering

Assistant messages render markdown:
- Code blocks: language label + syntax highlighting (Go, Python, JSON, YAML, etc.)
- Tables: aligned columns
- Bold/italic/links
- Implemented via Lip Gloss styled text

## Async Event Notifications

System events rendered in transcript with dashed border:

```
--- 📬 weixin  张三: 今天的报告写完了吗？ ---
--- ⏰ cron  test-noagent completed ✓ ---
--- 💾 checkpoint  snapshot saved before WriteFile ---
```

Sub-agent count and background task count appear in status bar: `agents: 2 │ bg: 1` (hidden when 0).

## History

- Persistent file: `~/.gclaw/history`
- Format: one entry per line, timestamp + content
- Max entries: configurable, default 10000
- ↑↓ navigates history; prefix matching filters
- Draft preserved when cycling history

## Log System

- All `slog` output redirected to in-memory ring buffer (default 200 lines)
- `/logs [N]` — display last N lines in transcript
- `/logs -f` — follow mode (stream new logs), Ctrl+C exits
- Color-coded levels: INFO=blue, WARN=yellow, ERROR=red, DEBUG=gray

## Themes

Four built-in themes, configured via `tui.theme` or `/theme` command:

| Theme | Background | Use Case |
|-------|-----------|----------|
| `tokyo-night` (default) | `#1a1b26` | Dark terminals |
| `catppuccin-mocha` | `#1e1e2e` | Warm dark terminals |
| `light` | `#fafafa` | Light terminals |
| `terminal` | ANSI defaults | Follow terminal palette |

Colors per theme:

**Tokyo Night:**
- Text: `#c0caf5`
- Accent: `#7aa2f7`
- Green: `#9ece6a`
- Orange: `#ff9e64`
- Purple: `#bb9af7`
- Yellow: `#e0af68`
- Red: `#f7768e`
- Muted: `#565f89`
- Border: `#3b3d57`

**Catppuccin Mocha:**
- Text: `#cdd6f4`
- Accent: `#89b4fa`
- Green: `#a6e3a1`
- Orange: `#fab387`
- Purple: `#cba6f7`
- Yellow: `#f9e2af`
- Red: `#f38ba8`
- Muted: `#6c7086`
- Border: `#313244`

**Light:**
- Text: `#333333`
- Accent: `#0066cc`
- Green: `#008800`
- Orange: `#cc6600`
- Red: `#cc0000`
- Muted: `#888888`

**Terminal:**
- All colors use standard ANSI codes (0-15)

## Virtual Scrolling

- Maintain slice of all message IDs and their rendered heights
- On scroll, calculate visible window and only render those rows
- Top/bottom spacers fill remaining space
- Scrollbar thumb reflects viewport position
- Re-render on terminal resize

## Configuration

New config fields in `.gclaw/config.yaml`:

```yaml
tui:
  theme: tokyo-night        # tokyo-night | light | terminal
  history:
    max_entries: 10000
  log:
    buffer_size: 200
  completion:
    debounce_ms: 60
    max_visible: 16
```

## File Structure

New packages:

```
internal/tui/
├── app.go              # Bubble Tea Model + main tea.Model interface
├── transcript.go       # Virtual scroll transcript view
├── composer.go         # Multi-line input with completion
├── statusbar.go        # Left-aligned status rule
├── completion.go       # Slash command completion engine
├── history.go          # Persistent history manager
├── logbuffer.go        # Ring buffer for slog capture
├── theme.go            # Theme definitions and styling
├── markdown.go         # Markdown to Lip Gloss styled text
├── approval.go         # Tool approval popup
├── message.go          # Message types and rendering
└── keymap.go           # Key bindings
```

Integration point: `cmd/gclaw/main.go` replaces the `bufio.Scanner` loop with `tea.NewProgram(app).Run()`.
