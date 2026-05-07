package tui

import "strings"

type Composer struct {
	lines       [][]rune
	curRow      int
	curCol      int
	attachments []Attachment
}

type Attachment struct {
	Path    string
	IsImage bool
}

func NewComposer() *Composer {
	return &Composer{lines: [][]rune{{}}}
}

func (c *Composer) Text() string {
	parts := make([]string, len(c.lines))
	for i, runes := range c.lines {
		parts[i] = string(runes)
	}
	return strings.Join(parts, "\n")
}

func (c *Composer) LineCount() int { return len(c.lines) }

func (c *Composer) IsEmpty() bool { return len(c.lines) == 1 && len(c.lines[0]) == 0 }

func (c *Composer) SetInput(text string) {
	parts := strings.Split(text, "\n")
	c.lines = make([][]rune, len(parts))
	for i, p := range parts {
		c.lines[i] = []rune(p)
	}
	if len(c.lines) == 0 {
		c.lines = [][]rune{{}}
	}
	c.curRow = len(c.lines) - 1
	c.curCol = len(c.lines[c.curRow])
}

func (c *Composer) InsertRune(r rune) {
	line := c.lines[c.curRow]
	line = append(line[:c.curCol], append([]rune{r}, line[c.curCol:]...)...)
	c.lines[c.curRow] = line
	c.curCol++
}

func (c *Composer) InsertNewLine() {
	line := c.lines[c.curRow]
	before := append([]rune{}, line[:c.curCol]...)
	after := append([]rune{}, line[c.curCol:]...)
	c.lines[c.curRow] = before
	c.lines = append(c.lines, nil)
	copy(c.lines[c.curRow+2:], c.lines[c.curRow+1:])
	c.lines[c.curRow+1] = after
	c.curRow++
	c.curCol = 0
}

func (c *Composer) Backspace() {
	if c.curCol > 0 {
		line := c.lines[c.curRow]
		c.lines[c.curRow] = append(line[:c.curCol-1], line[c.curCol:]...)
		c.curCol--
	} else if c.curRow > 0 {
		prevLen := len(c.lines[c.curRow-1])
		c.lines[c.curRow-1] = append(c.lines[c.curRow-1], c.lines[c.curRow]...)
		c.lines = append(c.lines[:c.curRow], c.lines[c.curRow+1:]...)
		c.curRow--
		c.curCol = prevLen
	}
}

func (c *Composer) Delete() {
	line := c.lines[c.curRow]
	if c.curCol < len(line) {
		c.lines[c.curRow] = append(line[:c.curCol], line[c.curCol+1:]...)
	} else if c.curRow < len(c.lines)-1 {
		c.lines[c.curRow] = append(c.lines[c.curRow], c.lines[c.curRow+1]...)
		c.lines = append(c.lines[:c.curRow+1], c.lines[c.curRow+2:]...)
	}
}

func (c *Composer) Clear() {
	c.lines = [][]rune{{}}
	c.curRow = 0
	c.curCol = 0
	c.attachments = nil
}

func (c *Composer) MoveLeft() {
	if c.curCol > 0 {
		c.curCol--
	} else if c.curRow > 0 {
		c.curRow--
		c.curCol = len(c.lines[c.curRow])
	}
}

func (c *Composer) MoveRight() {
	if c.curCol < len(c.lines[c.curRow]) {
		c.curCol++
	} else if c.curRow < len(c.lines)-1 {
		c.curRow++
		c.curCol = 0
	}
}

func (c *Composer) MoveHome() { c.curCol = 0 }
func (c *Composer) MoveEnd()  { c.curCol = len(c.lines[c.curRow]) }

func (c *Composer) MoveUp() bool {
	if c.curRow == 0 {
		return false
	}
	c.curRow--
	lineLen := len(c.lines[c.curRow])
	if c.curCol > lineLen {
		c.curCol = lineLen
	}
	return true
}

func (c *Composer) MoveDown() bool {
	if c.curRow >= len(c.lines)-1 {
		return false
	}
	c.curRow++
	lineLen := len(c.lines[c.curRow])
	if c.curCol > lineLen {
		c.curCol = lineLen
	}
	return true
}

func (c *Composer) AtFirstLineStart() bool {
	return c.curRow == 0 && c.curCol == 0
}

func (c *Composer) AtLastLineEnd() bool {
	return c.curRow == len(c.lines)-1 && c.curCol == len(c.lines[c.curRow])
}

// CursorPos returns the current cursor row and column.
func (c *Composer) CursorPos() (row, col int) { return c.curRow, c.curCol }

func (c *Composer) AddAttachment(path string, isImage bool) {
	c.attachments = append(c.attachments, Attachment{Path: path, IsImage: isImage})
}

func (c *Composer) RemoveAttachment(idx int) {
	if idx >= 0 && idx < len(c.attachments) {
		c.attachments = append(c.attachments[:idx], c.attachments[idx+1:]...)
	}
}

func (c *Composer) Attachments() []Attachment { return c.attachments }
