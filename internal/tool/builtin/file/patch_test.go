package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclaw/gclaw/internal/tool"
)

// Compile-time interface compliance check.
var _ tool.Tool = (*PatchTool)(nil)

func TestPatchTool_Interface(t *testing.T) {
	p := &PatchTool{}
	if p.Name() != "Patch" {
		t.Errorf("Name() = %q, want %q", p.Name(), "Patch")
	}
	if p.Toolset() != "file" {
		t.Errorf("Toolset() = %q, want %q", p.Toolset(), "file")
	}
	if !p.Check() {
		t.Error("Check() = false, want true")
	}
	if p.ConcurrencySafe() {
		t.Error("ConcurrencySafe() = true, want false")
	}
	if !p.RequiresApproval(nil) {
		t.Error("RequiresApproval() = false, want true")
	}
	schema := p.InputSchema()
	if schema.Type != "object" {
		t.Errorf("InputSchema().Type = %q, want %q", schema.Type, "object")
	}
	if _, ok := schema.Properties["file_path"]; !ok {
		t.Error("InputSchema() missing file_path property")
	}
	if _, ok := schema.Properties["old_string"]; !ok {
		t.Error("InputSchema() missing old_string property")
	}
	if _, ok := schema.Properties["new_string"]; !ok {
		t.Error("InputSchema() missing new_string property")
	}
	if _, ok := schema.Properties["replace_all"]; !ok {
		t.Error("InputSchema() missing replace_all property")
	}
}

func TestPatchTool_ExactMatch(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("hello world\nfoo bar\n"), 0644)

	p := &PatchTool{}
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "hello world",
		"new_string": "hello gclaw",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content)
	}

	data, _ := os.ReadFile(fp)
	got := string(data)
	want := "hello gclaw\nfoo bar\n"
	if got != want {
		t.Errorf("file content = %q, want %q", got, want)
	}
}

func TestPatchTool_NotFound(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("hello world\n"), 0644)

	p := &PatchTool{}
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "not present",
		"new_string": "replacement",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for not-found old_string")
	}
	if !strings.Contains(result.Content, "not found") {
		t.Errorf("error message should mention 'not found', got: %s", result.Content)
	}
}

func TestPatchTool_MultipleMatches_Rejection(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("aaa bbb aaa\n"), 0644)

	p := &PatchTool{}
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "aaa",
		"new_string": "ccc",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for multiple matches without replace_all")
	}
	if !strings.Contains(result.Content, "2 times") {
		t.Errorf("error message should mention occurrence count, got: %s", result.Content)
	}
}

func TestPatchTool_ReplaceAll(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("aaa bbb aaa ccc aaa\n"), 0644)

	p := &PatchTool{}
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path":   fp,
		"old_string":  "aaa",
		"new_string":  "zzz",
		"replace_all": true,
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content)
	}

	data, _ := os.ReadFile(fp)
	got := string(data)
	want := "zzz bbb zzz ccc zzz\n"
	if got != want {
		t.Errorf("file content = %q, want %q", got, want)
	}
}

func TestPatchTool_FileNotExist(t *testing.T) {
	p := &PatchTool{}
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path":  "/nonexistent/path/test.txt",
		"old_string": "foo",
		"new_string": "bar",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for nonexistent file")
	}
}

func TestPatchTool_DiffOutput(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("line1\nline2\nline3\nline4\nline5\n"), 0644)

	p := &PatchTool{}
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "line3",
		"new_string": "LINE_THREE",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content)
	}

	if !strings.Contains(result.Content, "-line3") {
		t.Errorf("diff should contain -line3, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "+LINE_THREE") {
		t.Errorf("diff should contain +LINE_THREE, got: %s", result.Content)
	}
}

func TestPatchTool_BinaryRejection(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "binary.bin")
	os.WriteFile(fp, []byte("hello\x00world\n"), 0644)

	p := &PatchTool{}
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "hello",
		"new_string": "bye",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for binary file")
	}
	if !strings.Contains(result.Content, "binary") {
		t.Errorf("error message should mention 'binary', got: %s", result.Content)
	}
}

func TestPatchTool_EmptyReplacement(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("hello world\nfoo bar\n"), 0644)

	p := &PatchTool{}
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "hello ",
		"new_string": "",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content)
	}

	data, _ := os.ReadFile(fp)
	got := string(data)
	want := "world\nfoo bar\n"
	if got != want {
		t.Errorf("file content = %q, want %q", got, want)
	}
}

func TestUnifiedDiff(t *testing.T) {
	old := "line1\nline2\nline3\nline4\nline5\n"
	new_ := "line1\nline2\nLINE_THREE\nline4\nline5\n"

	diff := unifiedDiff(old, new_, "test.txt")

	if !strings.Contains(diff, "--- test.txt") {
		t.Error("diff should contain --- header")
	}
	if !strings.Contains(diff, "+++ test.txt") {
		t.Error("diff should contain +++ header")
	}
	if !strings.Contains(diff, "-line3") {
		t.Error("diff should contain -line3")
	}
	if !strings.Contains(diff, "+LINE_THREE") {
		t.Error("diff should contain +LINE_THREE")
	}
	if !strings.Contains(diff, "@@") {
		t.Error("diff should contain @@ hunk header")
	}
}

func TestUnifiedDiff_AddedLines(t *testing.T) {
	old := "aaa\nbbb\n"
	new_ := "aaa\nbbb\nccc\nddd\n"

	diff := unifiedDiff(old, new_, "add.txt")

	if !strings.Contains(diff, "+ccc") {
		t.Error("diff should contain +ccc")
	}
	if !strings.Contains(diff, "+ddd") {
		t.Error("diff should contain +ddd")
	}
}

func TestUnifiedDiff_RemovedLines(t *testing.T) {
	old := "aaa\nbbb\nccc\n"
	new_ := "aaa\n"

	diff := unifiedDiff(old, new_, "remove.txt")

	if !strings.Contains(diff, "-bbb") {
		t.Error("diff should contain -bbb")
	}
	if !strings.Contains(diff, "-ccc") {
		t.Error("diff should contain -ccc")
	}
}

func TestPatchTool_MissingFilePath(t *testing.T) {
	p := &PatchTool{}
	result, err := p.Execute(context.Background(), map[string]any{
		"old_string": "foo",
		"new_string": "bar",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for missing file_path")
	}
}

func TestPatchTool_MissingOldString(t *testing.T) {
	p := &PatchTool{}
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path":  "test.txt",
		"new_string": "bar",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for missing old_string")
	}
}

func TestPatchTool_MissingNewString(t *testing.T) {
	p := &PatchTool{}
	result, err := p.Execute(context.Background(), map[string]any{
		"file_path":  "test.txt",
		"old_string": "foo",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for missing new_string")
	}
}
