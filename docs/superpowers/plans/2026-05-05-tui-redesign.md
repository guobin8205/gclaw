# gclaw TUI Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `bufio.Scanner` REPL with a Bubble Tea TUI providing interactive input, tool call visualization, streaming, images, and log viewing.

**Architecture:** Bubble Tea (`tea.Model`) drives the entire REPL. The app model holds transcript messages, composer state, and a reference to `agent.Agent`. Agent runs in a goroutine; results are delivered via `tea.Cmd`. Tool calls are intercepted via a callback that sends messages back to the TUI for rendering and approval.

**Tech Stack:** Go 1.26, Bubble Tea v2, Lip Gloss v2, Bubbles v2, existing gclaw packages.

---

## File Structure

```
internal/tui/
├── theme.go          # Theme color definitions + Lip Gloss styles
├── keymap.go         # Key binding constants
├── message.go        # TUI message types (user, assistant, tool, event)
├── logbuffer.go      # Ring buffer capturing slog output
├── history.go        # Persistent command history (~/.gclaw/history)
├── completion.go     # Slash command completion engine
├── markdown.go       # Markdown → Lip Gloss styled text converter
├── statusbar.go      # Status rule component (left-aligned)
├── composer.go       # Multi-line input with completion, attachments
├── transcript.go     # Virtual scroll transcript view
├── approval.go       # Tool approval popup component
├── app.go            # Bubble Tea top-level model + command routing
internal/config/
├── config.go         # Modified: add TUIConfig struct
cmd/gclaw/
├── main.go           # Modified: replace bufio.Scanner loop with tea.Program
```

---

### Task 1: Add Dependencies and TUI Config

**Files:**
- Modify: `go.mod`
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go` (existing)

- [ ] **Step 1: Add Bubble Tea dependencies**

Run:
```bash
cd E:\repos\ts\gclaw
go get github.com/charmbracelet/bubbletea/v2@latest
go get github.com/charmbracelet/lipgloss/v2@latest
go get github.com/charmbracelet/bubbles/v2@latest
```

- [ ] **Step 2: Add TUIConfig to config.go**

Add after the `CheckpointConfig` struct:

```go
type TUIConfig struct {
    Theme     string `yaml:"theme"`      // tokyo-night | catppuccin-mocha | light | terminal
    History   TUIHistoryConfig `yaml:"history"`
    Log       TUILogConfig     `yaml:"log"`
    Completion TUICompletionConfig `yaml:"completion"`
}

type TUIHistoryConfig struct {
    MaxEntries int `yaml:"max_entries"`
}

type TUILogConfig struct {
    BufferSize int `yaml:"buffer_size"`
}

type TUICompletionConfig struct {
    DebounceMs int `yaml:"debounce_ms"`
    MaxVisible int `yaml:"max_visible"`
}
```

Add field to `Config` struct:

```go
TUI TUIConfig `yaml:"tui"`
```

- [ ] **Step 3: Add defaults in the config loading**

In the config loading function, add defaults:

```go
if cfg.TUI.Theme == "" {
    cfg.TUI.Theme = "tokyo-night"
}
if cfg.TUI.History.MaxEntries == 0 {
    cfg.TUI.History.MaxEntries = 10000
}
if cfg.TUI.Log.BufferSize == 0 {
    cfg.TUI.Log.BufferSize = 200
}
if cfg.TUI.Completion.DebounceMs == 0 {
    cfg.TUI.Completion.DebounceMs = 60
}
if cfg.TUI.Completion.MaxVisible == 0 {
    cfg.TUI.Completion.MaxVisible = 16
}
```

- [ ] **Step 4: Run existing tests**

Run: `go test ./internal/config/`
Expected: All pass (new fields have zero-value defaults).

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/config/config.go
git commit -m "feat(tui): add Bubble Tea dependencies and TUI config struct"
```

---

### Task 2: Theme System

**Files:**
- Create: `internal/tui/theme.go`
- Test: `internal/tui/theme_test.go`

- [ ] **Step 1: Write theme tests**

```go
package tui

import (
    "testing"

    "github.com/charmbracelet/lipgloss/v2"
)

func TestLoadThemeTokyoNight(t *testing.T) {
    th := LoadTheme("tokyo-night")
    if th.Name != "tokyo-night" {
        t.Errorf("expected name tokyo-night, got %s", th.Name)
    }
    if th.Text != "#c0caf5" {
        t.Errorf("expected text #c0caf5, got %s", th.Text)
    }
}

func TestLoadThemeCatppuccinMocha(t *testing.T) {
    th := LoadTheme("catppuccin-mocha")
    if th.Name != "catppuccin-mocha" {
        t.Errorf("expected name catppuccin-mocha, got %s", th.Name)
    }
    if th.Text != "#cdd6f4" {
        t.Errorf("expected text #cdd6f4, got %s", th.Text)
    }
}

func TestLoadThemeLight(t *testing.T) {
    th := LoadTheme("light")
    if th.Name != "light" {
        t.Errorf("expected name light, got %s", th.Name)
    }
    if th.Text != "#333333" {
        t.Errorf("expected text #333333, got %s", th.Text)
    }
}

func TestLoadThemeTerminal(t *testing.T) {
    th := LoadTheme("terminal")
    if th.Name != "terminal" {
        t.Errorf("expected name terminal, got %s", th.Name)
    }
}

func TestLoadThemeUnknownDefaultsToTokyoNight(t *testing.T) {
    th := LoadTheme("unknown")
    if th.Name != "tokyo-night" {
        t.Errorf("expected fallback to tokyo-night, got %s", th.Name)
    }
}

func TestThemeStylesReturnsLipglossStyles(t *testing.T) {
    th := LoadTheme("tokyo-night")
    s := th.Styles()
    if s.UserText == (lipgloss.Style{}) {
        t.Error("expected UserText style to be non-zero")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestLoadTheme`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement theme.go**

```go
package tui

import (
    "github.com/charmbracelet/lipgloss/v2"
)

// Theme holds color definitions for a TUI theme.
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

// Styles holds pre-built Lip Gloss styles derived from a Theme.
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

// Themes is the registry of all built-in themes.
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

// LoadTheme returns a Theme by name, falling back to tokyo-night.
func LoadTheme(name string) Theme {
    th, ok := Themes[name]
    if !ok {
        return Themes["tokyo-night"]
    }
    return th
}

// Styles builds Lip Gloss styles from the theme.
func (th Theme) Styles() Styles {
    c := func(hex string) lipgloss.Color {
        if hex == "" {
            return lipgloss.Color("") // terminal default
        }
        return lipgloss.Color(hex)
    }
    return Styles{
        UserText:    lipgloss.NewStyle().Foreground(c(th.Text)),
        UserPrefix:  lipgloss.NewStyle().Foreground(c(th.Green)).Bold(true),
        Assistant:   lipgloss.NewStyle().Foreground(c(th.Text)).MarginLeft(2),
        ToolName:    lipgloss.NewStyle().Foreground(c(th.Green)),
        BashPrefix:  lipgloss.NewStyle().Foreground(c(th.Orange)),
        Delegate:    lipgloss.NewStyle().Foreground(c(th.Purple)),
        Skill:       lipgloss.NewStyle().Foreground(c(th.Yellow)),
        EventPrefix: lipgloss.NewStyle().Foreground(c(th.Accent)),
        Muted:       lipgloss.NewStyle().Foreground(c(th.Muted)),
        Accent:      lipgloss.NewStyle().Foreground(c(th.Accent)),
        StatusBar:   lipgloss.NewStyle().Foreground(c(th.Muted)).FontSize(1),
        Divider:     lipgloss.NewStyle().Foreground(c(th.Border)),
        Completion:  lipgloss.NewStyle().Foreground(c(th.Text)),
        CompActive:  lipgloss.NewStyle().Foreground(c(th.Purple)).Background(c(th.Border)),
        Error:       lipgloss.NewStyle().Foreground(c(th.Red)),
        Warning:     lipgloss.NewStyle().Foreground(c(th.Yellow)),
        Prompt:      lipgloss.NewStyle().Foreground(c(th.Orange)).Bold(true),
    }
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/theme.go internal/tui/theme_test.go
git commit -m "feat(tui): add theme system with four built-in themes"
```

---

### Task 3: Key Bindings

**Files:**
- Create: `internal/tui/keymap.go`

- [ ] **Step 1: Create keymap.go**

