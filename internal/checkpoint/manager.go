package checkpoint

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// SnapshotInfo holds metadata about a checkpoint snapshot.
type SnapshotInfo struct {
	Hash      string
	Message   string
	Timestamp string
}

// Manager creates and manages shadow-git snapshots of project directories.
type Manager struct {
	enabled      bool
	maxSnapshots int
	baseDir      string
	checkpointed map[string]bool // dir -> checked this turn
	mu           sync.Mutex
}

// NewManager creates a new checkpoint manager.
func NewManager(enabled bool, maxSnapshots int, baseDir string) *Manager {
	if maxSnapshots <= 0 {
		maxSnapshots = 50
	}
	return &Manager{
		enabled:      enabled,
		maxSnapshots: maxSnapshots,
		baseDir:      baseDir,
		checkpointed: make(map[string]bool),
	}
}

// NewTurn clears the per-turn checkpointed set. Call at the start of each agent turn.
func (m *Manager) NewTurn() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkpointed = make(map[string]bool)
}

// EnsureCheckpoint creates a snapshot of the project containing dir if one hasn't
// been created this turn. It uses a shadow git repo to avoid interfering with the
// project's own git history.
func (m *Manager) EnsureCheckpoint(dir string, reason string) error {
	if !m.enabled {
		return nil
	}

	projectRoot := findProjectRoot(dir)

	m.mu.Lock()
	if m.checkpointed[projectRoot] {
		m.mu.Unlock()
		return nil
	}
	m.checkpointed[projectRoot] = true
	m.mu.Unlock()

	// Ensure shadow repo directory exists.
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(projectRoot)))
	shadowDir := filepath.Join(m.baseDir, hash)
	if err := os.MkdirAll(shadowDir, 0755); err != nil {
		return fmt.Errorf("create shadow dir: %w", err)
	}

	shadowRepo := filepath.Join(shadowDir, "repo.git")

	// Initialize shadow repo if it doesn't exist.
	if _, err := os.Stat(shadowRepo); os.IsNotExist(err) {
		// git init --bare does not allow GIT_WORK_TREE; run without it.
		cmd := exec.Command("git", "init", "--bare", shadowRepo)
		cmd.Env = []string{
			"GIT_CONFIG_GLOBAL=" + os.DevNull,
			"GIT_CONFIG_NOSYSTEM=1",
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("init shadow repo: %w (stderr: %s)", err, stderr.String())
		}
	}

	// Configure the shadow repo to accept all commits.
	_ = runGit(projectRoot, shadowRepo, "config", "user.email", "checkpoint@gclaw.local")
	_ = runGit(projectRoot, shadowRepo, "config", "user.name", "gclaw-checkpoint")

	// Stage all changes and commit.
	if err := runGit(projectRoot, shadowRepo, "add", "-A"); err != nil {
		// No changes to add is fine.
		if !strings.Contains(err.Error(), "nothing to commit") {
			// Continue anyway; add may fail on untracked files but still work.
		}
	}

	msg := fmt.Sprintf("checkpoint: %s", reason)
	err := runGit(projectRoot, shadowRepo, "commit", "-m", msg, "--allow-empty")
	if err != nil {
		// If commit fails because nothing changed, that's acceptable.
		if strings.Contains(err.Error(), "nothing to commit") ||
			strings.Contains(err.Error(), "nothing added") {
			// Ensure at least an empty commit exists so there's a history.
			err = runGit(projectRoot, shadowRepo, "commit", "-m", msg, "--allow-empty")
		}
		if err != nil {
			return fmt.Errorf("commit checkpoint: %w", err)
		}
	}

	// Prune oldest commits if over maxSnapshots.
	if err := m.pruneOldest(projectRoot, shadowRepo); err != nil {
		// Non-fatal: log and continue.
		_ = err
	}

	return nil
}

