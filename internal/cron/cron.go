package cron

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// Job represents a scheduled cron job.
type Job struct {
	Name          string    // unique name
	Schedule      string    // 5-field cron expression
	Prompt        string    // what to tell the agent
	Enabled       bool      // can be disabled at runtime
	NotifyWeixin  bool      // push result to WeChat on completion

	LastRun  time.Time
	NextRun  time.Time
	RunCount int64

	OnResult func(name, prompt, response string) // called after job completes

	mu sync.RWMutex
}

// Executor is the interface for running a cron job's prompt.
type Executor interface {
	Submit(ctx context.Context, prompt string) (string, error)
	IsBusy() bool
	Reset()
}

// Scheduler manages and executes cron jobs on schedule.
type Scheduler struct {
	jobs     map[string]*Job
	executor Executor
	location *time.Location

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// NewScheduler creates a cron scheduler.
func NewScheduler(executor Executor) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		jobs:     make(map[string]*Job),
		executor: executor,
		location: time.Local,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// AddJob registers a cron job and computes its next run time.
func (s *Scheduler) AddJob(job *Job) error {
	if job.Name == "" {
		return fmt.Errorf("job name is required")
	}
	if job.Schedule == "" {
		return fmt.Errorf("job %s: schedule is required", job.Name)
	}
	if _, err := Parse(job.Schedule); err != nil {
		return fmt.Errorf("job %s: %w", job.Name, err)
	}

	job.NextRun = nextRun(job.Schedule, time.Now(), s.location)
	s.mu.Lock()
	s.jobs[job.Name] = job
	s.mu.Unlock()

	slog.Info("cron job added", "name", job.Name, "schedule", job.Schedule, "next", job.NextRun.Format(time.RFC3339))
	return nil
}

// RemoveJob removes a job by name.
func (s *Scheduler) RemoveJob(name string) {
	s.mu.Lock()
	delete(s.jobs, name)
	s.mu.Unlock()
}

// Jobs returns a list of all registered jobs.
func (s *Scheduler) Jobs() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	sort.Slice(jobs, func(i, k int) bool {
		return jobs[i].Name < jobs[k].Name
	})
	return jobs
}

// RunNow immediately executes a job by name, bypassing schedule.
func (s *Scheduler) RunNow(ctx context.Context, name string) (string, error) {
	s.mu.RLock()
	job, ok := s.jobs[name]
	s.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("job %q not found", name)
	}
	if !job.Enabled {
		return "", fmt.Errorf("job %q is disabled", name)
	}

	slog.Info("cron: RunNow triggered", "name", name)
	result, err := s.executor.Submit(ctx, job.Prompt)
	if err != nil {
		return "", err
	}

	job.RunCount++
	if job.OnResult != nil {
		job.OnResult(job.Name, job.Prompt, result)
	}
	return result, nil
}

// Start begins the cron loop, checking every second for due jobs.
func (s *Scheduler) Start() {
	slog.Info("cron scheduler starting", "jobs", len(s.jobs))
	s.wg.Add(1)
	go s.run()
}

func (s *Scheduler) run() {
	defer s.wg.Done()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			slog.Debug("cron scheduler stopped")
			return
		case now := <-ticker.C:
			s.checkDue(now)
		}
	}
}

func (s *Scheduler) checkDue(now time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, job := range s.jobs {
		job.mu.RLock()
		due := !job.NextRun.IsZero() && !now.Before(job.NextRun)
		enabled := job.Enabled
		job.mu.RUnlock()

		if !due || !enabled {
			continue
		}

		if s.executor.IsBusy() {
			slog.Debug("cron: executor busy, delaying", "job", job.Name)
			continue
		}

		slog.Info("cron job triggered", "name", job.Name, "schedule", job.Schedule)
		go s.executeJob(job)
	}
}

func (s *Scheduler) executeJob(job *Job) {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	s.executor.Reset()
	resp, err := s.executor.Submit(ctx, job.Prompt)
	if err != nil {
		slog.Error("cron job failed", "name", job.Name, "error", err)
	}

	job.mu.Lock()
	job.LastRun = time.Now()
	job.NextRun = nextRun(job.Schedule, time.Now(), s.location)
	job.RunCount++
	job.mu.Unlock()

	if resp != "" {
		slog.Info("cron job completed", "name", job.Name, "response_len", len(resp))
		if job.OnResult != nil {
			job.OnResult(job.Name, job.Prompt, resp)
		}
	}
}

// Stop gracefully shuts down the cron scheduler.
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
	slog.Info("cron scheduler stopped")
}

// Stats returns current scheduler statistics.
func (s *Scheduler) Stats() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := map[string]any{
		"total_jobs": len(s.jobs),
	}
	for name, job := range s.jobs {
		job.mu.RLock()
		stats["job."+name+".next_run"] = job.NextRun.Format(time.RFC3339)
		stats["job."+name+".last_run"] = job.LastRun.Format(time.RFC3339)
		stats["job."+name+".run_count"] = job.RunCount
		stats["job."+name+".enabled"] = job.Enabled
		job.mu.RUnlock()
	}
	return stats
}
