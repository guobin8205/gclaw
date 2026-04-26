package memory

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileProvider implements Provider using local markdown files.
// Uses the Claude Code memory format: MEMORY.md index + individual .md files.
type FileProvider struct {
	dir      string
	entries  []Entry
	ready    bool
}

// NewFileProvider creates a file-based memory provider.
func NewFileProvider(dir string) *FileProvider {
	return &FileProvider{dir: dir}
}

func (p *FileProvider) Available() bool { return true }

func (p *FileProvider) Initialize(sessionID string) error {
	_ = sessionID
	if err := p.loadEntries(); err != nil {
		return err
	}
	p.ready = true
	return nil
}

func (p *FileProvider) Shutdown() error {
	p.ready = false
	return nil
}

func (p *FileProvider) SystemPromptBlock() string {
	if len(p.entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("You have access to persistent memories stored in " + p.dir + ".\n")
	sb.WriteString("Relevant memories are prefetched before each turn and shown in <memory-context> tags.\n")
	sb.WriteString("Use this context to personalize responses. Do not mention 'memory' or 'prefetch' to the user.\n")
	return sb.String()
}

// Prefetch searches entries for keywords from the query.
func (p *FileProvider) Prefetch(ctx context.Context, query string, limit int) ([]Entry, error) {
	if !p.ready || len(p.entries) == 0 || query == "" {
		return nil, nil
	}

	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil, nil
	}

	type scored struct {
		entry Entry
		score int
	}
	var items []scored

	for _, e := range p.entries {
		content := strings.ToLower(e.Title + " " + e.Content)
		s := 0
		for _, w := range words {
			if len(w) < 3 {
				continue
			}
			if strings.Contains(content, w) {
				s++
			}
		}
		if s > 0 {
			items = append(items, scored{entry: e, score: s})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})

	if limit <= 0 {
		limit = 5
	}
	n := limit
	if n > len(items) {
		n = len(items)
	}

	result := make([]Entry, n)
	for i := 0; i < n; i++ {
		result[i] = items[i].entry
	}
	return result, nil
}

// SyncTurn is a no-op for file provider (writes are handled externally).
func (p *FileProvider) SyncTurn(ctx context.Context, userMsg, assistantMsg string) error {
	return nil
}

// loadEntries scans the memory directory for .md files.
func (p *FileProvider) loadEntries() error {
	if _, err := os.Stat(p.dir); os.IsNotExist(err) {
		return nil
	}

	var entries []Entry

	filepath.Walk(p.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		rel, _ := filepath.Rel(p.dir, path)
		if rel == "MEMORY.md" {
			return nil // index file, skip
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		entry := parseEntry(string(data), rel)
		if entry != nil {
			entries = append(entries, *entry)
		}
		return nil
	})

	// Sort by title for deterministic ordering
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Title < entries[j].Title
	})

	p.entries = entries
	return nil
}

// parseEntry parses a memory .md file with YAML frontmatter.
func parseEntry(content, relPath string) *Entry {
	e := &Entry{File: relPath}

	// Try to read frontmatter title and type
	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) >= 3 {
			fm := parts[1]
			for _, line := range strings.Split(fm, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name:") {
					e.Title = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				}
				if strings.HasPrefix(line, "type:") {
					e.Type = strings.TrimSpace(strings.TrimPrefix(line, "type:"))
				}
			}
			e.Content = strings.TrimSpace(parts[2])
		}
	}

	if e.Title == "" {
		// Use filename without extension as title
		e.Title = strings.TrimSuffix(filepath.Base(relPath), ".md")
	}
	if e.Type == "" {
		e.Type = "unknown"
	}

	return e
}
