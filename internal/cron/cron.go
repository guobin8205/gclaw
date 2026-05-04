package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
	Script        string    // relative path to script in scripts_dir

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
	jobs          map[string]*Job
	executor      Executor
	location      *time.Location
	scriptTimeout time.Duration
	scriptsDir    string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// NewScheduler creates a cron scheduler.
func NewScheduler(executor Executor, opts ...SchedulerOption) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		jobs:          make(map[string]*Job),
		executor:      executor,
		location:      time.Local,
		scriptTimeout: 120 * time.Second,
		ctx:           ctx,
		cancel:        cancel,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SchedulerOption configures a Scheduler.
type SchedulerOption func(*Scheduler)

// WithScriptTimeout sets the timeout for script execution.
func WithScriptTimeout(d time.Duration) SchedulerOption {
	return func(s *Scheduler) {
		s.scriptTimeout = d
	}
}

// WithScriptsDir sets the directory where scripts are located.
func WithScriptsDir(dir string) SchedulerOption {
	return func(s *Scheduler) {
		s.scriptsDir = dir
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

// PauseJob disables a job without removing it.
func (s *Scheduler) PauseJob(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[name]
	if !ok {
		return fmt.Errorf("job %q not found", name)
	}
	job.Enabled = false
	slog.Info("cron job paused", "name", name)
	return nil
}

// ResumeJob re-enables a paused job.
func (s *Scheduler) ResumeJob(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[name]
	if !ok {
		return fmt.Errorf("job %q not found", name)
	}
	job.Enabled = true
	job.NextRun = nextRun(job.Schedule, time.Now(), s.location)
	slog.Info("cron job resumed", "name", name)
	return nil
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

func (s *Scheduler) runScript(scriptPath string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if s.scriptsDir != "" {
		scriptPath = filepath.Join(s.scriptsDir, scriptPath)
	}

	// Ensure script exists
	if _, err := os.Stat(scriptPath); err != nil {
		return "", fmt.Errorf("script not found: %w", err)
	}

	ext := filepath.Ext(scriptPath)
	var cmd *exec.Cmd
	switch ext {
	case ".py":
		// Try python3 first, then python
		if _, err := exec.LookPath("python3"); err == nil {
			cmd = exec.CommandContext(ctx, "python3", scriptPath)
		} else if _, err := exec.LookPath("python"); err == nil {
			cmd = exec.CommandContext(ctx, "python", scriptPath)
		} else {
			return "", fmt.Errorf("no python interpreter found")
		}
	case ".sh":
		// Try sh first, then bash
		if _, err := exec.LookPath("sh"); err == nil {
			cmd = exec.CommandContext(ctx, "sh", scriptPath)
		} else if _, err := exec.LookPath("bash"); err == nil {
			cmd = exec.CommandContext(ctx, "bash", scriptPath)
		} else {
			return "", fmt.Errorf("no shell found")
		}
	case ".bat", ".cmd":
		cmd = exec.CommandContext(ctx, "cmd", "/c", scriptPath)
	default:
		// For unrecognized extensions, try executing directly if executable
		cmd = exec.CommandContext(ctx, scriptPath)
	}

	out, err := cmd.CombinedOutput()
	output := string(out)
	if err != nil {
		return output, fmt.Errorf("script exited with error: %w", err)
	}
	return output, nil
}

func (s *Scheduler) executeJob(job *Job) {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	var scriptOutput string
	var skipAgent bool

	if job.Script != "" {
		out, err := s.runScript(job.Script, s.scriptTimeout)
		if err != nil {
			slog.Error("cron script failed", "name", job.Name, "error", err)
			// Still continue with agent but include error in prompt
			scriptOutput = fmt.Sprintf("Script error: %v\n%s", err, out)
		} else {
			scriptOutput = out
			// Check wake gate
			if strings.TrimSpace(scriptOutput) == "" {
				skipAgent = true
			} else {
				lines := strings.Split(strings.TrimSpace(scriptOutput), "\n")
				lastLine := lines[len(lines)-1]
				if strings.Contains(lastLine, `"wakeAgent"`) {
					var wg struct{ WakeAgent bool `json:"wakeAgent"` }
					if json.Unmarshal([]byte(lastLine), &wg) == nil && !wg.WakeAgent {
						skipAgent = true
					}
				}
			}
		}
	}

	var resp string
	if !skipAgent {
		prompt := job.Prompt
		if scriptOutput != "" {
			prompt = fmt.Sprintf("%s\n\n## Script Output\n%s", prompt, scriptOutput)
		}
		s.executor.Reset()
		var err error
		resp, err = s.executor.Submit(ctx, prompt)
		if err != nil {
			slog.Error("cron job failed", "name", job.Name, "error", err)
		}
	} else {
		resp = scriptOutput
		slog.Info("cron job skipped agent (no-agent mode)", "name", job.Name)
	}

	job.mu.Lock()
	job.LastRun = time.Now()
	job.NextRun = nextRun(job.Schedule, time.Now(), s.location)
	job.RunCount++
	job.mu.Unlock()

	if resp != "" {
		slog.Info("cron job completed", "name", job.Name, "response_len", len(resp), "no_agent", skipAgent)
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