// Restore rolls back projectRoot to the given commit hash in the shadow repo.
func (m *Manager) Restore(dir string, commitHash string) error {
	if !m.enabled {
		return fmt.Errorf("checkpoint manager is disabled")
	}

	projectRoot := findProjectRoot(dir)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(projectRoot)))
	shadowRepo := filepath.Join(m.baseDir, hash, "repo.git")

	// Checkout the commit into the working tree.
	if err := runGit(projectRoot, shadowRepo, "checkout", commitHash, "--", "."); err != nil {
		return fmt.Errorf("restore checkpoint: %w", err)
	}
	return nil
}

// List returns all snapshots in the shadow repo for the project containing dir.
func (m *Manager) List(dir string) ([]SnapshotInfo, error) {
	if !m.enabled {
		return nil, fmt.Errorf("checkpoint manager is disabled")
	}

	projectRoot := findProjectRoot(dir)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(projectRoot)))
	shadowRepo := filepath.Join(m.baseDir, hash, "repo.git")

	out, err := runGitOutput(projectRoot, shadowRepo, "log", "--format=%H|%s|%ci")
	if err != nil {
		return nil, fmt.Errorf("list checkpoints: %w", err)
	}

	var snapshots []SnapshotInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 2 {
			continue
		}
		si := SnapshotInfo{
			Hash:    parts[0],
			Message: parts[1],
		}
		if len(parts) >= 3 {
			si.Timestamp = parts[2]
		}
		snapshots = append(snapshots, si)
	}

	return snapshots, nil
}

// Diff returns the diff between the current working tree and the given commit.
func (m *Manager) Diff(dir string, commitHash string) (string, error) {
	if !m.enabled {
		return "", fmt.Errorf("checkpoint manager is disabled")
	}

	projectRoot := findProjectRoot(dir)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(projectRoot)))
	shadowRepo := filepath.Join(m.baseDir, hash, "repo.git")

	out, err := runGitOutput(projectRoot, shadowRepo, "diff", commitHash, "--", ".")
	if err != nil {
		return "", fmt.Errorf("diff checkpoint: %w", err)
	}
	return string(out), nil
}

// pruneOldest collapses history to at most maxSnapshots by creating a fresh
// orphan commit when the limit is exceeded.
func (m *Manager) pruneOldest(projectRoot, shadowRepo string) error {
	out, err := runGitOutput(projectRoot, shadowRepo, "rev-list", "--count", "HEAD")
	if err != nil {
		return err
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	if count <= m.maxSnapshots {
		return nil
	}

	// Get the current tree (working directory state).
	treeOut, err := runGitOutput(projectRoot, shadowRepo, "write-tree")
	if err != nil {
		return err
	}
	treeHash := strings.TrimSpace(string(treeOut))

	// Create a new orphan commit with this tree (no parent).
	commitOut, err := runGitOutput(projectRoot, shadowRepo, "commit-tree", treeHash, "-m", "checkpoint: consolidated")
	if err != nil {
		return err
	}
	newCommit := strings.TrimSpace(string(commitOut))

	// Update HEAD to point to this new commit.
	return runGit(projectRoot, shadowRepo, "update-ref", "HEAD", newCommit)
}

// findProjectRoot walks up from dir looking for project markers.
func findProjectRoot(dir string) string {
	markers := []string{".git", "go.mod", "package.json", "pyproject.toml"}
	for {
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}

// runGit executes a git command with isolated environment variables.
func runGit(workTree, gitDir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = workTree
	cmd.Env = gitEnv(workTree, gitDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %v: %w (stderr: %s)", args, err, stderr.String())
	}
	return nil
}

// runGitOutput executes a git command and returns stdout.
func runGitOutput(workTree, gitDir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workTree
	cmd.Env = gitEnv(workTree, gitDir)
	return cmd.Output()
}

// gitEnv returns environment variables that isolate git from the user's global config.
func gitEnv(workTree, gitDir string) []string {
	return []string{
		"GIT_DIR=" + gitDir,
		"GIT_WORK_TREE=" + workTree,
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
	}
}
