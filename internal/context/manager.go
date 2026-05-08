package context

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/openclaw/gclaw/internal/model"
)

// Config holds context window configuration.
type Config struct {
	MaxTokens    int
	CompactAt    float64 // 0.0 - 1.0, trigger compact at this ratio
	ReserveRatio float64 // reserve for response
	SystemPrompt string
}

// Snapshot records context state at a point in time.
type Snapshot struct {
	Time       time.Time
	TotalMessages int
	TokenCount int
	TurnCount  int
}

// Manager handles context window management and compaction.
type Manager struct {
	cfg      Config
	messages []model.Message
	stats    []Snapshot
	mu       sync.RWMutex

	// LLM compression support
	compactor         Compactor
	previousSummary   string
	compressionHistory []CompressionResult

	// Actual token tracking from API responses
	actualInputTokens int
	actualMsgIndex    int
}

// NewManager creates a context manager.
func NewManager(cfg Config) *Manager {
	return &Manager{
		cfg:      cfg,
		messages: nil,
		stats:    nil,
	}
}

// NewManagerWithCompactor creates a context manager with LLM-powered compression.
func NewManagerWithCompactor(cfg Config, c Compactor) *Manager {
	return &Manager{
		cfg:       cfg,
		compactor: c,
	}
}

// AddMessage appends a message and returns the updated token count.
func (m *Manager) AddMessage(msg model.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
}

// GetMessages returns a copy of all messages.
func (m *Manager) GetMessages() []model.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make([]model.Message, len(m.messages))
	copy(cp, m.messages)
	return cp
}

// TokenCount estimates the current token usage.
// When actual API token data is available, uses that as the base
// and estimates only the delta for messages added since.
func (m *Manager) TokenCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tokenCountLocked()
}

func (m *Manager) tokenCountLocked() int {
	if m.actualInputTokens > 0 && m.actualMsgIndex <= len(m.messages) {
		base := m.actualInputTokens
		delta := 0
		for i := m.actualMsgIndex; i < len(m.messages); i++ {
			delta += len(m.messages[i].Content) / 2
		}
		return base + delta
	}
	// Fallback: char/2 estimation (more conservative than /4)
	count := len(m.cfg.SystemPrompt) / 2
	for _, msg := range m.messages {
		count += len(msg.Content) / 2
		if msg.ToolID != "" {
			count += len(msg.ToolID) / 2
		}
	}
	return count
}

// SetActualTokens records the real input token count from an API response.
func (m *Manager) SetActualTokens(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actualInputTokens = count
	m.actualMsgIndex = len(m.messages)
}

// ShouldCompact returns true when compaction should be triggered.
func (m *Manager) ShouldCompact() bool {
	if m.cfg.MaxTokens == 0 {
		return false
	}
	m.mu.RLock()
	count := m.tokenCountLocked()
	m.mu.RUnlock()
	ratio := float64(count) / float64(m.cfg.MaxTokens)
	return ratio >= m.cfg.CompactAt
}

// UsageRatio returns current token usage as a percentage string.
func (m *Manager) UsageRatio() string {
	if m.cfg.MaxTokens == 0 {
		return "N/A"
	}
	ratio := float64(m.TokenCount()) / float64(m.cfg.MaxTokens) * 100
	return fmt.Sprintf("%.1f%% (%d/%d)", ratio, m.TokenCount(), m.cfg.MaxTokens)
}

// Compact performs context compaction to reduce token usage.
// If an LLM compactor is configured, uses it for intelligent summarization.
// Otherwise falls back to truncation (keep first + last N messages).
func (m *Manager) Compact(keepRecent int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.compactor != nil {
		m.compactWithLLM()
	} else {
		m.compactTruncation(keepRecent)
	}
}

