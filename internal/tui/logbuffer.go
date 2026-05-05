package tui

import (
	"context"
	"log/slog"
	"sync"
)

type LogLine struct {
	Level   string
	Content string
}

type LogBuffer struct {
	mu    sync.Mutex
	buf   []LogLine
	size  int
	count int
	head  int
}

func NewLogBuffer(size int) *LogBuffer {
	return &LogBuffer{
		buf:  make([]LogLine, size),
		size: size,
	}
}

func (lb *LogBuffer) Append(content string) {
	lb.AppendLevel("INFO", content)
}

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

func (lb *LogBuffer) SlogHandler() slog.Handler {
	return &logBufferHandler{buf: lb}
}

type logBufferHandler struct {
	buf *LogBuffer
}

func (h *logBufferHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *logBufferHandler) Handle(ctx context.Context, rec slog.Record) error {
	h.buf.AppendLevel(rec.Level.String(), rec.Message)
	return nil
}

func (h *logBufferHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *logBufferHandler) WithGroup(name string) slog.Handler {
	return h
}
