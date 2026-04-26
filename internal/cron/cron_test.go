package cron

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockExecutor struct {
	mu       sync.Mutex
	results  []string
	busy     bool
}

func (e *mockExecutor) Submit(ctx context.Context, prompt string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.results = append(e.results, prompt)
	return "done: " + prompt, nil
}

func (e *mockExecutor) IsBusy() bool {
	return e.busy
}

func (e *mockExecutor) lastPrompt() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.results) == 0 {
		return ""
	}
	return e.results[len(e.results)-1]
}

func TestParseValidExpressions(t *testing.T) {
	tests := []string{
		"* * * * *",
		"0 9 * * *",
		"*/5 * * * *",
		"0 0 1 1 *",
		"0,30 9,17 * * 1-5",
		"*/15 */2 * * 0,6",
	}

	for _, expr := range tests {
		_, err := Parse(expr)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", expr, err)
		}
	}
}

func TestParseInvalidExpressions(t *testing.T) {
	tests := []string{
		"",
		"* * * *",
		"* * * * * *",
		"60 * * * *",
		"* 24 * * *",
		"* * 32 * *",
		"* * * 13 *",
		"* * * * 7",
		"invalid * * * *",
	}

	for _, expr := range tests {
		_, err := Parse(expr)
		if err == nil {
			t.Errorf("Parse(%q) should have errored", expr)
		}
	}
}

func TestFieldMatches(t *testing.T) {
	// Wildcard
	f := field{all: true}
	if !f.matches(42) {
		t.Error("wildcard should match everything")
	}

	// Exact
	f = field{exact: []int{1, 5, 9}}
	if !f.matches(5) {
		t.Error("exact should match 5")
	}
	if f.matches(3) {
		t.Error("exact should not match 3")
	}

	// Step
	f = field{step: 5, stepBase: 0}
	if !f.matches(0) {
		t.Error("step */5 should match 0")
	}
	if !f.matches(15) {
		t.Error("step */5 should match 15")
	}
	if f.matches(6) {
		t.Error("step */5 should not match 6")
	}
}

func TestNextRunEveryMinute(t *testing.T) {
	base := time.Date(2026, 4, 26, 13, 30, 0, 0, time.Local)
	next := nextRun("* * * * *", base, time.Local)
	if next.Minute() != 31 || next.Hour() != 13 {
		t.Errorf("expected 13:31, got %s", next.Format("15:04"))
	}
}

func TestNextRunDaily9AM(t *testing.T) {
	base := time.Date(2026, 4, 26, 10, 0, 0, 0, time.Local)
	next := nextRun("0 9 * * *", base, time.Local)
	if next.Hour() != 9 || next.Minute() != 0 {
		t.Errorf("expected 09:00, got %s", next.Format("15:04"))
	}
	if next.Day() != 27 {
		t.Errorf("expected next day (27), got %d", next.Day())
	}
}

func TestNextRunSpecificTime(t *testing.T) {
	// At 08:30 today — next should be 08:30 tomorrow or same day
	base := time.Date(2026, 4, 26, 8, 0, 0, 0, time.Local)
	next := nextRun("30 8 * * *", base, time.Local)
	if next.Hour() != 8 || next.Minute() != 30 {
		t.Errorf("expected 08:30, got %s", next.Format("15:04"))
	}
}

func TestSchedulerAddJob(t *testing.T) {
	exec := &mockExecutor{}
	s := NewScheduler(exec)

	err := s.AddJob(&Job{Enabled: true, 
		Name:     "test-job",
		Schedule: "0 9 * * *",
		Prompt:   "daily check",
	})
	if err != nil {
		t.Fatalf("add job failed: %v", err)
	}

	jobs := s.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Name != "test-job" {
		t.Errorf("expected 'test-job', got %q", jobs[0].Name)
	}
	if jobs[0].NextRun.IsZero() {
		t.Error("next run should not be zero")
	}
}

func TestSchedulerInvalidJob(t *testing.T) {
	exec := &mockExecutor{}
	s := NewScheduler(exec)

	err := s.AddJob(&Job{Enabled: true, 
		Name:     "bad-job",
		Schedule: "invalid expr",
		Prompt:   "test",
	})
	if err == nil {
		t.Error("expected error for invalid schedule")
	}
}

func TestSchedulerRemoveJob(t *testing.T) {
	exec := &mockExecutor{}
	s := NewScheduler(exec)
	s.AddJob(&Job{Enabled: true, Name: "j1", Schedule: "* * * * *", Prompt: "p1"})
	s.AddJob(&Job{Enabled: true, Name: "j2", Schedule: "* * * * *", Prompt: "p2"})

	s.RemoveJob("j1")
	jobs := s.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Name != "j2" {
		t.Errorf("expected j2, got %q", jobs[0].Name)
	}
}

func TestSchedulerStats(t *testing.T) {
	exec := &mockExecutor{}
	s := NewScheduler(exec)
	s.AddJob(&Job{Enabled: true, Name: "stats-job", Schedule: "0 0 * * *", Prompt: "stats"})

	stats := s.Stats()
	total, ok := stats["total_jobs"].(int)
	if !ok || total != 1 {
		t.Errorf("expected total_jobs=1, got %v", stats["total_jobs"])
	}
	if _, ok := stats["job.stats-job.next_run"]; !ok {
		t.Error("missing next_run stat")
	}
}

func TestSchedulerStartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long-running test in short mode")
	}
	exec := &mockExecutor{}
	s := NewScheduler(exec)
	s.AddJob(&Job{Enabled: true, Name: "quick", Schedule: "* * * * *", Prompt: "every minute"})

	s.Start()

	// Wait for the next minute boundary + 3 seconds
	now := time.Now()
	nextMinute := now.Truncate(time.Minute).Add(time.Minute)
	wait := nextMinute.Sub(now) + 3*time.Second
	if wait > 65*time.Second {
		t.Skip("too close to previous minute boundary")
	}
	t.Logf("waiting %v until %s", wait, nextMinute.Add(3*time.Second).Format("15:04:05"))
	time.Sleep(wait)
	s.Stop()

	exec.mu.Lock()
	count := len(exec.results)
	exec.mu.Unlock()
	if count < 1 {
		t.Error("expected at least 1 execution")
	}
	t.Logf("executed %d times", count)
}
