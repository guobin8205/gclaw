package curator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openclaw/gclaw/internal/delegate"
	"github.com/openclaw/gclaw/internal/skill"
)

// mockFactory returns a no-op agent factory for testing.
func mockFactory() delegate.AgentFactory {
	return func() delegate.AgentRunner {
		return &mockRunner{}
	}
}

type mockRunner struct{}

func (m *mockRunner) Run(ctx context.Context, prompt string) (string, error) {
	return "no changes needed", nil
}
func (m *mockRunner) Reset() {}

func setupTestManager(t *testing.T) (*skill.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	userDir := filepath.Join(dir, "user")
	agentDir := filepath.Join(dir, "agent")
	os.MkdirAll(userDir, 0755)
	os.MkdirAll(agentDir, 0755)

	mgr := skill.NewManager()
	if err := mgr.LoadAll(dir); err != nil {
		t.Fatal(err)
	}
	return mgr, dir
}

func createTestSkill(t *testing.T, mgr *skill.Manager, name, description string) {
	t.Helper()
	err := mgr.CreateSkill(name, "test body for "+name, description)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCuratorMaybeRun_SkipsWhenPaused(t *testing.T) {
	mgr, dir := setupTestManager(t)
	createTestSkill(t, mgr, "test-skill", "a test skill")

	cfg := Config{
		Enabled:      true,
		Interval:     time.Hour,
		MinIdle:      0,
		StaleAfter:   30 * 24 * time.Hour,
		ArchiveAfter: 90 * 24 * time.Hour,
	}
	c := New(cfg, mgr, mockFactory())
	c.stateFile = filepath.Join(dir, ".curator_state")

	// Pause and verify MaybeRun skips
	c.Pause()
	c.MaybeRun(context.Background(), time.Hour)

	skills := mgr.ListAgentSkills()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill after paused MaybeRun, got %d", len(skills))
	}
}

func TestCuratorMaybeRun_SkipsWithinInterval(t *testing.T) {
	mgr, dir := setupTestManager(t)
	createTestSkill(t, mgr, "test-skill", "a test skill")

	cfg := Config{
		Enabled:      true,
		Interval:     24 * time.Hour, // long interval
		MinIdle:      0,
		StaleAfter:   30 * 24 * time.Hour,
		ArchiveAfter: 90 * 24 * time.Hour,
	}
	c := New(cfg, mgr, mockFactory())
	c.stateFile = filepath.Join(dir, ".curator_state")

	// Force a run to set LastRunAt
	c.RunNow(context.Background())
	state := c.Status()
	if state.RunCount != 1 {
		t.Fatalf("expected 1 run after RunNow, got %d", state.RunCount)
	}

	// MaybeRun should be skipped (within 24h interval)
	c.MaybeRun(context.Background(), time.Hour)
	state = c.Status()
	if state.RunCount != 1 {
		t.Fatalf("expected still 1 run (skipped by interval), got %d", state.RunCount)
	}
}

func TestCuratorArchiveRestore(t *testing.T) {
	mgr, dir := setupTestManager(t)
	createTestSkill(t, mgr, "to-archive", "will be archived")

	// Archive
	if err := mgr.ArchiveSkill("to-archive"); err != nil {
		t.Fatal(err)
	}

	// Verify it's gone from active list
	skills := mgr.ListAgentSkills()
	if len(skills) != 0 {
		t.Fatalf("expected 0 active agent skills after archive, got %d", len(skills))
	}

	// Verify it exists in .archive
	archivePath := filepath.Join(dir, ".archive", "to-archive")
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		t.Fatal("archived skill directory not found")
	}

	// Restore
	if err := mgr.UnarchiveSkill("to-archive"); err != nil {
		t.Fatal(err)
	}

	// Verify it's back
	skills = mgr.ListAgentSkills()
	if len(skills) != 1 || skills[0].Name != "to-archive" {
		t.Fatalf("expected 1 restored skill, got %v", skills)
	}
}

func TestCuratorAutoTransitions_StaleMarking(t *testing.T) {
	mgr, dir := setupTestManager(t)
	createTestSkill(t, mgr, "old-skill", "an old skill")

	// Make the skill directory appear old by setting mod time to 31 days ago
	agentSkills := mgr.ListAgentSkills()
	oldTime := time.Now().Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(agentSkills[0].Dir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Enabled:      true,
		Interval:     0, // run immediately
		MinIdle:      0,
		StaleAfter:   30 * 24 * time.Hour,
		ArchiveAfter: 90 * 24 * time.Hour,
	}
	c := New(cfg, mgr, mockFactory())
	c.stateFile = filepath.Join(dir, ".curator_state")

	// Force a run (bypass interval check)
	c.RunNow(context.Background())

	// Reload skills to check description
	skills := mgr.ListAgentSkills()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Description != "[stale] an old skill" {
		t.Fatalf("expected [stale] prefix on description, got %q", skills[0].Description)
	}
}

