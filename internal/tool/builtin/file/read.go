package file

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/openclaw/gclaw/internal/tool"
)

// readAction represents the action to take for a read request.
type readAction int

const (
	readNew       readAction = iota // proceed with normal read
	readUnchanged                   // file unchanged since last read
	readBlocked                     // too many consecutive reads
)

// readEntry identifies a specific read request by path, offset, and limit.
type readEntry struct {
	path   string
	offset int
	limit  int
}

// readTracker tracks file reads for deduplication and loop detection.
type readTracker struct {
	mu          sync.Mutex
	entries     map[readEntry]time.Time // entry -> mtime of the file when last read
	order       []readEntry             // for LRU eviction (insertion order)
	mtimes      map[readEntry]time.Time // entry -> mtime at time of read
	consecCount map[string]int          // path -> consecutive read count
	lastPath    string                  // last file path read
}

const maxTrackerEntries = 500
const maxConsecutiveReads = 4

var tracker = &readTracker{
	entries:     make(map[readEntry]time.Time),
	mtimes:      make(map[readEntry]time.Time),
	consecCount: make(map[string]int),
}

// check determines whether a read should proceed, be skipped as unchanged, or blocked.
func (rt *readTracker) check(entry readEntry, mtime time.Time) readAction {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Update consecutive read tracking
	if entry.path == rt.lastPath {
		rt.consecCount[entry.path]++
	} else {
		// Reset consecutive count for previous path, start new count
		if rt.lastPath != "" {
			rt.consecCount[rt.lastPath] = 0
		}
		rt.consecCount[entry.path] = 1
		rt.lastPath = entry.path
	}

	// Check consecutive read limit
	if rt.consecCount[entry.path] >= maxConsecutiveReads {
		return readBlocked
	}

	// Check if we've seen this exact (path, offset, limit) before with same mtime
	if lastMtime, ok := rt.mtimes[entry]; ok && lastMtime.Equal(mtime) {
		return readUnchanged
	}

	return readNew
}

// record stores a successful read in the tracker.
func (rt *readTracker) record(entry readEntry, mtime time.Time) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// If entry already exists, remove it from order slice so we can re-append at end
	if _, exists := rt.entries[entry]; exists {
		for i, e := range rt.order {
			if e == entry {
				rt.order = append(rt.order[:i], rt.order[i+1:]...)
				break
			}
		}
	}

	rt.entries[entry] = time.Now()
	rt.mtimes[entry] = mtime
	rt.order = append(rt.order, entry)

	rt.evict()
}

// evict removes the oldest entries when the tracker exceeds the cap.
func (rt *readTracker) evict() {
	for len(rt.order) > maxTrackerEntries {
		oldest := rt.order[0]
		rt.order = rt.order[1:]
		delete(rt.entries, oldest)
		delete(rt.mtimes, oldest)
	}
}

// ReadFileTool reads the contents of a file.
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string                              { return "ReadFile" }
func (t *ReadFileTool) Toolset() string                            { return "file" }
func (t *ReadFileTool) Description() string                        { return "Read the contents of a file at the given path." }
func (t *ReadFileTool) Check() bool                                { return true }
func (t *ReadFileTool) ConcurrencySafe() bool                      { return true }
func (t *ReadFileTool) RequiresApproval(params map[string]any) bool { return false }

func (t *ReadFileTool) InputSchema() tool.Schema {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Property{
			"file_path": {Type: "string", Description: "The absolute path to the file to read"},
			"offset":    {Type: "integer", Description: "Line number to start reading from (0-indexed)"},
			"limit":     {Type: "integer", Description: "Maximum number of lines to read"},
		},
		Required: []string{"file_path"},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, params map[string]any) (tool.ToolResult, error) {
	filePath, ok := params["file_path"].(string)
	if !ok {
		return tool.ToolResult{Content: "Error: file_path is required", IsError: true}, nil
	}

	// Get file info for mtime
	info, err := os.Stat(filePath)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error reading file %s: %v", filePath, err),
			IsError: true,
		}, nil
	}
	mtime := info.ModTime()

	// Build readEntry from params
	offset := 0
	limit := 0
	if v, ok := params["offset"]; ok {
		if n, ok := v.(float64); ok {
			offset = int(n)
		}
	}
	if v, ok := params["limit"]; ok {
		if n, ok := v.(float64); ok {
			limit = int(n)
		}
	}

	entry := readEntry{path: filePath, offset: offset, limit: limit}

	// Check tracker
	action := tracker.check(entry, mtime)
	switch action {
	case readUnchanged:
		return tool.ToolResult{
			Content: fmt.Sprintf("File unchanged since last read (path: %s)", filePath),
		}, nil
	case readBlocked:
		return tool.ToolResult{
			Content: fmt.Sprintf("File read blocked: %s has been read %d+ times consecutively. Use a different approach or specify why you need to re-read.", filePath, maxConsecutiveReads),
			IsError: true,
		}, nil
	}

	// Proceed with normal read
	data, err := os.ReadFile(filePath)
	if err != nil {
		return tool.ToolResult{
			Content: fmt.Sprintf("Error reading file %s: %v", filePath, err),
			IsError: true,
		}, nil
	}

	// Record the successful read
	tracker.record(entry, mtime)

	return tool.ToolResult{Content: string(data)}, nil
}

func init() {
	tool.GlobalRegistry.Register(&ReadFileTool{})
}
