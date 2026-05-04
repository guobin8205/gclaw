package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface compliance check.
var _ tool.Tool = (*ReadFileTool)(nil)

// resetTracker clears the global tracker state between tests.
func resetTracker() {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	tracker.entries = make(map[readEntry]time.Time)
	tracker.mtimes = make(map[readEntry]time.Time)
	tracker.order = nil
	tracker.consecCount = make(map[string]int)
	tracker.lastPath = ""
}

func TestReadFileTool_Interface(t *testing.T) {
	r := &ReadFileTool{}
	if r.Name() != "ReadFile" {
		t.Errorf("Name() = %q, want %q", r.Name(), "ReadFile")
	}
	if r.Toolset() != "file" {
		t.Errorf("Toolset() = %q, want %q", r.Toolset(), "file")
	}
	if !r.Check() {
		t.Error("Check() = false, want true")
	}
	if !r.ConcurrencySafe() {
		t.Error("ConcurrencySafe() = false, want true")
	}
	if r.RequiresApproval(nil) {
		t.Error("RequiresApproval() = true, want false")
	}
	schema := r.InputSchema()
	if schema.Type != "object" {
		t.Errorf("InputSchema().Type = %q, want %q", schema.Type, "object")
	}
	if _, ok := schema.Properties["file_path"]; !ok {
		t.Error("InputSchema() missing file_path property")
	}
}

func TestReadFileTool_FirstRead(t *testing.T) {
	resetTracker()
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("hello world"), 0644)

	r := &ReadFileTool{}
	result, err := r.Execute(context.Background(), map[string]any{
		"file_path": fp,
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content)
	}
	if result.Content != "hello world" {
		t.Errorf("Content = %q, want %q", result.Content, "hello world")
	}
}

func TestReadFileTool_UnchangedFile(t *testing.T) {
	resetTracker()
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("hello world"), 0644)

	r := &ReadFileTool{}

	// First read should succeed
	result, err := r.Execute(context.Background(), map[string]any{
		"file_path": fp,
	})
	if err != nil {
		t.Fatalf("First read error: %v", err)
	}
	if result.IsError {
		t.Fatalf("First read returned error: %s", result.Content)
	}

	// Second read should return unchanged
	result, err = r.Execute(context.Background(), map[string]any{
		"file_path": fp,
	})
	if err != nil {
		t.Fatalf("Second read error: %v", err)
	}
	if !strings.Contains(result.Content, "File unchanged") {
		t.Errorf("Expected unchanged message, got: %s", result.Content)
	}
	if result.IsError {
		t.Error("Unchanged result should not be an error")
	}
}

func TestReadFileTool_ModifiedFileTriggersFreshRead(t *testing.T) {
	resetTracker()
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("hello world"), 0644)

	r := &ReadFileTool{}

	// First read
	result, err := r.Execute(context.Background(), map[string]any{
		"file_path": fp,
	})
	if err != nil {
		t.Fatalf("First read error: %v", err)
	}
	if result.IsError {
		t.Fatalf("First read error: %s", result.Content)
	}

	// Modify the file (change mtime)
	time.Sleep(10 * time.Millisecond) // ensure mtime differs
	os.WriteFile(fp, []byte("modified content"), 0644)

	// Second read should detect the change and re-read
	result, err = r.Execute(context.Background(), map[string]any{
		"file_path": fp,
	})
	if err != nil {
		t.Fatalf("Second read error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Second read returned error: %s", result.Content)
	}
	if result.Content != "modified content" {
		t.Errorf("Content = %q, want %q", result.Content, "modified content")
	}
}

func TestReadFileTool_ConsecutiveReadsBlocked(t *testing.T) {
	resetTracker()
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("content"), 0644)

	r := &ReadFileTool{}

	// Modify file between reads to avoid "unchanged" early return
	for i := 0; i < maxConsecutiveReads-1; i++ {
		time.Sleep(10 * time.Millisecond)
		os.WriteFile(fp, []byte("content"), 0644)

		result, err := r.Execute(context.Background(), map[string]any{
			"file_path": fp,
		})
		if err != nil {
			t.Fatalf("Read %d error: %v", i+1, err)
		}
		if result.IsError {
			t.Fatalf("Read %d should not be blocked yet: %s", i+1, result.Content)
		}
	}

	// The next read should be blocked
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(fp, []byte("content"), 0644)
	result, err := r.Execute(context.Background(), map[string]any{
		"file_path": fp,
	})
	if err != nil {
		t.Fatalf("Blocked read error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected blocked read to return error")
	}
	if !strings.Contains(result.Content, "File read blocked") {
		t.Errorf("Expected blocked message, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, fp) {
		t.Errorf("Blocked message should contain file path, got: %s", result.Content)
	}
}