```go
package tui

// Key constants for Bubble Tea key matching.
const (
    KeyEnter      = "enter"
    KeyCtrlEnter  = "ctrl+enter"
    KeyTab        = "tab"
    KeyEsc        = "esc"
    KeyUp         = "up"
    KeyDown       = "down"
    KeyLeft       = "left"
    KeyRight      = "right"
    KeyHome       = "home"
    KeyEnd        = "end"
    KeyBackspace  = "backspace"
    KeyDelete     = "delete"
    KeyCtrlC      = "ctrl+c"
    KeyCtrlL      = "ctrl+l"
    KeyCtrlO      = "ctrl+o"
    KeyCtrlV      = "ctrl+v"
    KeyCtrlI      = "ctrl+i"
    KeyCtrlU      = "ctrl+u"
    KeyPgUp       = "pgup"
    KeyPgDown     = "pgdown"
    KeyMouse      = "mouse"
    KeyRunes      = "runes"
)
```

- [ ] **Step 2: Run build check**

Run: `go build ./internal/tui/`
Expected: Success.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/keymap.go
git commit -m "feat(tui): add key binding constants"
```

---

### Task 4: Log Ring Buffer

**Files:**
- Create: `internal/tui/logbuffer.go`
- Test: `internal/tui/logbuffer_test.go`

- [ ] **Step 1: Write log buffer tests**

```go
package tui

import (
    "strings"
    "testing"
)

func TestLogBufferAppendAndGet(t *testing.T) {
    lb := NewLogBuffer(5)
    lb.Append("line1")
    lb.Append("line2")
    lb.Append("line3")
    lines := lb.Last(3)
    if len(lines) != 3 {
        t.Fatalf("expected 3 lines, got %d", len(lines))
    }
    if lines[0].Content != "line1" {
        t.Errorf("expected line1, got %s", lines[0].Content)
    }
}

func TestLogBufferWraps(t *testing.T) {
    lb := NewLogBuffer(3)
    lb.Append("a")
    lb.Append("b")
    lb.Append("c")
    lb.Append("d") // wraps, drops "a"
    lines := lb.Last(3)
    if len(lines) != 3 {
        t.Fatalf("expected 3 lines, got %d", len(lines))
    }
    if lines[0].Content != "b" {
        t.Errorf("expected b, got %s", lines[0].Content)
    }
    if lines[2].Content != "d" {
        t.Errorf("expected d, got %s", lines[2].Content)
    }
}

func TestLogBufferLastMoreThanSize(t *testing.T) {
    lb := NewLogBuffer(2)
    lb.Append("x")
    lb.Append("y")
    lines := lb.Last(10)
    if len(lines) != 2 {
        t.Fatalf("expected 2 lines, got %d", len(lines))
    }
}

func TestLogBufferEmpty(t *testing.T) {
    lb := NewLogBuffer(5)
    lines := lb.Last(3)
    if len(lines) != 0 {
        t.Fatalf("expected 0 lines, got %d", len(lines))
    }
}

func TestLogBufferLevels(t *testing.T) {
    lb := NewLogBuffer(5)
    lb.AppendLevel("INFO", "info msg")
    lb.AppendLevel("WARN", "warn msg")
    lb.AppendLevel("ERROR", "err msg")
    lines := lb.Last(3)
    if lines[0].Level != "INFO" || lines[1].Level != "WARN" || lines[2].Level != "ERROR" {
        t.Error("level mismatch")
    }
}

func TestLogBufferSlogHandler(t *testing.T) {
    lb := NewLogBuffer(200)
    handler := lb.SlogHandler()
    if handler == nil {
        t.Fatal("expected non-nil handler")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestLogBuffer`
Expected: FAIL.

- [ ] **Step 3: Implement logbuffer.go**

```go
package tui

import (
    "context"
    "log/slog"
    "os"
    "sync"
)

// LogLine is a single log entry in the ring buffer.
type LogLine struct {
    Level   string
    Content string
}

// LogBuffer is a thread-safe ring buffer for log lines.
type LogBuffer struct {
    mu     sync.Mutex
    buf    []LogLine
    size   int
    count  int
    head   int
}

// NewLogBuffer creates a ring buffer with the given capacity.
func NewLogBuffer(size int) *LogBuffer {
    return &LogBuffer{
        buf:  make([]LogLine, size),
        size: size,
    }
}

// Append adds a log line (no level).
func (lb *LogBuffer) Append(content string) {
    lb.AppendLevel("INFO", content)
}

// AppendLevel adds a log line with a level.
func (lb *LogBuffer) AppendLevel(level, content string) {
    lb.mu.Lock()
    defer lb.mu.Unlock()
    idx := (lb.head + lb.count) % lb.size
    lb.buf[idx] = LogLine{Level: level, Content: content}
    if lb.count < lb.size {
        lb.count++
    } else {
        lb.head = (lb.head + 1) % lb.size
    }
}

// Last returns the most recent n lines (or fewer if not enough).
func (lb *LogBuffer) Last(n int) []LogLine {
    lb.mu.Lock()
    defer lb.mu.Unlock()
    if n > lb.count {
        n = lb.count
    }
    result := make([]LogLine, n)
    start := (lb.head + lb.count - n) % lb.size
    for i := 0; i < n; i++ {
        idx := (start + i) % lb.size
        result[i] = lb.buf[idx]
    }
    return result
}

// SlogHandler returns a slog.Handler that writes to this buffer and stderr.
func (lb *LogBuffer) SlogHandler() slog.Handler {
    return &logBufferHandler{
        buf:    lb,
        inner:  slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}),
    }
}

type logBufferHandler struct {
    buf   *LogBuffer
    inner slog.Handler
}

func (h *logBufferHandler) Enabled(ctx context.Context, level slog.Level) bool {
    return h.inner.Enabled(ctx, level)
}

func (h *logBufferHandler) Handle(ctx context.Context, rec slog.Record) error {
    h.buf.AppendLevel(rec.Level.String(), rec.Message)
    return h.inner.Handle(ctx, rec)
}

func (h *logBufferHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &logBufferHandler{buf: h.buf, inner: h.inner.WithAttrs(attrs)}
}

func (h *logBufferHandler) WithGroup(name string) slog.Handler {
    return &logBufferHandler{buf: h.buf, inner: h.inner.WithGroup(name)}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -run TestLogBuffer -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/logbuffer.go internal/tui/logbuffer_test.go
git commit -m "feat(tui): add log ring buffer with slog handler"
```

---

### Task 5: History Manager

**Files:**
- Create: `internal/tui/history.go`
- Test: `internal/tui/history_test.go`

- [ ] **Step 1: Write history tests**

```go
package tui

import (
    "os"
    "path/filepath"
    "testing"
)

func TestHistorySaveAndLoad(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "history")
    h := NewHistory(path, 100)

    h.Add("hello")
    h.Add("/status")
    h.Add("world")

    if h.Len() != 3 {
        t.Fatalf("expected 3, got %d", h.Len())
    }
}

func TestHistoryNavigation(t *testing.T) {
    h := NewHistory("", 100)
    h.Add("first")
    h.Add("second")
    h.Add("third")

    if h.Current() != "" {
        t.Error("expected empty current before navigation")
    }
    if h.Older() != "third" {
        t.Error("expected third")
    }
    if h.Older() != "second" {
        t.Error("expected second")
    }
    if h.Older() != "first" {
        t.Error("expected first")
    }
    if h.Older() != "first" {
        t.Error("expected first (clamped)")
    }
    if h.Newer() != "second" {
        t.Error("expected second")
    }
}

func TestHistoryMaxEntries(t *testing.T) {
    h := NewHistory("", 3)
    h.Add("a")
    h.Add("b")
    h.Add("c")
    h.Add("d") // evicts "a"
    if h.Len() != 3 {
        t.Fatalf("expected 3, got %d", h.Len())
    }
}

func TestHistorySaveLoadRoundTrip(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "history")

    h1 := NewHistory(path, 100)
    h1.Add("line1")
    h1.Add("line2")
    h1.Save()

    h2 := NewHistory(path, 100)
    if h2.Len() != 2 {
        t.Fatalf("expected 2, got %d", h2.Len())
    }
    if h2.Older() != "line2" {
        t.Error("expected line2")
    }
}

func TestHistoryPrefixFilter(t *testing.T) {
    h := NewHistory("", 100)
    h.Add("/help")
    h.Add("/status")
    h.Add("hello")
    h.Add("/model")

    results := h.Search("/")
    if len(results) != 3 {
        t.Fatalf("expected 3, got %d", len(results))
    }
}

