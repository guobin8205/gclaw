package tui

import (
	"bufio"
	"os"
	"strings"
	"sync"
)

type History struct {
	mu      sync.Mutex
	path    string
	max     int
	entries []string
	idx     int
	draft   string
}

func NewHistory(path string, maxEntries int) *History {
	h := &History{
		path: path,
		max:  maxEntries,
		idx:  -1,
	}
	h.load()
	return h
}

func (h *History) Add(entry string) {
	if entry == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == entry {
		return
	}
	h.entries = append(h.entries, entry)
	if len(h.entries) > h.max {
		h.entries = h.entries[len(h.entries)-h.max:]
	}
	h.idx = -1
	h.draft = ""
}

func (h *History) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.entries)
}

func (h *History) Older() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := len(h.entries)
	if n == 0 {
		return ""
	}
	if h.idx == -1 {
		h.draft = ""
		h.idx = n - 1
	} else if h.idx > 0 {
		h.idx--
	} else {
		return ""
	}
	return h.entries[h.idx]
}

func (h *History) Newer() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.idx == -1 {
		return h.draft
	}
	h.idx++
	if h.idx >= len(h.entries) {
		h.idx = -1
		return h.draft
	}
	return h.entries[h.idx]
}

func (h *History) ResetNav(draft string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.idx = -1
	h.draft = draft
}

func (h *History) Current() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.idx == -1 || h.idx >= len(h.entries) {
		return ""
	}
	return h.entries[h.idx]
}

func (h *History) Search(prefix string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var results []string
	for i := len(h.entries) - 1; i >= 0; i-- {
		if strings.HasPrefix(h.entries[i], prefix) {
			results = append(results, h.entries[i])
		}
	}
	return results
}

func (h *History) Save() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.path == "" {
		return
	}
	f, err := os.Create(h.path)
	if err != nil {
		return
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, e := range h.entries {
		w.WriteString(e)
		w.WriteByte('\n')
	}
	w.Flush()
}

func (h *History) load() {
	if h.path == "" {
		return
	}
	f, err := os.Open(h.path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			h.entries = append(h.entries, line)
		}
	}
	if len(h.entries) > h.max {
		h.entries = h.entries[len(h.entries)-h.max:]
	}
}