func TestReadFileTool_DifferentFileResetsConsecutiveCount(t *testing.T) {
	resetTracker()
	dir := t.TempDir()
	fp1 := filepath.Join(dir, "file1.txt")
	fp2 := filepath.Join(dir, "file2.txt")
	os.WriteFile(fp1, []byte("content1"), 0644)
	os.WriteFile(fp2, []byte("content2"), 0644)

	r := &ReadFileTool{}

	// Read file1 multiple times (less than max)
	for i := 0; i < maxConsecutiveReads-2; i++ {
		time.Sleep(10 * time.Millisecond)
		os.WriteFile(fp1, []byte("content1"), 0644)
		result, err := r.Execute(context.Background(), map[string]any{
			"file_path": fp1,
		})
		if err != nil {
			t.Fatalf("Read %d error: %v", i+1, err)
		}
		if result.IsError {
			t.Fatalf("Read %d should not be blocked: %s", i+1, result.Content)
		}
	}

	// Read file2 — should reset consecutive count for file1
	result, err := r.Execute(context.Background(), map[string]any{
		"file_path": fp2,
	})
	if err != nil {
		t.Fatalf("file2 read error: %v", err)
	}
	if result.IsError {
		t.Fatalf("file2 read error: %s", result.Content)
	}

	// Now read file1 again — should not be blocked since count was reset
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(fp1, []byte("content1"), 0644)
	result, err = r.Execute(context.Background(), map[string]any{
		"file_path": fp1,
	})
	if err != nil {
		t.Fatalf("file1 re-read error: %v", err)
	}
	if result.IsError {
		t.Fatalf("file1 should not be blocked after reading different file: %s", result.Content)
	}
}

func TestReadFileTool_TrackerCap(t *testing.T) {
	resetTracker()

	rt := &readTracker{
		entries:     make(map[readEntry]time.Time),
		mtimes:      make(map[readEntry]time.Time),
		consecCount: make(map[string]int),
	}

	// Add maxTrackerEntries + 50 entries
	total := maxTrackerEntries + 50
	for i := 0; i < total; i++ {
		entry := readEntry{
			path:   filepath.Join("/tmp", fmt.Sprintf("file_%d.txt", i)),
			offset: 0,
			limit:  0,
		}
		mtime := time.Now()
		rt.record(entry, mtime)
	}

	rt.mu.Lock()
	count := len(rt.order)
	rt.mu.Unlock()

	if count > maxTrackerEntries {
		t.Errorf("tracker has %d entries, expected at most %d", count, maxTrackerEntries)
	}

	// Verify oldest entries were evicted: first entry should now be the one at index (total - maxTrackerEntries)
	rt.mu.Lock()
	firstEntry := rt.order[0]
	expectedFirstPath := filepath.Join("/tmp", fmt.Sprintf("file_%d.txt", total-maxTrackerEntries))
	rt.mu.Unlock()

	if firstEntry.path != expectedFirstPath {
		t.Errorf("oldest surviving entry path = %q, want %q", firstEntry.path, expectedFirstPath)
	}
}

func TestReadFileTool_MissingFilePath(t *testing.T) {
	resetTracker()
	r := &ReadFileTool{}
	result, err := r.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for missing file_path")
	}
}

func TestReadFileTool_NonexistentFile(t *testing.T) {
	resetTracker()
	r := &ReadFileTool{}
	result, err := r.Execute(context.Background(), map[string]any{
		"file_path": "/nonexistent/path/test.txt",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for nonexistent file")
	}
}

func TestReadFileTool_DifferentOffsetLimitNotUnchanged(t *testing.T) {
	resetTracker()
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("line1\nline2\nline3\n"), 0644)

	r := &ReadFileTool{}

	// Read without offset/limit
	result, err := r.Execute(context.Background(), map[string]any{
		"file_path": fp,
	})
	if err != nil {
		t.Fatalf("First read error: %v", err)
	}
	if result.IsError {
		t.Fatalf("First read error: %s", result.Content)
	}

	// Read same file with different offset — should NOT be unchanged (different entry)
	result, err = r.Execute(context.Background(), map[string]any{
		"file_path": fp,
		"offset":    float64(1),
		"limit":     float64(1),
	})
	if err != nil {
		t.Fatalf("Second read error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Second read with different offset should succeed: %s", result.Content)
	}
	// It should return actual content, not "unchanged"
	if strings.Contains(result.Content, "File unchanged") {
		t.Error("Different offset/limit should not trigger unchanged")
	}
}