// compactWithLLM uses the auxiliary model to compress conversation history.
func (m *Manager) compactWithLLM() {
	// Anti-thrashing: skip if last 2 compressions saved < 10%
	if len(m.compressionHistory) >= 2 {
		lastTwo := m.compressionHistory[len(m.compressionHistory)-2:]
		for _, r := range lastTwo {
			if r.MessagesBefore > 0 {
				ratio := float64(r.TokensSaved) / float64(estimateTokensList(m.messages))
				if ratio >= 0.10 {
					goto proceed
				}
			}
		}
		slog.Debug("skipping compression: anti-thrashing (last 2 saved < 10%)")
		return
	}

proceed:
	result, err := m.compactor.Compact(m.messages, m.previousSummary)
	if err != nil {
		slog.Warn("LLM compression failed, falling back to truncation", "error", err)
		m.compactTruncation(10)
		return
	}
	if result == nil {
		return
	}

	// Rebuild messages: keepFirst + summary + keepRecent
	keepFirst := 1
	keepRecent := 20
	if len(m.messages) <= keepFirst+keepRecent {
		return
	}

	var newMessages []model.Message
	newMessages = append(newMessages, m.messages[:keepFirst]...)
	newMessages = append(newMessages, model.Message{
		Role:    "user",
		Content: fmt.Sprintf("[Conversation Summary]\n%s", result.Summary),
	})
tail := m.messages[len(m.messages)-keepRecent:]
	for i := range tail {
		if tail[i].Role == "tool" && len(tail[i].Content) > 200 {
			tail[i].Content = pruneToolOutput(tail[i].Content)
		}
	}
	newMessages = append(newMessages, tail...)

	m.messages = newMessages
	m.previousSummary = result.Summary
	m.compressionHistory = append(m.compressionHistory, *result)

	slog.Info("context compressed via LLM",
		"messages_before", result.MessagesBefore,
		"messages_after", len(newMessages),
		"tokens_saved", result.TokensSaved,
	)
}

// compactTruncation is the legacy truncation strategy: keep first + last N messages.
// Tool outputs >200 chars in kept messages are pruned to 1-line summaries.
func (m *Manager) compactTruncation(keepRecent int) {
	if len(m.messages) <= keepRecent*2 {
		return
	}

	first := m.messages[0]
	recent := make([]model.Message, keepRecent)
	copy(recent, m.messages[len(m.messages)-keepRecent:])

	// Prune large tool outputs in kept messages
	for i := range recent {
		if recent[i].Role == "tool" && len(recent[i].Content) > 200 {
			recent[i].Content = pruneToolOutput(recent[i].Content)
		}
	}

	boundary := model.Message{
		Role:    "user",
		Content: fmt.Sprintf("[Earlier messages compacted. %d messages removed to stay within context budget.]", len(m.messages)-keepRecent-1),
	}

	newMessages := []model.Message{first, boundary}
	newMessages = append(newMessages, recent...)
	m.messages = newMessages
}

// pruneToolOutput reduces a large tool output to a 1-line summary.
func pruneToolOutput(content string) string {
	lines := strings.Split(content, "\n")
	n := len(lines)
	first := lines[0]
	if len(first) > 120 {
		first = first[:120] + "..."
	}
	return fmt.Sprintf("[%d lines output] %s", n, first)
}

func estimateTokensList(messages []model.Message) int {
	count := 0
	for _, msg := range messages {
		count += len(msg.Content) / 4
	}
	return count
}

// UsageStats returns a formatted string with context usage stats.
func (m *Manager) UsageStats() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Messages: %d\n", len(m.messages)))
	sb.WriteString(fmt.Sprintf("Tokens: %d / %d (%.1f%%)\n",
		m.TokenCount(), m.cfg.MaxTokens,
		float64(m.TokenCount())/float64(m.cfg.MaxTokens)*100))
	sb.WriteString(fmt.Sprintf("Compact threshold: %.0f%%\n", m.cfg.CompactAt*100))
	sb.WriteString(fmt.Sprintf("Reserve ratio: %.0f%%\n", m.cfg.ReserveRatio*100))

	if len(m.stats) > 0 {
		last := m.stats[len(m.stats)-1]
		sb.WriteString(fmt.Sprintf("Last snapshot: %s\n", last.Time.Format("15:04:05")))
	}

	return sb.String()
}

// Snapshot records current stats.
func (m *Manager) Snapshot(turnCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := m.tokenCountLocked()

	m.stats = append(m.stats, Snapshot{
		Time:          time.Now(),
		TotalMessages: len(m.messages),
		TokenCount:    count,
		TurnCount:     turnCount,
	})
}

// GetStats returns the stats history.
func (m *Manager) GetStats() []Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make([]Snapshot, len(m.stats))
	copy(cp, m.stats)
	return cp
}

// Reset clears all messages and stats.
func (m *Manager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = nil
	m.stats = nil
	m.previousSummary = ""
	m.compressionHistory = nil
	m.actualInputTokens = 0
	m.actualMsgIndex = 0
}

// RemainingBudget returns the available token budget.
func (m *Manager) RemainingBudget() int {
	budget := m.cfg.MaxTokens - int(float64(m.cfg.MaxTokens)*m.cfg.ReserveRatio)
	used := m.TokenCount()
	if used >= budget {
		return 0
	}
	return budget - used
}
