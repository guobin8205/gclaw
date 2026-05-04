package checkpoint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerCheckpointAndList(t *testing.T) {
	baseDir := t.TempDir()
	projectDir := t.TempDir()

	// Create a marker file so findProjectRoot identifies this directory.
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(true, 5, baseDir)

	// Write a file in the project.
	testFile := filepath.Join(projectDir, "hello.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a checkpoint.
	if err := mgr.EnsureCheckpoint(projectDir, "initial snapshot"); err != nil {
		t.Fatalf("EnsureCheckpoint failed: %v", err)
	}

	// List should return one snapshot.
	snapshots, err := mgr.List(projectDir)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	if !strings.Contains(snapshots[0].Message, "initial snapshot") {
		t.Fatalf("expected message to contain 'initial snapshot', got %q", snapshots[0].Message)
	}
}

func TestManagerNewTurnClearsCheckpointed(t *testing.T) {
	baseDir := t.TempDir()
	projectDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(true, 5, baseDir)

	if err := mgr.EnsureCheckpoint(projectDir, "first"); err != nil {
		t.Fatalf("first checkpoint failed: %v", err)
	}

	// Same turn: second checkpoint should be skipped.
	if err := mgr.EnsureCheckpoint(projectDir, "second"); err != nil {
		t.Fatalf("second checkpoint failed: %v", err)
	}

	snapshots, err := mgr.List(projectDir)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot (deduped per turn), got %d", len(snapshots))
	}

	// New turn: checkpoint should succeed and create a second commit.
	mgr.NewTurn()
	if err := mgr.EnsureCheckpoint(projectDir, "after new turn"); err != nil {
		t.Fatalf("post-NewTurn checkpoint failed: %v", err)
	}

	snapshots, err = mgr.List(projectDir)
	if err != nil {
		t.Fatalf("List after NewTurn failed: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots after NewTurn, got %d", len(snapshots))
	}
}

func TestManagerDiffShowsChanges(t *testing.T) {
	baseDir := t.TempDir()
	projectDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(true, 5, baseDir)

	testFile := filepath.Join(projectDir, "data.txt")
	if err := os.WriteFile(testFile, []byte("version 1"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := mgr.EnsureCheckpoint(projectDir, "v1"); err != nil {
		t.Fatal(err)
	}

	snapshots, _ := mgr.List(projectDir)
	if len(snapshots) == 0 {
		t.Fatal("expected at least one snapshot")
	}
	commitHash := snapshots[0].Hash

	// Modify the file.
	if err := os.WriteFile(testFile, []byte("version 2"), 0644); err != nil {
		t.Fatal(err)
	}

	diff, err := mgr.Diff(projectDir, commitHash)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if !strings.Contains(diff, "version 1") || !strings.Contains(diff, "version 2") {
		t.Fatalf("expected diff to show both versions, got:\n%s", diff)
	}
}

func TestManagerRestoreRollsBack(t *testing.T) {
	baseDir := t.TempDir()
	projectDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(true, 5, baseDir)

	testFile := filepath.Join(projectDir, "data.txt")
	if err := os.WriteFile(testFile, []byte("original content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := mgr.EnsureCheckpoint(projectDir, "before change"); err != nil {
		t.Fatal(err)
	}

	snapshots, _ := mgr.List(projectDir)
	if len(snapshots) == 0 {
		t.Fatal("expected at least one snapshot")
	}
	commitHash := snapshots[0].Hash

	// Modify the file.
	if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Restore should roll back.
	if err := mgr.Restore(projectDir, commitHash); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original content" {
		t.Fatalf("expected restored content 'original content', got %q", string(data))
	}
}

func TestManagerDisabled(t *testing.T) {
	baseDir := t.TempDir()
	projectDir := t.TempDir()

	mgr := NewManager(false, 5, baseDir)

	if err := mgr.EnsureCheckpoint(projectDir, "should not run"); err != nil {
		t.Fatalf("disabled manager should return nil, got: %v", err)
	}

	_, err := mgr.List(projectDir)
	if err == nil {
		t.Fatal("expected error when List called on disabled manager")
	}

	_, err = mgr.Diff(projectDir, "abc123")
	if err == nil {
		t.Fatal("expected error when Diff called on disabled manager")
	}

	err = mgr.Restore(projectDir, "abc123")
	if err == nil {
		t.Fatal("expected error when Restore called on disabled manager")
	}
}

func TestManagerMaxSnapshotsPruning(t *testing.T) {
	baseDir := t.TempDir()
	projectDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(true, 3, baseDir)

	// Create more checkpoints than maxSnapshots.
	for i := 0; i < 5; i++ {
		mgr.NewTurn()
		f := filepath.Join(projectDir, "file.txt")
		if err := os.WriteFile(f, []byte("content "+string(rune('0'+i))), 0644); err != nil {
			t.Fatal(err)
		}
		if err := mgr.EnsureCheckpoint(projectDir, "snapshot"); err != nil {
			t.Fatalf("checkpoint %d failed: %v", i, err)
		}
	}

	snapshots, err := mgr.List(projectDir)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(snapshots) > 3 {
		t.Fatalf("expected at most 3 snapshots after pruning, got %d", len(snapshots))
	}
}

func TestFindProjectRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "go.mod")
	if err := os.WriteFile(marker, []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	found := findProjectRoot(sub)
	if found != root {
		t.Fatalf("expected project root %q, got %q", root, found)
	}
}