func TestHistoryNoDupes(t *testing.T) {
    h := NewHistory("", 100)
    h.Add("same")
    h.Add("same")
    h.Add("same")
    if h.Len() != 1 {
        t.Fatalf("expected 1 deduplicated entry, got %d", h.Len())
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestHistory`
Expected: FAIL.

- [ ] **Step 3: Implement history.go**

```go
package tui

import (
    "bufio"
    "os"
    "strings"
    "sync"
)

// History manages persistent command history.
type History struct {
    mu      sync.Mutex
    path    string
    max     int
    entries []string
    idx     int
    draft   string
}

// NewHistory creates a history manager. If path is empty, history is memory-only.
func NewHistory(path string, maxEntries int) *History {
    h := &History{
        path: path,
        max:  maxEntries,
        idx:  -1,
    }
    h.load()
    return h
}

// Add appends an entry, deduplicating consecutive duplicates.
func (h *History) Add(entry string) {
    if entry == "" {
        return
    }
    h.mu.Lock()
    defer h.mu.Unlock()
    if len(h.entries) > 0 && h.entries[len(h.entries)-1] == entry {
        return
    }
    h.entries = append(h.entries, entry)
    if len(h.entries) > h.max {
        h.entries = h.entries[len(h.entries)-h.max:]
    }
    h.idx = -1
    h.draft = ""
}

// Len returns the number of history entries.
func (h *History) Len() int {
    h.mu.Lock()
    defer h.mu.Unlock()
    return len(h.entries)
}

// Older moves toward older entries and returns the entry. Saves draft on first call.
func (h *History) Older() string {
    h.mu.Lock()
    defer h.mu.Unlock()
    n := len(h.entries)
    if n == 0 {
        return ""
    }
    if h.idx == -1 {
        h.draft = ""
        h.idx = n - 1
    } else if h.idx > 0 {
        h.idx--
    }
    return h.entries[h.idx]
}

// Newer moves toward newer entries. Returns draft when reaching the end.
func (h *History) Newer() string {
    h.mu.Lock()
    defer h.mu.Unlock()
    if h.idx == -1 {
        return h.draft
    }
    h.idx++
    if h.idx >= len(h.entries) {
        h.idx = -1
        return h.draft
    }
    return h.entries[h.idx]
}

// ResetNav resets navigation position (call when user types).
func (h *History) ResetNav(draft string) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.idx = -1
    h.draft = draft
}

// Current returns the current navigation entry without moving.
func (h *History) Current() string {
    h.mu.Lock()
    defer h.mu.Unlock()
    if h.idx == -1 || h.idx >= len(h.entries) {
        return ""
    }
    return h.entries[h.idx]
}

// Search returns entries matching the given prefix.
func (h *History) Search(prefix string) []string {
    h.mu.Lock()
    defer h.mu.Unlock()
    var results []string
    for i := len(h.entries) - 1; i >= 0; i-- {
        if strings.HasPrefix(h.entries[i], prefix) {
            results = append(results, h.entries[i])
        }
    }
    return results
}

// Save persists history to disk.
func (h *History) Save() {
    h.mu.Lock()
    defer h.mu.Unlock()
    if h.path == "" {
        return
    }
    f, err := os.Create(h.path)
    if err != nil {
        return
    }
    defer f.Close()
    w := bufio.NewWriter(f)
    for _, e := range h.entries {
        w.WriteString(e)
        w.WriteByte('\n')
    }
    w.Flush()
}

func (h *History) load() {
    if h.path == "" {
        return
    }
    f, err := os.Open(h.path)
    if err != nil {
        return
    }
    defer f.Close()
    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        line := scanner.Text()
        if line != "" {
            h.entries = append(h.entries, line)
        }
    }
    if len(h.entries) > h.max {
        h.entries = h.entries[len(h.entries)-h.max:]
    }
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -run TestHistory -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/history.go internal/tui/history_test.go
git commit -m "feat(tui): add persistent command history manager"
```

---

### Task 6: Message Types

**Files:**
- Create: `internal/tui/message.go`
- Test: `internal/tui/message_test.go`

- [ ] **Step 1: Write message tests**

```go
package tui

import "testing"

func TestToolCallIcon(t *testing.T) {
    tests := []struct {
        name string
        want string
    }{
        {"ReadFile", "⚙"},
        {"WriteFile", "⚙"},
        {"Grep", "⚙"},
        {"Glob", "⚙"},
        {"web_search", "⚙"},
        {"web_extract", "⚙"},
        {"Bash", "$"},
        {"delegate_task", "⚡"},
        {"skill_create", "★"},
        {"skill_delete", "★"},
        {"vision", "🖼"},
        {"unknown", "⚙"},
    }
    for _, tt := range tests {
        got := ToolCallIcon(tt.name)
        if got != tt.want {
            t.Errorf("ToolCallIcon(%q) = %q, want %q", tt.name, got, tt.want)
        }
    }
}

func TestToolCallFormat(t *testing.T) {
    tc := ToolCall{
        Name:     "ReadFile",
        Detail:   "internal/config/config.go",
        Duration: "0.4s",
        Status:   ToolStatusDone,
    }
    got := tc.Format()
    if got != "⚙ read config.go 0.4s" {
        t.Errorf("unexpected format: %q", got)
    }
}

func TestToolCallFormatNested(t *testing.T) {
    parent := ToolCall{
        Name:   "delegate_task",
        Detail: "configure weixin",
        Children: []ToolCall{
            {Name: "Grep", Detail: "weixin.*setup", Duration: "0.4s", Status: ToolStatusDone},
            {Name: "ReadFile", Detail: "client.go", Duration: "0.8s", Status: ToolStatusDone},
        },
    }
    lines := parent.FormatTree(0)
    if len(lines) != 3 {
        t.Fatalf("expected 3 lines, got %d", len(lines))
    }
    if lines[0] != "⚡ delegate_task configure weixin" {
        t.Errorf("unexpected first line: %q", lines[0])
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestToolCall`
Expected: FAIL.

- [ ] **Step 3: Implement message.go**

```go
package tui

import "strings"

// MsgKind distinguishes message types in the transcript.
type MsgKind int

const (
    MsgUser      MsgKind = iota // User input
    MsgAssistant                 // Agent text response
    MsgToolCall                  // Tool invocation (may have children)
    MsgEvent                     // System event notification
)

// ToolStatus represents tool call state.
type ToolStatus int

const (
    ToolStatusRunning ToolStatus = iota
    ToolStatusDone
    ToolStatusError
)

// ToolCall represents a single tool invocation in the transcript.
type ToolCall struct {
    Name     string
    Detail   string // file path, command summary, query
    Duration string // "0.4s", "" if running
    Status   ToolStatus
    Output   string     // collapsed output text
    Collapsed bool      // true = output hidden
    Children []ToolCall // nested calls (delegate/skill)
}

// TranscriptMsg is a single message displayed in the transcript.
type TranscriptMsg struct {
    Kind      MsgKind
    Content   string     // text for user/assistant messages
    Tool      *ToolCall   // for MsgToolCall
    EventIcon string     // for MsgEvent: 📬 ⏰ 💾
    EventSrc  string     // event source label
    Thinking  string     // ReasoningContent (collapsed)
    ThinkingOpen bool    // true = thinking block expanded
    Images    []string   // attached image paths
}

// ToolCallIcon returns the display icon for a tool name.
func ToolCallIcon(name string) string {
    switch name {
    case "Bash":
        return "$"
    case "delegate_task":
        return "⚡"
    case "skill_create", "skill_delete", "skill_list":
        return "★"
    case "vision":
        return "🖼"
    case "ReadFile", "WriteFile", "Grep", "Glob",
        "web_search", "web_extract", "Patch":
        return "⚙"
    default:
        return "⚙"
    }
}

// toolCallShortName returns a human-readable short name for display.
func toolCallShortName(name string) string {
    switch name {
    case "ReadFile":
        return "read"
    case "WriteFile":
        return "write"
    case "web_search":
        return "search"
    case "web_extract":
        return "extract"
    default:
        return strings.ToLower(name)
    }
}

// Format renders a single tool call line (no indent).
func (tc ToolCall) Format() string {
    icon := ToolCallIcon(tc.Name)
    short := toolCallShortName(tc.Name)
    parts := []string{icon + " " + short}
    if tc.Detail != "" {
        parts = append(parts, tc.Detail)
    }
    if tc.Duration != "" {
        parts = append(parts, tc.Duration)
    }
    if tc.Status == ToolStatusRunning {
        parts = append(parts, "...")
    }
    return strings.Join(parts, " ")
}

// FormatTree renders a tool call and its children as indented lines.
func (tc ToolCall) FormatTree(depth int) []string {
    var lines []string
    indent := strings.Repeat("┊ ", depth)
    if depth == 0 {
        lines = append(lines, tc.Format())
    } else {
        lines = append(lines, indent+tc.Format())
    }
    for _, child := range tc.Children {
        lines = append(lines, child.FormatTree(depth+1)...)
    }
    return lines
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -run TestToolCall -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/message.go internal/tui/message_test.go
git commit -m "feat(tui): add message types and tool call tree rendering"
```

---

### Task 7: Completion Engine

**Files:**
- Create: `internal/tui/completion.go`
- Test: `internal/tui/completion_test.go`

- [ ] **Step 1: Write completion tests**

```go
package tui

import "testing"

func TestCompletionMatchSlash(t *testing.T) {
    engine := NewCompletionEngine(defaultCommands)
    items := engine.Match("/h")
    if len(items) == 0 {
        t.Fatal("expected matches for /h")
    }
    found := false
    for _, item := range items {
        if item.Text == "/help" {
            found = true
        }
    }
    if !found {
        t.Error("expected /help in results")
    }
}

func TestCompletionMatchExact(t *testing.T) {
    engine := NewCompletionEngine(defaultCommands)
    items := engine.Match("/status")
    if len(items) != 1 {
        t.Fatalf("expected 1 match, got %d", len(items))
    }
    if items[0].Text != "/status" {
        t.Errorf("expected /status, got %s", items[0].Text)
    }
}

func TestCompletionNoMatch(t *testing.T) {
    engine := NewCompletionEngine(defaultCommands)
    items := engine.Match("/xyz")
    if len(items) != 0 {
        t.Fatalf("expected 0 matches, got %d", len(items))
    }
}

func TestCompletionNotSlash(t *testing.T) {
    engine := NewCompletionEngine(defaultCommands)
    items := engine.Match("hello")
    if len(items) != 0 {
        t.Fatalf("expected 0 matches for non-slash input, got %d", len(items))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestCompletion`
Expected: FAIL.

- [ ] **Step 3: Implement completion.go**

```go
package tui

import (
    "sort"
    "strings"
)

// CompletionItem is a single completion suggestion.
type CompletionItem struct {
    Text        string // e.g. "/help"
    Display     string // e.g. "/help"
    Description string // e.g. "Show available commands"
}

// CompletionEngine provides slash command completion.
type CompletionEngine struct {
    commands []CompletionItem
}

// defaultCommands lists all built-in slash commands.
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

// NewCompletionEngine creates a completion engine with the given commands.
func NewCompletionEngine(commands []CompletionItem) *CompletionEngine {
    return &CompletionEngine{commands: commands}
}

// DefaultCompletionEngine creates an engine with built-in commands.
func DefaultCompletionEngine() *CompletionEngine {
    return NewCompletionEngine(defaultCommands)
}

// Match returns completion items matching the given input.
// Only matches when input starts with "/".
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -run TestCompletion -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/completion.go internal/tui/completion_test.go
git commit -m "feat(tui): add slash command completion engine"
```

---

### Task 8: Markdown Renderer

**Files:**
- Create: `internal/tui/markdown.go`
- Test: `internal/tui/markdown_test.go`

- [ ] **Step 1: Write markdown tests**

```go
package tui

import "testing"

func TestRenderCodeBlock(t *testing.T) {
    input := "```go\nfmt.Println(\"hello\")\n```"
    lines := RenderMarkdown(input, Themes["tokyo-night"])
    if len(lines) == 0 {
        t.Fatal("expected non-empty output")
    }
}

func TestRenderBold(t *testing.T) {
    input := "this is **bold** text"
    lines := RenderMarkdown(input, Themes["tokyo-night"])
    if len(lines) == 0 {
        t.Fatal("expected non-empty output")
    }
}

func TestRenderInlineCode(t *testing.T) {
    input := "use `fmt.Println` to print"
    lines := RenderMarkdown(input, Themes["tokyo-night"])
    if len(lines) == 0 {
        t.Fatal("expected non-empty output")
    }
}

func TestRenderPlainText(t *testing.T) {
    input := "hello world"
    lines := RenderMarkdown(input, Themes["tokyo-night"])
    if len(lines) != 1 {
        t.Fatalf("expected 1 line, got %d", len(lines))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestRender`
Expected: FAIL.

- [ ] **Step 3: Implement markdown.go**

```go
package tui

import (
    "regexp"
    "strings"

    "github.com/charmbracelet/lipgloss/v2"
)

var (
    reCodeBlock   = regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```")
    reBold        = regexp.MustCompile("\\*\\*(.+?)\\*\\*")
    reItalic      = regexp.MustCompile("\\*(.+?)\\*")
    reInlineCode  = regexp.MustCompile("`([^`]+)`")
    reHeader      = regexp.MustCompile("^(#{1,6})\\s+(.+)$")
)

// RenderMarkdown converts markdown text to a slice of styled strings.
// Each string is one display line with Lip Gloss styling applied.
func RenderMarkdown(text string, th Theme) []string {
    styles := th.Styles()

    // Handle code blocks first (replace with placeholder)
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
            // Extract code block index
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

        // Apply inline formatting
        styled := applyInlineStyles(line, styles)
        result = append(result, styled)
    }
    return result
}

func applyInlineStyles(text string, s Styles) string {
    // Bold
    text = reBold.ReplaceAllStringFunc(text, func(match string) string {
        inner := reBold.FindStringSubmatch(match)[1]
        return lipgloss.NewStyle().Bold(true).Render(inner)
    })
    // Inline code
    text = reInlineCode.ReplaceAllStringFunc(text, func(match string) string {
        inner := reInlineCode.FindStringSubmatch(match)[1]
        return s.Muted.Render(inner)
    })
    // Italic
    text = reItalic.ReplaceAllStringFunc(text, func(match string) string {
        inner := reItalic.FindStringSubmatch(match)[1]
        return lipgloss.NewStyle().Italic(true).Render(inner)
    })
    return text
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -run TestRender -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/markdown.go internal/tui/markdown_test.go
git commit -m "feat(tui): add markdown renderer with code blocks and inline styles"
```

---

### Task 9: Status Bar Component

**Files:**
- Create: `internal/tui/statusbar.go`
- Test: `internal/tui/statusbar_test.go`

- [ ] **Step 1: Write status bar tests**

```go
package tui

import "testing"

func TestStatusBarRender(t *testing.T) {
    th := LoadTheme("tokyo-night")
    sb := NewStatusBar(th)
    sb.SetModel("deepseek-v4-flash")
    sb.SetContextUsage(1200, 200000)
    sb.SetCronActive(true)

    rendered := sb.Render(80)
    if rendered == "" {
        t.Error("expected non-empty render")
    }
}

func TestStatusBarWithAgents(t *testing.T) {
    th := LoadTheme("tokyo-night")
    sb := NewStatusBar(th)
    sb.SetModel("deepseek-v4-flash")
    sb.SetAgentCount(2)
    sb.SetBackgroundTasks(1)

    rendered := sb.Render(80)
    if rendered == "" {
        t.Error("expected non-empty render")
    }
}

func TestStatusBarHideZeroCounts(t *testing.T) {
    th := LoadTheme("tokyo-night")
    sb := NewStatusBar(th)
    sb.SetModel("test")
    sb.SetAgentCount(0)
    sb.SetBackgroundTasks(0)

    rendered := sb.Render(80)
    if rendered == "" {
        t.Error("expected non-empty render")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestStatusBar`
Expected: FAIL.

- [ ] **Step 3: Implement statusbar.go**

```go
package tui

import (
    "fmt"
    "strings"

    "github.com/charmbracelet/lipgloss/v2"
)

// StatusBar displays model, context, and status info. Left-aligned.
type StatusBar struct {
    theme     Theme
    styles    Styles
    model     string
    state     string // "ready", "busy", "error"
    ctxInput  int
    ctxTotal  int
    agents    int
    bgTasks   int
    cronOn    bool
    elapsed   string
}

// NewStatusBar creates a status bar with the given theme.
func NewStatusBar(th Theme) *StatusBar {
    return &StatusBar{
        theme:  th,
        styles: th.Styles(),
        state:  "ready",
    }
}

func (sb *StatusBar) SetModel(m string)      { sb.model = m }
func (sb *StatusBar) SetState(s string)       { sb.state = s }
func (sb *StatusBar) SetContextUsage(input, total int) {
    sb.ctxInput = input
    sb.ctxTotal = total
}
func (sb *StatusBar) SetAgentCount(n int)     { sb.agents = n }
func (sb *StatusBar) SetBackgroundTasks(n int) { sb.bgTasks = n }
func (sb *StatusBar) SetCronActive(on bool)   { sb.cronOn = on }
func (sb *StatusBar) SetElapsed(s string)     { sb.elapsed = s }

// Render produces the status bar string for the given terminal width.
func (sb *StatusBar) Render(width int) string {
    var parts []string

    // Dot + model
    dotColor := sb.theme.Green
    if sb.state == "busy" {
        dotColor = sb.theme.Yellow
    } else if sb.state == "error" {
        dotColor = sb.theme.Red
    }
    dot := lipgloss.NewStyle().Foreground(lipgloss.Color(dotColor)).Render("●")
    parts = append(parts, dot+" "+sb.model)

    // Context
    if sb.ctxTotal > 0 {
        ctxStr := fmt.Sprintf("ctx: %s/%s", formatTokens(sb.ctxInput), formatTokens(sb.ctxTotal))
        parts = append(parts, ctxStr)
    }

    // Agents (hide when 0)
    if sb.agents > 0 {
        parts = append(parts, fmt.Sprintf("agents: %d", sb.agents))
    }

    // Background tasks (hide when 0)
    if sb.bgTasks > 0 {
        parts = append(parts, fmt.Sprintf("bg: %d", sb.bgTasks))
    }

    // Cron
    if sb.cronOn {
        parts = append(parts, "cron: active")
    }

    // Elapsed
    if sb.elapsed != "" {
        parts = append(parts, sb.elapsed)
    }

    line := strings.Join(parts, " │ ")
    return sb.styles.Muted.Render(line)
}

func formatTokens(n int) string {
    if n >= 1000000 {
        return fmt.Sprintf("%.1fM", float64(n)/1000000)
    }
    if n >= 1000 {
        return fmt.Sprintf("%.1fk", float64(n)/1000)
    }
    return fmt.Sprintf("%d", n)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -run TestStatusBar -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/statusbar.go internal/tui/statusbar_test.go
git commit -m "feat(tui): add left-aligned status bar component"
```

---

### Task 10: Composer (Multi-line Input)

**Files:**
- Create: `internal/tui/composer.go`
- Test: `internal/tui/composer_test.go`

- [ ] **Step 1: Write composer tests**

```go
package tui

import "testing"

func TestComposerInsertRune(t *testing.T) {
    c := NewComposer(nil, nil)
    c.InsertRune('a')
    c.InsertRune('b')
    if c.Text() != "ab" {
        t.Errorf("expected 'ab', got %q", c.Text())
    }
}

func TestComposerBackspace(t *testing.T) {
    c := NewComposer(nil, nil)
    c.InsertRune('a')
    c.InsertRune('b')
    c.Backspace()
    if c.Text() != "a" {
        t.Errorf("expected 'a', got %q", c.Text())
    }
}

func TestComposerNewLine(t *testing.T) {
    c := NewComposer(nil, nil)
    c.InsertRune('a')
    c.InsertNewLine()
    c.InsertRune('b')
    if c.Text() != "a\nb" {
        t.Errorf("expected 'a\\nb', got %q", c.Text())
    }
    if c.LineCount() != 2 {
        t.Errorf("expected 2 lines, got %d", c.LineCount())
    }
}

func TestComposerBackspaceMergeLines(t *testing.T) {
    c := NewComposer(nil, nil)
    c.InsertRune('a')
    c.InsertNewLine()
    c.InsertRune('b')
    c.Backspace() // at line start, merge with previous
    if c.Text() != "ab" {
        t.Errorf("expected 'ab', got %q", c.Text())
    }
    if c.LineCount() != 1 {
        t.Errorf("expected 1 line, got %d", c.LineCount())
    }
}

func TestComposerClear(t *testing.T) {
    c := NewComposer(nil, nil)
    c.InsertRune('x')
    c.Clear()
    if c.Text() != "" {
        t.Errorf("expected empty, got %q", c.Text())
    }
}

func TestComposerSetInput(t *testing.T) {
    c := NewComposer(nil, nil)
    c.SetInput("hello world")
    if c.Text() != "hello world" {
        t.Errorf("expected 'hello world', got %q", c.Text())
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestComposer`
Expected: FAIL.

- [ ] **Step 3: Implement composer.go**

```go
package tui

import (
    "strings"
)

// Composer manages multi-line text input with cursor movement.
type Composer struct {
    lines  []string
    curRow int
    curCol int
    comp   *CompletionEngine
    hist   *History
    compItems []CompletionItem
    compIdx  int
    compActive bool
    attachments []Attachment
}

// Attachment holds a file/image attached to the input.
type Attachment struct {
    Path     string
    IsImage  bool
}

// NewComposer creates a multi-line input composer.
func NewComposer(comp *CompletionEngine, hist *History) *Composer {
    return &Composer{
        lines: []string{""},
    }
}

// Text returns the full input text (lines joined with \n).
func (c *Composer) Text() string {
    return strings.Join(c.lines, "\n")
}

// LineCount returns the number of lines in the input buffer.
func (c *Composer) LineCount() int {
    return len(c.lines)
}

// IsEmpty returns true if the input is empty.
func (c *Composer) IsEmpty() bool {
    return len(c.lines) == 1 && c.lines[0] == ""
}

// SetInput replaces the entire input text.
func (c *Composer) SetInput(text string) {
    c.lines = strings.Split(text, "\n")
    if len(c.lines) == 0 {
        c.lines = []string{""}
    }
    c.curRow = len(c.lines) - 1
    c.curCol = len(c.lines[c.curRow])
}

// InsertRune inserts a character at the cursor position.
func (c *Composer) InsertRune(r rune) {
    line := c.lines[c.curRow]
    c.lines[c.curRow] = line[:c.curCol] + string(r) + line[c.curCol:]
    c.curCol++
}

// InsertNewLine adds a new line at the cursor (Ctrl+Enter).
func (c *Composer) InsertNewLine() {
    line := c.lines[c.curRow]
    before := line[:c.curCol]
    after := line[c.curCol:]
    c.lines[c.curRow] = before
    c.lines = append(c.lines, "")
    copy(c.lines[c.curRow+2:], c.lines[c.curRow+1:])
    c.lines[c.curRow+1] = after
    c.curRow++
    c.curCol = 0
}

// Backspace deletes the character before the cursor. At line start, merges with previous line.
func (c *Composer) Backspace() {
    if c.curCol > 0 {
        line := c.lines[c.curRow]
        c.lines[c.curRow] = line[:c.curCol-1] + line[c.curCol:]
        c.curCol--
    } else if c.curRow > 0 {
        prevLen := len(c.lines[c.curRow-1])
        c.lines[c.curRow-1] += c.lines[c.curRow]
        c.lines = append(c.lines[:c.curRow], c.lines[c.curRow+1:]...)
        c.curRow--
        c.curCol = prevLen
    }
}

// Delete removes the character after the cursor.
func (c *Composer) Delete() {
    line := c.lines[c.curRow]
    if c.curCol < len(line) {
        c.lines[c.curRow] = line[:c.curCol] + line[c.curCol+1:]
    } else if c.curRow < len(c.lines)-1 {
        c.lines[c.curRow] += c.lines[c.curRow+1]
        c.lines = append(c.lines[:c.curRow+1], c.lines[c.curRow+2:]...)
    }
}

// Clear resets the input buffer.
func (c *Composer) Clear() {
    c.lines = []string{""}
    c.curRow = 0
    c.curCol = 0
    c.compActive = false
    c.compItems = nil
}

// CursorPos returns the current (row, col) position.
func (c *Composer) CursorPos() (row, col int) {
    return c.curRow, c.curCol
}

// MoveLeft moves cursor left, wrapping to previous line.
func (c *Composer) MoveLeft() {
    if c.curCol > 0 {
        c.curCol--
    } else if c.curRow > 0 {
        c.curRow--
        c.curCol = len(c.lines[c.curRow])
    }
}

// MoveRight moves cursor right, wrapping to next line.
func (c *Composer) MoveRight() {
    if c.curCol < len(c.lines[c.curRow]) {
        c.curCol++
    } else if c.curRow < len(c.lines)-1 {
        c.curRow++
        c.curCol = 0
    }
}

// MoveHome moves cursor to start of current line.
func (c *Composer) MoveHome() {
    c.curCol = 0
}

// MoveEnd moves cursor to end of current line.
func (c *Composer) MoveEnd() {
    c.curCol = len(c.lines[c.curRow])
}

// AddAttachment adds a file/image attachment.
func (c *Composer) AddAttachment(path string, isImage bool) {
    c.attachments = append(c.attachments, Attachment{Path: path, IsImage: isImage})
}

// RemoveAttachment removes attachment at index.
func (c *Composer) RemoveAttachment(idx int) {
    if idx >= 0 && idx < len(c.attachments) {
        c.attachments = append(c.attachments[:idx], c.attachments[idx+1:]...)
    }
}

// Attachments returns the current attachment list.
func (c *Composer) Attachments() []Attachment {
    return c.attachments
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -run TestComposer -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/composer.go internal/tui/composer_test.go
git commit -m "feat(tui): add multi-line input composer with cursor movement"
```

---

### Task 11: Transcript (Virtual Scroll)

**Files:**
- Create: `internal/tui/transcript.go`

- [ ] **Step 1: Implement transcript.go**

```go
package tui

import (
    "github.com/charmbracelet/lipgloss/v2"
)

// Transcript manages the scrollable message list in the upper pane.
type Transcript struct {
    msgs       []TranscriptMsg
    styles     Styles
    height     int
    width      int
    yOffset    int    // scroll offset from top
    atBottom   bool   // auto-scroll to bottom
    maxID      int    // monotonic ID for height tracking
}

// NewTranscript creates a transcript view with the given styles.
func NewTranscript(styles Styles) *Transcript {
    return &Transcript{
        styles:   styles,
        atBottom: true,
    }
}

// Resize updates the visible area dimensions.
func (tr *Transcript) Resize(w, h int) {
    tr.width = w
    tr.height = h
    if tr.atBottom {
        tr.scrollToBottom()
    }
}

// Append adds a message to the transcript.
func (tr *Transcript) Append(msg TranscriptMsg) {
    tr.msgs = append(tr.msgs, msg)
    if tr.atBottom {
        tr.scrollToBottom()
    }
}

// UpdateLast replaces the last message (used for streaming updates).
func (tr *Transcript) UpdateLast(msg TranscriptMsg) {
    if len(tr.msgs) > 0 {
        tr.msgs[len(tr.msgs)-1] = msg
    }
}

// ScrollUp moves the viewport up. Disables auto-scroll.
func (tr *Transcript) ScrollUp(n int) {
    tr.yOffset -= n
    if tr.yOffset < 0 {
        tr.yOffset = 0
    }
    tr.atBottom = false
}

// ScrollDown moves the viewport down.
func (tr *Transcript) ScrollDown(n int) {
    tr.yOffset += n
    maxOffset := tr.maxScrollOffset()
    if tr.yOffset >= maxOffset {
        tr.yOffset = maxOffset
        tr.atBottom = true
    }
}

// ScrollToBottom re-enables auto-scroll.
func (tr *Transcript) ScrollToBottom() {
    tr.atBottom = true
    tr.scrollToBottom()
}

// Messages returns all messages (for external iteration).
func (tr *Transcript) Messages() []TranscriptMsg {
    return tr.msgs
}

// Render produces the transcript view as a string for the given area.
func (tr *Transcript) Render() string {
    if tr.height <= 0 || tr.width <= 0 {
        return ""
    }

    // Flatten all messages into renderable lines
    var allLines []string
    for _, msg := range tr.msgs {
        allLines = append(allLines, tr.renderMessage(msg)...)
    }

    totalLines := len(allLines)
    visibleHeight := tr.height

    // Calculate visible window
    var start int
    if tr.atBottom || totalLines <= visibleHeight {
        start = totalLines - visibleHeight
        if start < 0 {
            start = 0
        }
    } else {
        start = tr.yOffset
    }
    end := start + visibleHeight
    if end > totalLines {
        end = totalLines
    }

    var visible []string
    if start < end {
        visible = allLines[start:end]
    }

    // Pad if fewer lines than height
    for len(visible) < visibleHeight {
        visible = append([]string{""}, visible...)
    }

    // Add scrollbar
    content := lipgloss.NewStyle().
        Width(tr.width - 2).
        Render(joinLines(visible))

    if totalLines > visibleHeight {
        scrollbar := tr.renderScrollbar(start, totalLines, visibleHeight)
        return lipgloss.JoinHorizontal(lipgloss.Left, content, scrollbar)
    }
    return content
}

func (tr *Transcript) renderMessage(msg TranscriptMsg) []string {
    var lines []string
    switch msg.Kind {
    case MsgUser:
        prefix := tr.styles.UserPrefix.Render("❯ ")
        text := tr.styles.UserText.Render(msg.Content)
        lines = append(lines, prefix+text)
        for _, img := range msg.Images {
            lines = append(lines, tr.styles.EventPrefix.Render("🖼 "+img))
        }
    case MsgAssistant:
        theme := Themes["tokyo-night"] // resolved from app state in real impl
        mdLines := RenderMarkdown(msg.Content, theme)
        if len(mdLines) == 0 {
            mdLines = []string{msg.Content}
        }
        for _, l := range mdLines {
            lines = append(lines, tr.styles.Assistant.Render(l))
        }
    case MsgToolCall:
        if msg.Tool != nil {
            treeLines := msg.Tool.FormatTree(0)
            for _, tl := range treeLines {
                lines = append(lines, "  "+tl)
            }
        }
    case MsgEvent:
        icon := msg.EventIcon
        src := msg.EventSrc
        content := msg.Content
        line := tr.styles.Muted.Render("--- ") +
            tr.styles.EventPrefix.Render(icon+" "+src) +
            tr.styles.Muted.Render("  "+content+" ---")
        lines = append(lines, line)
    }
    return lines
}

func (tr *Transcript) renderScrollbar(offset, total, visible int) string {
    thumbSize := max(1, visible*visible/total)
    thumbPos := offset * visible / total

    var sb []rune
    for i := 0; i < visible; i++ {
        if i >= thumbPos && i < thumbPos+thumbSize {
            sb = append(sb, '█')
        } else {
            sb = append(sb, '│')
        }
    }
    return lipgloss.NewStyle().Foreground(lipgloss.Color(tr.styles.Muted.Render("")[:7])).String()
}

func (tr *Transcript) scrollToBottom() {
    tr.yOffset = tr.maxScrollOffset()
}

func (tr *Transcript) maxScrollOffset() int {
    return len(tr.msgs) // approximate; real impl tracks line counts
}

func joinLines(lines []string) string {
    result := ""
    for i, l := range lines {
        if i > 0 {
            result += "\n"
        }
        result += l
    }
    return result
}
```

- [ ] **Step 2: Run build check**

Run: `go build ./internal/tui/`

Note: The `renderMessage` method references a ternary-like expression. Fix the theme resolution:

Replace the problematic line with:
```go
theme := Themes["tokyo-night"] // resolved from app state in real impl
```

- [ ] **Step 3: Fix and rebuild**

Run: `go build ./internal/tui/`
Expected: Success.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/transcript.go
git commit -m "feat(tui): add virtual scroll transcript view"
```

---

### Task 12: Tool Approval Popup

**Files:**
- Create: `internal/tui/approval.go`
- Test: `internal/tui/approval_test.go`

- [ ] **Step 1: Write approval tests**

```go
package tui

import "testing"

func TestApprovalParseYes(t *testing.T) {
    a := ApprovalRequest{ToolName: "Bash", Detail: "rm -rf /tmp"}
    result := a.HandleKey("y")
    if result != ApprovalAllow {
        t.Errorf("expected Allow, got %d", result)
    }
}

func TestApprovalParseNo(t *testing.T) {
    a := ApprovalRequest{ToolName: "Bash", Detail: "rm -rf /tmp"}
    result := a.HandleKey("n")
    if result != ApprovalDeny {
        t.Errorf("expected Deny, got %d", result)
    }
}

func TestApprovalParseAlways(t *testing.T) {
    a := ApprovalRequest{ToolName: "Bash", Detail: "rm -rf /tmp"}
    result := a.HandleKey("a")
    if result != ApprovalAlways {
        t.Errorf("expected Always, got %d", result)
    }
}

func TestApprovalParseEsc(t *testing.T) {
    a := ApprovalRequest{ToolName: "Bash", Detail: "rm -rf /tmp"}
    result := a.HandleKey("esc")
    if result != ApprovalCancel {
        t.Errorf("expected Cancel, got %d", result)
    }
}

func TestApprovalParsePending(t *testing.T) {
    a := ApprovalRequest{ToolName: "Bash", Detail: "rm -rf /tmp"}
    result := a.HandleKey("x")
    if result != ApprovalPending {
        t.Errorf("expected Pending, got %d", result)
    }
}

func TestApprovalRender(t *testing.T) {
    th := LoadTheme("tokyo-night")
    a := ApprovalRequest{ToolName: "Bash", Detail: "rm -rf /tmp"}
    rendered := a.Render(th)
    if rendered == "" {
        t.Error("expected non-empty render")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestApproval`
Expected: FAIL.

- [ ] **Step 3: Implement approval.go**

```go
package tui

import (
    "fmt"

    "github.com/charmbracelet/lipgloss/v2"
)

// ApprovalResult represents the user's decision on a tool approval request.
type ApprovalResult int

const (
    ApprovalPending ApprovalResult = iota
    ApprovalAllow
    ApprovalDeny
    ApprovalAlways
    ApprovalCancel
)

// ApprovalRequest represents a pending tool approval prompt.
type ApprovalRequest struct {
    ToolName string
    Detail   string
}

// HandleKey processes a key press and returns the approval result.
func (a ApprovalRequest) HandleKey(key string) ApprovalResult {
    switch key {
    case "y", "Y":
        return ApprovalAllow
    case "n", "N":
        return ApprovalDeny
    case "a", "A":
        return ApprovalAlways
    case "esc":
        return ApprovalCancel
    default:
        return ApprovalPending
    }
}

// Render produces the approval popup display string.
func (a ApprovalRequest) Render(th Theme) string {
    styles := th.Styles()
    warn := styles.Warning.Render("⚠ Approval Required")
    detail := styles.BashPrefix.Render("$ ") + styles.UserText.Render(a.Detail)
    allowBtn := lipgloss.NewStyle().
        Background(lipgloss.Color(th.Green)).
        Foreground(lipgloss.Color(th.BG)).
        Padding(0, 1).
        Render("Y 允许")
    denyBtn := lipgloss.NewStyle().
        Background(lipgloss.Color(th.Red)).
        Foreground(lipgloss.Color(th.BG)).
        Padding(0, 1).
        Render("N 拒绝")
    alwaysBtn := lipgloss.NewStyle().
        Background(lipgloss.Color(th.Border)).
        Foreground(lipgloss.Color(th.Text)).
        Padding(0, 1).
        Render("A 总是允许")
    escBtn := styles.Muted.Render("Esc 取消")

    return fmt.Sprintf("%s\n%s\n%s %s %s %s", warn, detail, allowBtn, denyBtn, alwaysBtn, escBtn)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -run TestApproval -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/approval.go internal/tui/approval_test.go
git commit -m "feat(tui): add tool approval popup component"
```

---

### Task 13: App Model (Bubble Tea Integration)

**Files:**
- Create: `internal/tui/app.go`

- [ ] **Step 1: Implement app.go — Bubble Tea top-level model**

This is the largest file. It wires all components together and implements the `tea.Model` interface.

```go
package tui

import (
    "context"
    "strings"
    "time"

    "github.com/charmbracelet/bubbletea/v2"
    "github.com/charmbracelet/lipgloss/v2"
    "github.com/openclaw/gclaw/internal/agent"
    "github.com/openclaw/gclaw/internal/config"
)

// Deps holds runtime dependencies injected from main.go.
type Deps struct {
    Config   *config.Config
    Agent    *agent.Agent
    Theme    Theme
    LogBuf   *LogBuffer
    History  *History
    CompEng  *CompletionEngine
    OnSubmit func(ctx context.Context, input string, images []string) (string, error)
    OnSlash  func(cmd string)
}

// App is the top-level Bubble Tea model.
type App struct {
    deps       Deps
    theme      Theme
    styles     Styles
    transcript *Transcript
    composer   *Composer
    statusbar  *StatusBar
    approval   *ApprovalRequest

    // State
    width      int
    height     int
    busy       bool
    startTime  time.Time
}

// NewApp creates the TUI app model.
func NewApp(deps Deps) *App {
    th := deps.Theme
    styles := th.Styles()
    return &App{
        deps:       deps,
        theme:      th,
        styles:     styles,
        transcript: NewTranscript(styles),
        composer:   NewComposer(deps.CompEng, deps.History),
        statusbar:  NewStatusBar(th),
        startTime:  time.Now(),
    }
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
    return tickCmd()
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch m := msg.(type) {
    case tea.WindowSizeMsg:
        a.width = m.Width
        a.height = m.Height
        a.transcript.Resize(a.width, a.height-6) // reserve for composer
        return a, nil

    case tea.KeyPressMsg:
        return a.handleKey(m)

    case tea.PasteMsg:
        a.composer.SetInput(a.composer.Text() + m.Text)
        return a, nil

    case agentResponseMsg:
        a.busy = false
        a.statusbar.SetState("ready")
        if m.err != nil {
            a.transcript.Append(TranscriptMsg{
                Kind:    MsgAssistant,
                Content: "Error: " + m.err.Error(),
            })
        } else {
            a.transcript.Append(TranscriptMsg{
                Kind:    MsgAssistant,
                Content: m.text,
            })
        }
        return a, nil

    case tickMsg:
        a.statusbar.SetElapsed(time.Since(a.startTime).Truncate(time.Second).String())
        return a, tickCmd()
    }

    return a, nil
}

// View implements tea.Model.
func (a *App) View() string {
    if a.width == 0 {
        return "Loading..."
    }

    // Transcript
    transcriptView := a.transcript.Render()

    // Divider
    divider := lipgloss.NewStyle().
        Foreground(lipgloss.Color(a.theme.Border)).
        Render(strings.Repeat("─", a.width))

    // Status bar
    statusView := a.statusbar.Render(a.width)

    // Composer area
    var composerView string
    if a.approval != nil {
        composerView = a.approval.Render(a.theme)
    } else {
        composerView = a.renderComposer()
    }

    return lipgloss.JoinVertical(
        lipgloss.Left,
        transcriptView,
        divider,
        statusView,
        composerView,
    )
}

func (a *App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
    // Approval mode takes priority
    if a.approval != nil {
        result := a.approval.HandleKey(msg.String())
        if result != ApprovalPending {
            // TODO: wire result back to agent permission system
            a.approval = nil
        }
        return a, nil
    }

    switch msg.String() {
    case KeyCtrlC:
        if a.composer.IsEmpty() {
            // Interrupt agent
            a.deps.Agent.Interrupt("user interrupt")
            return a, nil
        }
        a.composer.Clear()
        return a, nil

    case KeyCtrlL:
        a.transcript = NewTranscript(a.styles)
        return a, nil

    case KeyEsc:
        a.composer.Clear()
        return a, nil

    case KeyEnter:
        if a.busy {
            // Queue message (TODO)
            return a, nil
        }
        return a.submitInput()

    case KeyCtrlEnter:
        a.composer.InsertNewLine()
        return a, nil

    case KeyBackspace:
        a.composer.Backspace()
        return a, nil

    case KeyDelete:
        a.composer.Delete()
        return a, nil

    case KeyLeft:
        a.composer.MoveLeft()
        return a, nil

    case KeyRight:
        a.composer.MoveRight()
        return a, nil

    case KeyHome:
        a.composer.MoveHome()
        return a, nil

    case KeyEnd:
        a.composer.MoveEnd()
        return a, nil

    case KeyUp:
        if a.deps.History != nil {
            entry := a.deps.History.Older()
            if entry != "" {
                a.composer.SetInput(entry)
            }
        }
        return a, nil

    case KeyDown:
        if a.deps.History != nil {
            entry := a.deps.History.Newer()
            a.composer.SetInput(entry)
        }
        return a, nil

    case KeyTab:
        // Apply completion (TODO: completion integration)
        return a, nil

    case KeyPgUp:
        a.transcript.ScrollUp(a.transcript.height)
        return a, nil

    case KeyPgDown:
        a.transcript.ScrollDown(a.transcript.height)
        return a, nil

    case KeyCtrlO:
        // Toggle thinking fold (TODO)
        return a, nil

    default:
        // Rune input
        if len(msg.Runes) > 0 {
            for _, r := range msg.Runes {
                a.composer.InsertRune(r)
            }
        }
        return a, nil
    }
}

func (a *App) submitInput() (tea.Model, tea.Cmd) {
    text := a.composer.Text()
    if text == "" {
        return a, nil
    }

    // Slash command
    if strings.HasPrefix(text, "/") {
        if a.deps.OnSlash != nil {
            a.deps.OnSlash(text)
        }
        a.composer.Clear()
        return a, nil
    }

    // Add to history
    if a.deps.History != nil {
        a.deps.History.Add(text)
    }

    // Add user message to transcript
    a.transcript.Append(TranscriptMsg{
        Kind:    MsgUser,
        Content: text,
    })

    // Collect image attachments
    var images []string
    for _, att := range a.composer.Attachments() {
        if att.IsImage {
            images = append(images, att.Path)
        }
    }

    // Clear composer
    a.composer.Clear()

    // Run agent in background
    a.busy = true
    a.statusbar.SetState("busy")
    return a, runAgentCmd(a.deps, text, images)
}

func (a *App) renderComposer() string {
    var parts []string

    // Queue preview (when busy)
    if a.busy {
        parts = append(parts, a.styles.Warning.Render("⏳ agent busy, message queued"))
    }

    // Completion dropdown (if active)
    // TODO: render completion items

    // Input lines
    text := a.composer.Text()
    if text == "" {
        parts = append(parts, a.styles.Prompt.Render("❯ ")+"_|")
    } else {
        lines := strings.Split(text, "\n")
        for i, line := range lines {
            if i == 0 {
                parts = append(parts, a.styles.Prompt.Render("❯ ")+line)
            } else {
                parts = append(parts, "  "+line)
            }
        }
        parts[len(parts)-1] += "▌"
    }

    // Attachments
    for i, att := range a.composer.Attachments() {
        icon := "📎"
        if att.IsImage {
            icon = "🖼"
        }
        tag := a.styles.EventPrefix.Render(icon+" "+att.Path) +
            a.styles.Error.Render(" ✕")
        parts = append(parts, tag)
        _ = i
    }

    // Help hint
    parts = append(parts, a.styles.Muted.Render("Ctrl+Enter: 换行 · Enter: 发送 · Ctrl+V: 粘贴 · Esc: 取消"))

    return strings.Join(parts, "\n")
}

// Messages for async communication

type agentResponseMsg struct {
    text string
    err  error
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
    return tea.Tick(time.Second, func(t time.Time) tea.Msg {
        return tickMsg(t)
    })
}

func runAgentCmd(deps Deps, input string, images []string) tea.Cmd {
    return func() tea.Msg {
        if deps.OnSubmit != nil {
            text, err := deps.OnSubmit(context.Background(), input, images)
            return agentResponseMsg{text: text, err: err}
        }
        return agentResponseMsg{text: "", err: nil}
    }
}
```

- [ ] **Step 2: Run build check**

Run: `go build ./internal/tui/`

Fix any compile errors (e.g., `tea.KeyPressMsg` vs `tea.KeyMsg` — check Bubble Tea v2 API). Adjust method signatures to match the actual v2 API.

- [ ] **Step 3: Fix and rebuild until clean**

Run: `go build ./internal/tui/`
Expected: Success.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/app.go
git commit -m "feat(tui): add Bubble Tea app model with agent integration"
```

---

### Task 14: Wire into main.go

**Files:**
- Modify: `cmd/gclaw/main.go`

- [ ] **Step 1: Add TUI import and initialization**

At the top of `cmd/gclaw/main.go`, add import:

```go
"github.com/openclaw/gclaw/internal/tui"
```

- [ ] **Step 2: Add TUI setup function**

Add after `setupLogging`:

```go
func setupTUI(cfg *config.Config) (*tui.LogBuffer, *tui.History, *tui.CompletionEngine) {
    logBuf := tui.NewLogBuffer(cfg.TUI.Log.BufferSize)
    slog.SetDefault(slog.New(logBuf.SlogHandler()))

    histPath := filepath.Join(config.HomeDir(), ".gclaw", "history")
    hist := tui.NewHistory(histPath, cfg.TUI.History.MaxEntries)

    comp := tui.DefaultCompletionEngine()
    return logBuf, hist, comp
}
```

- [ ] **Step 3: Replace the REPL loop in runREPL**

In `runREPL()`, after all existing setup (agent creation, channel init, etc.), replace the `for` loop (lines 563-634) and the `scanner` creation (line 532):

Remove:
```go
scanner := bufio.NewScanner(os.Stdin)
```

Remove the `clarifypkg.Callback` block (lines 535-561).

Add before the removed scanner line:
```go
// TUI setup
logBuf, hist, compEng := setupTUI(cfg)

deps := tui.Deps{
    Config:  cfg,
    Agent:   ag,
    Theme:   tui.LoadTheme(cfg.TUI.Theme),
    LogBuf:  logBuf,
    History: hist,
    CompEng: compEng,
    OnSubmit: func(ctx context.Context, input string, images []string) (string, error) {
        agentInput := input
        if len(images) > 0 {
            agentInput = input + "\n[images attached: " + strings.Join(images, ", ") + "]"
        }
        return ag.Run(ctx, agentInput)
    },
    OnSlash: func(cmd string) {
        handleCommand(cmd, &cmdCtx{...}) // pass existing cmdCtx fields
    },
}

app := tui.NewApp(deps)
p := tea.NewProgram(app, tea.WithAltScreen())
if _, err := p.Run(); err != nil {
    log.Fatal("TUI error", "error", err)
}
// Save history on exit
hist.Save()
return
```

- [ ] **Step 4: Build and test**

Run: `go build -o gclaw.exe ./cmd/gclaw/`
Expected: Build succeeds.

Run: `./gclaw.exe`
Expected: TUI starts with Bubble Tea, shows banner, accepts input.

- [ ] **Step 5: Commit**

```bash
git add cmd/gclaw/main.go
git commit -m "feat(tui): wire Bubble Tea TUI into main.go, replacing bufio.Scanner REPL"
```

---

### Task 15: Streaming, Events, and Tool Callbacks

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `cmd/gclaw/main.go`

- [ ] **Step 1: Add streaming support to app.go**

Add a `streamChunkMsg` type and handle it in `Update`:

```go
type streamChunkMsg struct {
    text string
}

// In Update(), add case:
case streamChunkMsg:
    // Update last assistant message with new chunk
    msgs := a.transcript.Messages()
    if len(msgs) > 0 && msgs[len(msgs)-1].Kind == MsgAssistant {
        msgs[len(msgs)-1].Content += m.text
        a.transcript.UpdateLast(msgs[len(msgs)-1])
    } else {
        a.transcript.Append(TranscriptMsg{
            Kind:    MsgAssistant,
            Content: m.text,
        })
    }
    return a, nil
```

- [ ] **Step 2: Update OnSubmit in main.go to use RunStreaming**

Replace the `OnSubmit` function:

```go
OnSubmit: func(ctx context.Context, input string, images []string) (string, error) {
    agentInput := input
    if len(images) > 0 {
        agentInput = input + "\n[images attached: " + strings.Join(images, ", ") + "]"
    }
    return ag.RunStreaming(ctx, agentInput, func(chunk string) {
        p.Send(streamChunkMsg{text: chunk})
    })
},
```

This requires `p` (the `tea.Program`) to be accessible — store it in a variable before passing to `deps`.

- [ ] **Step 3: Add async event forwarding**

In `runREPL`, after creating the program, start a goroutine that forwards weixin/cron events:

```go
// Forward async events to TUI
go func() {
    if weixinCh != nil {
        for msg := range weixinCh.Messages() {
            p.Send(tui.EventMsg{
                Icon:    "📬",
                Source:  "weixin",
                Content: msg.From + ": " + msg.Text,
            })
        }
    }
}()
```

Add `EventMsg` to app.go:

```go
type EventMsg struct {
    Icon    string
    Source  string
    Content string
}
```

Handle in `Update`:

```go
case EventMsg:
    a.transcript.Append(TranscriptMsg{
        Kind:      MsgEvent,
        EventIcon: m.Icon,
        EventSrc:  m.Source,
        Content:   m.Content,
    })
    return a, nil
```

- [ ] **Step 4: Add tool call forwarding**

Add a tool callback mechanism. In the agent's tool execution, send tool call info to the TUI:

```go
type toolCallMsg struct {
    tool ToolCall
}

case toolCallMsg:
    a.transcript.Append(TranscriptMsg{
        Kind: MsgToolCall,
        Tool: &m.tool,
    })
    return a, nil
```

Wire this in `main.go` by setting a tool execution callback on the agent (requires adding a callback field to `agent.Config`).

- [ ] **Step 5: Build and manual test**

Run: `go build -o gclaw.exe ./cmd/gclaw/`
Expected: Build succeeds.

Test: Run gclaw, type a message, verify streaming output appears in transcript.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/app.go cmd/gclaw/main.go
git commit -m "feat(tui): add streaming output, async events, and tool call forwarding"
```

---

### Task 16: Final Integration Testing

**Files:**
- All TUI files

- [ ] **Step 1: Run all tests**

Run: `go test ./...`
Expected: All pass (including new TUI tests).

- [ ] **Step 2: Build release binary**

Run: `go build -o gclaw.exe ./cmd/gclaw/`
Expected: Success.

- [ ] **Step 3: Manual smoke test checklist**

Test the following interactions:
- [ ] Type text and press Enter — message appears in transcript, agent responds
- [ ] Ctrl+Enter — new line inserted in composer
- [ ] Backspace at line start — merges with previous line
- [ ] ↑ / ↓ — navigate history
- [ ] Type `/` — completion dropdown appears
- [ ] Tab — apply completion
- [ ] Ctrl+C with text — clear input
- [ ] Ctrl+L — clear transcript
- [ ] `/logs` — show log buffer in transcript
- [ ] `/status` — show status panel in transcript
- [ ] `/theme catppuccin-mocha` — switch theme
- [ ] `/exit` — exit cleanly

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat(tui): complete TUI redesign with Bubble Tea"
```
