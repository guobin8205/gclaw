package context

import (
	"context"
	"strings"
	"testing"

	"github.com/openclaw/gclaw/internal/model"
)

// --- LLM 压缩集成验证 ---

// mockCompressorModel 返回固定的结构化摘要
type mockCompressorModel struct{}

func (m *mockCompressorModel) ID() string                         { return "mock-compressor" }
func (m *mockCompressorModel) MaxTokens() int                     { return 4096 }
func (m *mockCompressorModel) SupportsThinking() bool             { return false }
func (m *mockCompressorModel) SupportsStreaming() bool            { return false }
func (m *mockCompressorModel) SupportsVision() bool               { return false }
func (m *mockCompressorModel) CountTokens(_ []model.Message) int { return 100 }
func (m *mockCompressorModel) Stream(_ context.Context, _ model.StreamParams) (<-chan model.StreamEvent, error) {
	return nil, nil
}

func (m *mockCompressorModel) Call(_ context.Context, params model.CallParams) (*model.Response, error) {
	return &model.Response{
		Text: "## Active Task\nImplementing feature X\n## Completed Actions\n1. Read file /tmp/test.txt\n## Active State\nWorking in /home/user/project\n## In Progress\nEditing config.yaml\n## Blocked\nNone\n## Key Decisions\nUsing SQLite for storage\n## Pending Items\nWrite tests\n## Critical Context\nAPI endpoint: /v1/chat",
		Usage: model.Usage{InputTokens: 200, OutputTokens: 100},
	}, nil
}

func TestCompressionIntegration_ShouldCompactTriggersCompression(t *testing.T) {
	compactor := NewLLMCompactor(&mockCompressorModel{}, 1, 2, 2048)

	cfg := Config{
		MaxTokens:    200, // 很小的窗口，容易触发
		CompactAt:    0.8,
		ReserveRatio: 0.15,
	}
	mgr := NewManagerWithCompactor(cfg, compactor)

	// 添加足够多的消息以触发压缩
	for i := 0; i < 30; i++ {
		msg := model.Message{
			Role:    "user",
			Content: strings.Repeat("This is message content that takes up tokens. ", 10),
		}
		mgr.AddMessage(msg)
	}

	t.Logf("Before compact: messages=%d, tokens=%d, should=%v",
		len(mgr.GetMessages()), mgr.TokenCount(), mgr.ShouldCompact())

	if !mgr.ShouldCompact() {
		t.Fatal("expected ShouldCompact to be true")
	}

	// 触发压缩
	mgr.Compact(2)

	msgs := mgr.GetMessages()
	t.Logf("After compact: messages=%d, tokens=%d", len(msgs), mgr.TokenCount())

	// LLM 压缩固定 keepFirst=1, keepRecent=10: 1 + 1 (summary) + 10 = 12
	if len(msgs) != 12 {
		t.Errorf("expected 12 messages after compression (1+1+10), got %d", len(msgs))
	}

	// 第一条是原始消息
	if msgs[0].Role != "user" {
		t.Errorf("expected first msg role=user, got %s", msgs[0].Role)
	}

	// 第二条是摘要
	if !strings.Contains(msgs[1].Content, "[Conversation Summary]") {
		t.Errorf("expected summary marker, got: %q", truncate(msgs[1].Content, 80))
	}
	if !strings.Contains(msgs[1].Content, "Active Task") {
		t.Error("expected structured summary with Active Task section")
	}
	if !strings.Contains(msgs[1].Content, "Pending Items") {
		t.Error("expected structured summary with Pending Items section")
	}

	// 最后 10 条是最近消息
	for i := 2; i <= 11; i++ {
		if msgs[i].Role != "user" {
			t.Errorf("expected recent msg[%d] role=user, got %s", i, msgs[i].Role)
		}
	}

	t.Logf("Summary content: %q", truncate(msgs[1].Content, 200))
}

func TestCompressionIntegration_FallbackToTruncation(t *testing.T) {
	// 模拟压缩模型失败时回退到截断
	failingCompactor := &failingCompactor{}
	cfg := Config{
		MaxTokens:    200,
		CompactAt:    0.8,
		ReserveRatio: 0.15,
	}
	mgr := NewManagerWithCompactor(cfg, failingCompactor)

	for i := 0; i < 30; i++ {
		mgr.AddMessage(model.Message{
			Role:    "user",
			Content: strings.Repeat("Content here. ", 10),
		})
	}

	mgr.Compact(2)

	msgs := mgr.GetMessages()
	// 截断模式也是 keepRecent=10: 1 (first) + 1 (boundary) + 10 (recent) = 12
	if len(msgs) != 12 {
		t.Errorf("expected 12 messages after truncation fallback, got %d", len(msgs))
	}

	// 边界标记（不是摘要）
	if !strings.Contains(msgs[1].Content, "compacted") {
		t.Errorf("expected boundary marker, got: %q", msgs[1].Content)
	}
}

type failingCompactor struct{}

func (f *failingCompactor) Compact(_ []model.Message, _ string) (*CompressionResult, error) {
	return nil, &compressionError{"model unavailable"}
}

type compressionError struct{ msg string }

func (e *compressionError) Error() string { return e.msg }

func TestCompressionIntegration_NoCompactorUsesTruncation(t *testing.T) {
	cfg := Config{
		MaxTokens:    200,
		CompactAt:    0.8,
		ReserveRatio: 0.15,
	}
	mgr := NewManager(cfg)

	for i := 0; i < 30; i++ {
		mgr.AddMessage(model.Message{
			Role:    "user",
			Content: strings.Repeat("Content. ", 10),
		})
	}

	mgr.Compact(2)

	msgs := mgr.GetMessages()
	if len(msgs) != 4 {
		t.Errorf("expected 4 messages from truncation, got %d", len(msgs))
	}
}

func TestCompressionIntegration_ResetClearsSummary(t *testing.T) {
	compactor := NewLLMCompactor(&mockCompressorModel{}, 1, 2, 2048)
	cfg := Config{
		MaxTokens:    200,
		CompactAt:    0.8,
		ReserveRatio: 0.15,
	}
	mgr := NewManagerWithCompactor(cfg, compactor)

	for i := 0; i < 30; i++ {
		mgr.AddMessage(model.Message{Role: "user", Content: strings.Repeat("Content. ", 10)})
	}

	mgr.Compact(2)
	if mgr.previousSummary == "" {
		t.Error("expected summary after compression")
	}

	mgr.Reset()
	if mgr.previousSummary != "" {
		t.Error("expected summary to be cleared after reset")
	}
	if len(mgr.compressionHistory) != 0 {
		t.Error("expected compression history to be cleared after reset")
	}
}

// 为了避免和 compactor_test.go 中的 message 冲突，这里直接用 model.Message
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
