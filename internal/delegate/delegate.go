package delegate

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AgentRunner abstracts the ability to run a turn.
type AgentRunner interface {
	Run(ctx context.Context, prompt string) (string, error)
	Reset()
}

// AgentFactory creates fresh AgentRunners for sub-tasks.
type AgentFactory func() AgentRunner

// Dispatcher manages sub-agent goroutines.
type Dispatcher struct {
	factory      AgentFactory
	sem          chan struct{}
	maxDepth     int
	blockedTools map[string]bool
	mu           sync.RWMutex
	jobs         map[string]*JobState
}

// JobState tracks a delegate job.
type JobState struct {
	ID       string
	Goal     string
	Depth    int
	Result   *DelegateResult
	Done     chan struct{}
	StartAt  time.Time
	FinishAt time.Time
}

// DelegateResult is the outcome of a delegated task.
type DelegateResult struct {
	Output string
	Turns  int
	Error  error
}

// DispatcherStats holds dispatcher metrics.
type DispatcherStats struct {
	ActiveJobs    int
	TotalJobs     int
	QueueLength   int
	MaxConcurrent int
}

// NewDispatcher creates a sub-agent dispatcher.
func NewDispatcher(factory AgentFactory, maxConcurrent, maxDepth int) *Dispatcher {
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	if maxDepth <= 0 {
		maxDepth = 2
	}
	return &Dispatcher{
		factory:  factory,
		sem:      make(chan struct{}, maxConcurrent),
		maxDepth: maxDepth,
		blockedTools: map[string]bool{
			"delegate_task": true,
		},
		jobs: make(map[string]*JobState),
	}
}

// Delegate runs a sub-task in a goroutine and returns the result.
func (d *Dispatcher) Delegate(ctx context.Context, goal string, depth int, timeout time.Duration) (*DelegateResult, error) {
	if depth > d.maxDepth {
		return nil, fmt.Errorf("max depth exceeded: %d > %d", depth, d.maxDepth)
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	jobID := fmt.Sprintf("delegate-%d", time.Now().UnixNano())
	job := &JobState{
		ID:      jobID,
		Goal:    goal,
		Depth:   depth,
		Done:    make(chan struct{}),
		StartAt: time.Now(),
	}

	d.mu.Lock()
	d.jobs[jobID] = job
	d.mu.Unlock()

	// Acquire semaphore
	select {
	case d.sem <- struct{}{}:
	default:
		select {
		case d.sem <- struct{}{}:
		case <-ctx.Done():
			d.mu.Lock()
			delete(d.jobs, jobID)
			d.mu.Unlock()
			return nil, ctx.Err()
		}
	}

	go func() {
		defer func() { <-d.sem }()
		defer close(job.Done)

		execCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		runner := d.factory()
		defer runner.Reset()

		output, err := runner.Run(execCtx, goal)
		job.Result = &DelegateResult{
			Output: output,
			Error:  err,
		}
		job.FinishAt = time.Now()
	}()

	// Wait for completion
	select {
	case <-job.Done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	d.mu.Lock()
	delete(d.jobs, jobID)
	d.mu.Unlock()

	if job.Result.Error != nil {
		return nil, job.Result.Error
	}
	return job.Result, nil
}

// Stats returns dispatcher metrics.
func (d *Dispatcher) Stats() DispatcherStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	active := 0
	for _, j := range d.jobs {
		if j.Result == nil {
			active++
		}
	}
	return DispatcherStats{
		ActiveJobs:    active,
		TotalJobs:     len(d.jobs),
		QueueLength:   len(d.sem),
		MaxConcurrent: cap(d.sem),
	}
}
