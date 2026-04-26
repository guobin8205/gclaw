package task

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/openclaw/gclaw/internal/model"
	"github.com/openclaw/gclaw/internal/tool"
)

// Executor executes a task and returns its output.
type Executor interface {
	Execute(ctx context.Context, task *State) error
}

// Manager handles task lifecycle and scheduling.
type Manager struct {
	tasks     map[string]*State
	executors map[Type]Executor
	mu        sync.RWMutex

	maxConcurrent int
	sem           chan struct{}

	// Dependency management
	blockedBy map[string][]string // taskID → list of taskIDs blocked by this
	blocks    map[string][]string // taskID → list of taskIDs that block this
	depMu     sync.RWMutex

	// Background task tracking
	bgWg sync.WaitGroup
}

// NewManager creates a task manager.
func NewManager(maxConcurrent int) *Manager {
	if maxConcurrent <= 0 {
		maxConcurrent = 10
	}
	return &Manager{
		tasks:         make(map[string]*State),
		executors:     make(map[Type]Executor),
		maxConcurrent: maxConcurrent,
		sem:           make(chan struct{}, maxConcurrent),
		blockedBy:     make(map[string][]string),
		blocks:        make(map[string][]string),
	}
}

// RegisterExecutor adds an executor for a task type.
func (m *Manager) RegisterExecutor(taskType Type, exec Executor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executors[taskType] = exec
}

// Create creates a new task and adds it to the manager.
func (m *Manager) Create(taskType Type, description string, toolUseID string) *State {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := NewState(taskType, description, toolUseID)
	m.tasks[task.ID] = task
	return task
}

// Start begins executing a task asynchronously.
func (m *Manager) Start(task *State) {
	exec, ok := m.executors[task.Type]
	if !ok {
		task.SetStatus(StatusFailed)
		task.Error = fmt.Sprintf("no executor for task type: %s", task.Type)
		return
	}

	// Check dependencies
	if !m.canStart(task) {
		task.SetStatus(StatusPending)
		return
	}

	m.run(task, exec)
}

// run starts execution in a goroutine.
func (m *Manager) run(task *State, exec Executor) {
	m.sem <- struct{}{}
	task.SetStatus(StatusRunning)

	m.bgWg.Add(1)
	go func() {
		defer m.bgWg.Done()
		defer func() { <-m.sem }()

		var lastErr error
		for attempt := 0; attempt <= task.MaxRetries; attempt++ {
			task.RetryCount = attempt

			err := exec.Execute(task.Context(), task)
			if err == nil {
				task.SetStatus(StatusCompleted)
				m.unblockDependents(task)
				return
			}

			lastErr = err
			slog.Warn("task failed, retrying",
				"task", task.ID,
				"attempt", attempt+1,
				"max", task.MaxRetries,
				"error", err,
			)
		}

		task.SetStatus(StatusFailed)
		task.Error = fmt.Sprintf("failed after %d retries: %v", task.MaxRetries, lastErr)
	}()
}

// Kill terminates a running task.
func (m *Manager) Kill(taskID string) error {
	m.mu.RLock()
	task, ok := m.tasks[taskID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if IsTerminal(task.GetStatus()) {
		return fmt.Errorf("task already terminal: %s", task.GetStatus())
	}

	task.Cancel()
	task.SetStatus(StatusKilled)
	return nil
}

// Get returns a task by ID.
func (m *Manager) Get(taskID string) (*State, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[taskID]
	return t, ok
}

// List returns all tasks.
func (m *Manager) List() []*State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*State, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, t)
	}
	return out
}

// ListByStatus returns tasks filtered by status.
func (m *Manager) ListByStatus(status Status) []*State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*State
	for _, t := range m.tasks {
		if t.GetStatus() == status {
			out = append(out, t)
		}
	}
	return out
}

// Metrics returns aggregated task metrics.
func (m *Manager) Metrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	byType := make(map[Type]int)
	byStatus := make(map[Status]int)

	for _, t := range m.tasks {
		byType[t.Type]++
		byStatus[t.GetStatus()]++
	}

	return map[string]interface{}{
		"total":     len(m.tasks),
		"by_type":   byType,
		"by_status": byStatus,
	}
}

// Wait blocks until all running tasks complete.
func (m *Manager) Wait() {
	m.bgWg.Wait()
}

// WaitWithTimeout blocks until all tasks complete or timeout.
func (m *Manager) WaitWithTimeout(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		m.bgWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// AddDependency sets up a dependency: blockedID cannot start until blockerID completes.
func (m *Manager) AddDependency(blockerID, blockedID string) {
	m.depMu.Lock()
	defer m.depMu.Unlock()
	m.blockedBy[blockerID] = append(m.blockedBy[blockerID], blockedID)
	m.blocks[blockedID] = append(m.blocks[blockedID], blockerID)
}

func (m *Manager) canStart(task *State) bool {
	m.depMu.RLock()
	defer m.depMu.RUnlock()

	blockers := m.blocks[task.ID]
	for _, blockerID := range blockers {
		t, ok := m.Get(blockerID)
		if !ok || !IsTerminal(t.GetStatus()) {
			return false
		}
	}
	return true
}

func (m *Manager) unblockDependents(task *State) {
	m.depMu.RLock()
	dependents := make([]string, len(m.blockedBy[task.ID]))
	copy(dependents, m.blockedBy[task.ID])
	m.depMu.RUnlock()

	for _, depID := range dependents {
		dep, ok := m.Get(depID)
		if !ok {
			continue
		}
		if dep.GetStatus() == StatusPending && m.canStart(dep) {
			exec, ok := m.executors[dep.Type]
			if ok {
				m.run(dep, exec)
			}
		}
	}
}

// ShellExecutor runs bash commands as tasks.
type ShellExecutor struct{}

func (e *ShellExecutor) Execute(ctx context.Context, task *State) error {
	slog.Debug("shell task executed (stub)", "task", task.ID, "desc", task.Description)
	return nil
}

// AgentExecutor runs sub-agent loops as tasks.
type AgentExecutor struct {
	ToolRegistry *tool.Registry
	Model        model.Model
}

func (e *AgentExecutor) Execute(ctx context.Context, task *State) error {
	slog.Debug("agent task executed (stub)", "task", task.ID, "desc", task.Description)
	return nil
}