func TestCuratorAutoTransitions_Archive(t *testing.T) {
	mgr, dir := setupTestManager(t)
	createTestSkill(t, mgr, "ancient-skill", "very old")

	// Make the skill directory appear 91 days old
	agentSkills := mgr.ListAgentSkills()
	oldTime := time.Now().Add(-91 * 24 * time.Hour)
	if err := os.Chtimes(agentSkills[0].Dir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Enabled:      true,
		Interval:     0,
		MinIdle:      0,
		StaleAfter:   30 * 24 * time.Hour,
		ArchiveAfter: 90 * 24 * time.Hour,
	}
	c := New(cfg, mgr, mockFactory())
	c.stateFile = filepath.Join(dir, ".curator_state")

	c.RunNow(context.Background())

	// Should be archived (not in active list)
	skills := mgr.ListAgentSkills()
	if len(skills) != 0 {
		t.Fatalf("expected 0 active skills after archive, got %d", len(skills))
	}

	// Should exist in .archive
	archivePath := filepath.Join(dir, ".archive", "ancient-skill")
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		t.Fatal("archived skill not found in .archive")
	}
}

func TestCuratorPinnedSkillNotTouched(t *testing.T) {
	mgr, dir := setupTestManager(t)
	createTestSkill(t, mgr, "pinned-skill", "important")

	// Pin the skill
	if err := mgr.SetPinned("pinned-skill", true); err != nil {
		t.Fatal(err)
	}

	// Make it old
	agentSkills := mgr.ListAgentSkills()
	oldTime := time.Now().Add(-91 * 24 * time.Hour)
	if err := os.Chtimes(agentSkills[0].Dir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Enabled:      true,
		Interval:     0,
		MinIdle:      0,
		StaleAfter:   30 * 24 * time.Hour,
		ArchiveAfter: 90 * 24 * time.Hour,
	}
	c := New(cfg, mgr, mockFactory())
	c.stateFile = filepath.Join(dir, ".curator_state")

	c.RunNow(context.Background())

	// Pinned skill should still be active
	skills := mgr.ListAgentSkills()
	if len(skills) != 1 {
		t.Fatalf("pinned skill should not be archived, got %d skills", len(skills))
	}
}

func TestCuratorStatePersistence(t *testing.T) {
	mgr, dir := setupTestManager(t)

	cfg := Config{Enabled: true, Interval: 0, MinIdle: 0}
	c := New(cfg, mgr, mockFactory())
	c.stateFile = filepath.Join(dir, ".curator_state")

	// Pause and verify persistence
	c.Pause()
	state := c.Status()
	if !state.Paused {
		t.Fatal("expected curator to be paused")
	}

	// Create new curator instance reading same state file
	c2 := New(cfg, mgr, mockFactory())
	c2.stateFile = filepath.Join(dir, ".curator_state")
	state2 := c2.Status()
	if !state2.Paused {
		t.Fatal("expected persisted pause state to survive curator restart")
	}

	// Resume
	c2.Resume()
	state3 := c2.Status()
	if state3.Paused {
		t.Fatal("expected curator to be resumed")
	}
}

func TestCuratorDoesNotTouchUserSkills(t *testing.T) {
	mgr, dir := setupTestManager(t)
	// Create agent skill
	createTestSkill(t, mgr, "agent-skill", "agent created")

	// Also manually create a user skill
	userSkillDir := filepath.Join(dir, "user", "my-user-skill")
	os.MkdirAll(userSkillDir, 0755)
	userSkillFile := filepath.Join(userSkillDir, "SKILL.md")
	os.WriteFile(userSkillFile, []byte("---\nname: my-user-skill\ndescription: user created\n---\n\nbody"), 0644)
	mgr.LoadAll(dir) // reload

	// Make both old
	oldTime := time.Now().Add(-91 * 24 * time.Hour)
	os.Chtimes(filepath.Join(dir, "agent", "agent-skill"), oldTime, oldTime)
	os.Chtimes(userSkillDir, oldTime, oldTime)

	cfg := Config{
		Enabled:      true,
		Interval:     0,
		MinIdle:      0,
		StaleAfter:   30 * 24 * time.Hour,
		ArchiveAfter: 90 * 24 * time.Hour,
	}
	c := New(cfg, mgr, mockFactory())
	c.stateFile = filepath.Join(dir, ".curator_state")
	c.RunNow(context.Background())

	// Agent skill should be archived
	agentSkills := mgr.ListAgentSkills()
	if len(agentSkills) != 0 {
		t.Fatalf("agent skill should be archived, got %d", len(agentSkills))
	}

	// User skill should still exist
	allSkills := mgr.List()
	found := false
	for _, s := range allSkills {
		if s.Name == "my-user-skill" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("user skill should not be touched by curator")
	}
}
