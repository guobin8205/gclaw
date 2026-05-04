package tui

import "testing"

func TestComposerInsertRune(t *testing.T) {
	c := NewComposer()
	c.InsertRune('a')
	c.InsertRune('b')
	if c.Text() != "ab" {
		t.Errorf("expected 'ab', got %q", c.Text())
	}
}

func TestComposerBackspace(t *testing.T) {
	c := NewComposer()
	c.InsertRune('a')
	c.InsertRune('b')
	c.Backspace()
	if c.Text() != "a" {
		t.Errorf("expected 'a', got %q", c.Text())
	}
}

func TestComposerNewLine(t *testing.T) {
	c := NewComposer()
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

func TestComposerBackspaceMerge(t *testing.T) {
	c := NewComposer()
	c.InsertRune('a')
	c.InsertNewLine() // cursor at col 0 of new empty line
	c.Backspace()     // at line start, merge with previous
	if c.Text() != "a" {
		t.Errorf("expected 'a', got %q", c.Text())
	}
	if c.LineCount() != 1 {
		t.Errorf("expected 1 line, got %d", c.LineCount())
	}
}

func TestComposerClear(t *testing.T) {
	c := NewComposer()
	c.InsertRune('x')
	c.Clear()
	if !c.IsEmpty() {
		t.Error("expected empty")
	}
}
