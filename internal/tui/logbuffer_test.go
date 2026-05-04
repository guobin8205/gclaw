package tui

import "testing"

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
	lb.Append("d")
	lines := lb.Last(3)
	if lines[0].Content != "b" {
		t.Errorf("expected b, got %s", lines[0].Content)
	}
	if lines[2].Content != "d" {
		t.Errorf("expected d, got %s", lines[2].Content)
	}
}

func TestLogBufferEmpty(t *testing.T) {
	lb := NewLogBuffer(5)
	lines := lb.Last(3)
	if len(lines) != 0 {
		t.Fatalf("expected 0, got %d", len(lines))
	}
}

func TestLogBufferLevels(t *testing.T) {
	lb := NewLogBuffer(5)
	lb.AppendLevel("INFO", "info msg")
	lb.AppendLevel("WARN", "warn msg")
	lines := lb.Last(2)
	if lines[0].Level != "INFO" {
		t.Errorf("expected INFO, got %s", lines[0].Level)
	}
}
