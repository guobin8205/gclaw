package agent

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openclaw/gclaw/internal/model"
	"github.com/openclaw/gclaw/internal/tool"
	"github.com/openclaw/gclaw/internal/tool/builtin/file_read"
)

// --- 中断系统集成验证 ---

// multiTurnMock 返回多次 tool call，强制 agent 循环多个 turn。
type multiTurnMock struct {
	toolCalls int32 // 已发出的 tool call 数
	maxCalls  int32 // 最多发几轮 tool call 再结束
	delay     time.Duration
}

func newMultiTurnMock(maxCalls int32, delay time.Duration) *multiTurnMock {
	return &multiTurnMock{maxCalls: maxCalls, delay: delay}
}

func (m *multiTurnMock) ID() string                     { return "multi-turn-mock" }
func (m *multiTurnMock) MaxTokens() int                 { return 128000 }
func (m *multiTurnMock) SupportsThinking() bool         { return false }
func (m *multiTurnMock) SupportsStreaming() bool        { return false }
func (m *multiTurnMock) SupportsVision() bool           { return false }
func (m *multiTurnMock) CountTokens(_ []model.Message) int { return 100 }
func (m *multiTurnMock) Stream(_ context.Context, _ model.StreamParams) (<-chan model.StreamEvent, error) {
	return nil, nil
}

func (m *multiTurnMock) Call(ctx context.Context, _ model.CallParams) (*model.Response, error) {
	select {
	case <-time.After(m.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	calls := atomic.AddInt32(&m.toolCalls, 1)
	if calls <= m.maxCalls {
		// 返回 tool call，让 loop 继续
		return &model.Response{
			Text: "Working...",
			ToolUse: []model.ToolUse{
				{ID: "tu-1", Name: "ReadFile", Input: map[string]any{"path": "/tmp/test.txt"}},
			},
			Usage: model.Usage{InputTokens: 100, OutputTokens: 50},
		}, nil
	}

	// 最后返回纯文本，结束 loop
	return &model.Response{
		Text:  "Task completed successfully.",
		Usage: model.Usage{InputTokens: 100, OutputTokens: 50},
	}, nil
}

func TestInterrupt_InjectedDuringRun(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(&file_read.ReadFileTool{})

	// 模拟 8 轮 tool call + 1 轮结束 = 9 turns，每轮 30ms
	mock := newMultiTurnMock(8, 30*time.Millisecond)

	ag := New(Config{
		Model:        mock,
		Tools:        registry,
		SystemPrompt: "You are a test agent.",
		MaxTurns:     20,
		Autonomy:     Interactive,
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := ag.Run(context.Background(), "Start a long task")
		t.Logf("Run finished: resp=%q err=%v", truncate(resp, 50), err)
	}()

	// 等几个 turn 后注入中断
	time.Sleep(120 * time.Millisecond)
	ag.Interrupt("Stop! Change direction to task B.")

	<-done

	// 验证中断消息出现在历史中
	msgs := ag.Messages()
	found := false
	for _, msg := range msgs {
		if strings.Contains(msg.Content, "[User Interrupt]") && strings.Contains(msg.Content, "task B") {
			found = true
			break
		}
	}
	if !found {
		t.Error("interrupt message not found in agent messages")
		for i, msg := range msgs {
			t.Logf("  msg[%d] role=%s content=%q", i, msg.Role, truncate(msg.Content, 80))
		}
	}

	t.Logf("Total messages: %d", len(msgs))
}

func TestInterrupt_MultipleMerged(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(&file_read.ReadFileTool{})

	mock := newMultiTurnMock(8, 30*time.Millisecond)

	ag := New(Config{
		Model:        mock,
		Tools:        registry,
		SystemPrompt: "You are a test agent.",
		MaxTurns:     20,
		Autonomy:     Interactive,
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		ag.Run(context.Background(), "Start")
	}()

	time.Sleep(120 * time.Millisecond)

	// 快速发送多条中断
	ag.Interrupt("First interrupt")
	ag.Interrupt("Second interrupt")
	ag.Interrupt("Third interrupt")

	<-done

	msgs := ag.Messages()
	merged := false
	for _, msg := range msgs {
		if strings.Contains(msg.Content, "[User Interrupt]") {
			count := strings.Count(msg.Content, "[User Interrupt]")
			if count >= 2 {
				merged = true
			}
			t.Logf("Merged interrupt (%d parts): %q", count, truncate(msg.Content, 200))
			break
		}
	}
	if !merged {
		t.Error("expected multiple interrupts merged into one message")
	}
}

func TestInterrupt_BufferOverflow(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(&file_read.ReadFileTool{})

	mock := newMultiTurnMock(8, 50*time.Millisecond)

	ag := New(Config{
		Model:        mock,
		Tools:        registry,
		SystemPrompt: "You are a test agent.",
		MaxTurns:     20,
		Autonomy:     Interactive,
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		ag.Run(context.Background(), "Start")
	}()

	time.Sleep(50 * time.Millisecond)

	// 发送超过缓冲区容量（8）的消息
	for i := 0; i < 15; i++ {
		ag.Interrupt("overflow message")
	}

	<-done

	// 不应该 panic 或死锁
	t.Logf("Buffer overflow test passed, messages: %d", len(ag.Messages()))
}

func TestInterruptAndStop(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(&file_read.ReadFileTool{})

	mock := newMultiTurnMock(50, 100*time.Millisecond)

	ag := New(Config{
		Model:        mock,
		Tools:        registry,
		SystemPrompt: "You are a test agent.",
		MaxTurns:     100,
		Autonomy:     Interactive,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := ag.Run(ctx, "Start a very long task")
		t.Logf("Run stopped: resp=%q err=%v", truncate(resp, 50), err)
	}()

	time.Sleep(150 * time.Millisecond)

	ag.InterruptAndStop("Emergency stop!", cancel)

	<-done

	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}
	t.Log("InterruptAndStop test passed")
}

func TestInterrupt_ResetDrains(t *testing.T) {
	ag := New(Config{
		Model:    model.NewMock("mock"),
		Tools:    tool.NewRegistry(),
		MaxTurns: 5,
	})

	for i := 0; i < 5; i++ {
		ag.Interrupt("pending message")
	}

	ag.Reset()

	select {
	case msg := <-ag.interruptCh:
		t.Errorf("channel should be empty after reset, got %q", msg)
	default:
		// 正确：通道已空
	}
}
