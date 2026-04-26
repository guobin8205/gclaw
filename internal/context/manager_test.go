package context

import (
	"strings"
	"testing"

	"github.com/openclaw/gclaw/internal/model"
)

func TestManagerTokenCount(t *testing.T) {
	m := NewManager(Config{
		MaxTokens:    10000,
		CompactAt:    0.85,
		ReserveRatio: 0.15,
		SystemPrompt: "You are a helpful assistant.",
	})

	if m.TokenCount() <= 0 {
		t.Error("token count should include system prompt")
	}

	m.AddMessage(model.Message{Role: "user", Content: "Hello world"})
	m.AddMessage(model.Message{Role: "assistant", Content: "Hi there!"})

	count := m.TokenCount()
	if count <= 0 {
		t.Error("token count should be positive after adding messages")
	}
}

func TestManagerShouldCompact(t *testing.T) {
	// Small max tokens so we hit the threshold quickly
	m := NewManager(Config{
		MaxTokens:    500,
		CompactAt:    0.5,
		ReserveRatio: 0.15,
		SystemPrompt: "You are a helpful assistant.",
	})

	// Add enough messages to exceed 50% usage
	longText := strings.Repeat("a", 2000)
	m.AddMessage(model.Message{Role: "user", Content: longText})

	if !m.ShouldCompact() {
		t.Error("should trigger compaction")
	}

	m2 := NewManager(Config{
		MaxTokens:    1000000,
		CompactAt:    0.85,
		ReserveRatio: 0.15,
		SystemPrompt: "Short.",
	})

	if m2.ShouldCompact() {
		t.Error("should not trigger compaction with few tokens")
	}
}

func TestManagerCompact(t *testing.T) {
	m := NewManager(Config{
		MaxTokens:    10000,
		CompactAt:    0.5,
		ReserveRatio: 0.15,
		SystemPrompt: "System prompt.",
	})

	for i := 0; i < 50; i++ {
		m.AddMessage(model.Message{Role: "user", Content: strings.Repeat("x", 100)})
		m.AddMessage(model.Message{Role: "assistant", Content: strings.Repeat("y", 100)})
	}

	before := len(m.GetMessages())
	if before != 100 {
		t.Fatalf("expected 100 messages, got %d", before)
	}

	m.Compact(5)
	after := len(m.GetMessages())

	if after >= before {
		t.Errorf("expected fewer messages after compaction: %d -> %d", before, after)
	}

	// Check boundary marker exists
	msgs := m.GetMessages()
	found := false
	for _, msg := range msgs {
		if msg.Role == "user" && strings.Contains(msg.Content, "compacted") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected compaction boundary marker in messages")
	}
}

func TestManagerCompactNoop(t *testing.T) {
	m := NewManager(Config{
		MaxTokens:    10000,
		CompactAt:    0.5,
		ReserveRatio: 0.15,
		SystemPrompt: "System.",
	})

	// Only 3 messages, keepRecent=5 -> should not compact
	m.AddMessage(model.Message{Role: "user", Content: "hi"})
	m.AddMessage(model.Message{Role: "assistant", Content: "hello"})

	before := len(m.GetMessages())
	m.Compact(5)
	after := len(m.GetMessages())

	if before != after {
		t.Errorf("should not compact few messages: %d -> %d", before, after)
	}
}

func TestManagerUsageRatio(t *testing.T) {
	m := NewManager(Config{
		MaxTokens:    100000,
		CompactAt:    0.85,
		ReserveRatio: 0.15,
		SystemPrompt: "Test prompt.",
	})

	ratio := m.UsageRatio()
	if ratio == "" || ratio == "N/A" {
		t.Error("usage ratio should not be empty")
	}
}

func TestManagerRemainingBudget(t *testing.T) {
	m := NewManager(Config{
		MaxTokens:    10000,
		CompactAt:    0.85,
		ReserveRatio: 0.15,
		SystemPrompt: "Short.",
	})

	budget := m.RemainingBudget()
	if budget <= 0 {
		t.Error("budget should be positive")
	}
	if budget > 10000 {
		t.Errorf("budget %d exceeds max tokens", budget)
	}
}

func TestManagerUsageStats(t *testing.T) {
	m := NewManager(Config{
		MaxTokens:    10000,
		CompactAt:    0.85,
		ReserveRatio: 0.15,
		SystemPrompt: "System prompt.",
	})

	m.AddMessage(model.Message{Role: "user", Content: "test message"})
	m.Snapshot(1)

	stats := m.UsageStats()
	if stats == "" {
		t.Error("stats should not be empty")
	}
	if !strings.Contains(stats, "Messages:") {
		t.Error("stats should include Messages count")
	}
	if !strings.Contains(stats, "Tokens:") {
		t.Error("stats should include Tokens count")
	}
	if !strings.Contains(stats, "Compact threshold:") {
		t.Error("stats should include Compact threshold")
	}
}

func TestManagerReset(t *testing.T) {
	m := NewManager(Config{
		MaxTokens:    10000,
		CompactAt:    0.85,
		ReserveRatio: 0.15,
		SystemPrompt: "Test.",
	})

	m.AddMessage(model.Message{Role: "user", Content: "test"})
	m.Snapshot(1)

	m.Reset()

	if len(m.GetMessages()) != 0 {
		t.Error("messages should be empty after reset")
	}
	if len(m.GetStats()) != 0 {
		t.Error("stats should be empty after reset")
	}
}

func TestManagerSnapshot(t *testing.T) {
	m := NewManager(Config{
		MaxTokens:    10000,
		SystemPrompt: "System.",
	})

	m.Snapshot(1)
	m.Snapshot(2)
	m.Snapshot(3)

	stats := m.GetStats()
	if len(stats) != 3 {
		t.Errorf("expected 3 snapshots, got %d", len(stats))
	}
	if stats[0].TurnCount != 1 {
		t.Errorf("expected turn 1, got %d", stats[0].TurnCount)
	}
	if stats[2].TurnCount != 3 {
		t.Errorf("expected turn 3, got %d", stats[2].TurnCount)
	}
}
