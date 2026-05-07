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

func TestComposerMoveUpDown(t *testing.T) {
	c := NewComposer()
	c.InsertRune('a')
	c.InsertRune('b')
	c.InsertNewLine()
	c.InsertRune('c')
	c.InsertRune('d')
	// State: lines=["ab","cd"], curRow=1, curCol=2

	// MoveUp from line 1 col 2 -> line 0 col 2 (at end of "ab")
	if !c.MoveUp() {
		t.Error("expected MoveUp to succeed")
	}
	row, col := c.CursorPos()
	if row != 0 || col != 2 {
		t.Errorf("expected row=0 col=2, got row=%d col=%d", row, col)
	}

	// MoveUp from line 0 -> fails
	if c.MoveUp() {
		t.Error("expected MoveUp to fail on first line")
	}
	row, col = c.CursorPos()
	if row != 0 || col != 2 {
		t.Errorf("expected still at row=0 col=2, got row=%d col=%d", row, col)
	}

	// MoveDown from line 0 col 2 -> line 1 col 2 (at end of "cd")
	if !c.MoveDown() {
		t.Error("expected MoveDown to succeed")
	}
	row, col = c.CursorPos()
	if row != 1 || col != 2 {
		t.Errorf("expected row=1 col=2, got row=%d col=%d", row, col)
	}

	// MoveDown from last line -> fails
	if c.MoveDown() {
		t.Error("expected MoveDown to fail on last line")
	}
}

func TestComposerMoveUpColClamp(t *testing.T) {
	c := NewComposer()
	c.InsertRune('a')
	c.InsertRune('b')
	c.InsertRune('c')
	c.InsertNewLine()
	c.InsertRune('x')
	// State: lines=["abc","x"], curRow=1, curCol=1

	// MoveHome on line 1, then MoveEnd -> col=1
	c.MoveHome()
	row, col := c.CursorPos()
	if row != 1 || col != 0 {
		t.Errorf("expected row=1 col=0, got row=%d col=%d", row, col)
	}

	// MoveUp from line 1 col 0 -> line 0 col 0
	if !c.MoveUp() {
		t.Error("expected MoveUp to succeed")
	}
	row, col = c.CursorPos()
	if row != 0 || col != 0 {
		t.Errorf("expected row=0 col=0, got row=%d col=%d", row, col)
	}

	// Now set cursor at col 3 (past end of line 1 which has len 1) and move down
	c.MoveEnd() // col=3 on line 0
	c.MoveDown()
	row, col = c.CursorPos()
	// col should clamp to 1 (length of "x")
	if row != 1 || col != 1 {
		t.Errorf("expected row=1 col=1 (clamped), got row=%d col=%d", row, col)
	}
}

// TestComposerNewLineNoAliasing is a regression test for a slice aliasing bug
// where InsertNewLine's before/after shared the same backing array, causing
// subsequent InsertRune on the before line to corrupt the after line's data.
func TestComposerNewLineNoAliasing(t *testing.T) {
	c := NewComposer()
	// Build a line with 4 runes so before has spare capacity after split
	for _, r := range "啊米的啊" {
		c.InsertRune(r)
	}
	// State: lines=["啊米的啊"], curRow=0, curCol=4

	// Move to col 2 and split — before="啊米" gets cap=4, after="的啊" shares backing
	c.curCol = 2
	c.InsertNewLine()
	// lines=["啊米", "的啊"], curRow=1, curCol=0

	// Move back to line 0 end
	c.MoveUp()   // curRow=0, curCol=2
	c.MoveEnd()  // curRow=0, curCol=2

	// Insert a rune — without the fix this would overwrite after's data via shared backing
	c.InsertRune('我')
	// lines should be ["啊米我", "的啊"]

	if c.Text() != "啊米我\n的啊" {
		t.Errorf("expected '啊米我\\n的啊', got %q", c.Text())
	}
	// Also check the after line wasn't corrupted
	row, _ := c.CursorPos()
	if row != 0 {
		t.Errorf("expected cursor on row 0, got %d", row)
	}
}

func TestComposerAtBoundary(t *testing.T) {
	c := NewComposer()

	// Empty composer: at first line start AND last line end
	if !c.AtFirstLineStart() {
		t.Error("expected AtFirstLineStart on empty")
	}
	if !c.AtLastLineEnd() {
		t.Error("expected AtLastLineEnd on empty")
	}

	c.InsertRune('a')
	c.InsertNewLine()
	c.InsertRune('b')
	// lines=["a","b"], curRow=1, curCol=1

	if c.AtFirstLineStart() {
		t.Error("should not be at first line start")
	}
	if !c.AtLastLineEnd() {
		t.Error("should be at last line end")
	}

	c.MoveHome()
	c.MoveUp() // row=0, col=0
	if !c.AtFirstLineStart() {
		t.Error("expected AtFirstLineStart at row=0 col=0")
	}
	if c.AtLastLineEnd() {
		t.Error("should not be at last line end")
	}
}
