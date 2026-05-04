package tui

import (
	"path/filepath"
	"testing"
)

func TestHistoryAdd(t *testing.T) {
	h := NewHistory("", 100)
	h.Add("hello")
	h.Add("world")
	if h.Len() != 2 {
		t.Fatalf("expected 2, got %d", h.Len())
	}
}

func TestHistoryNavigation(t *testing.T) {
	h := NewHistory("", 100)
	h.Add("first")
	h.Add("second")
	h.Add("third")
	if h.Older() != "third" {
		t.Error("expected third")
	}
	if h.Older() != "second" {
		t.Error("expected second")
	}
	if h.Newer() != "third" {
		t.Error("expected third")
	}
}

func TestHistoryNoDupes(t *testing.T) {
	h := NewHistory("", 100)
	h.Add("same")
	h.Add("same")
	if h.Len() != 1 {
		t.Fatalf("expected 1, got %d", h.Len())
	}
}

func TestHistoryMaxEntries(t *testing.T) {
	h := NewHistory("", 3)
	h.Add("a")
	h.Add("b")
	h.Add("c")
	h.Add("d")
	if h.Len() != 3 {
		t.Fatalf("expected 3, got %d", h.Len())
	}
}

func TestHistorySaveLoad(t *testing.T) {
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
}

func TestHistorySearch(t *testing.T) {
	h := NewHistory("", 100)
	h.Add("/help")
	h.Add("/status")
	h.Add("hello")
	results := h.Search("/")
	if len(results) != 2 {
		t.Fatalf("expected 2, got %d", len(results))
	}
}
