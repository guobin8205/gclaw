package tui

import (
	"fmt"
	"io"
	"os"
	"strconv"
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
	for _, e := range h.entries {
		fmt.Fprintf(f, "%d\n%s", len(e), e)
	}
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
	data, err := io.ReadAll(f)
	if err != nil {
		return
	}
	s := string(data)
	for len(s) > 0 {
		nl := strings.IndexByte(s, '\n')
		if nl < 0 {
			break
		}
		n, err := strconv.Atoi(s[:nl])
		if err != nil || n < 0 {
			break
		}
		start := nl + 1
		end := start + n
		if end > len(s) {
			break
		}
		entry := s[start:end]
		if entry != "" {
			h.entries = append(h.entries, entry)
		}
		s = s[end:]
	}
	if len(h.entries) > h.max {
		h.entries = h.entries[len(h.entries)-h.max:]
	}
}
