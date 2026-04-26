package task

import (
	"context"
	"crypto/rand"
	"sync"
	"time"
)

// Type represents the kind of task.
type Type string

const (
	TypeLocalBash  Type = "local_bash"
	TypeLocalAgent Type = "local_agent"
	TypeRemoteAgent Type = "remote_agent"
	TypeCron       Type = "cron_task"
	TypeMonitor    Type = "monitor_task"
)

// Status represents task lifecycle state.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusKilled    Status = "killed"
)

// IsTerminal returns true when a task will not transition further.
func IsTerminal(s Status) bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusKilled
}

// State holds the current state of a task.
type State struct {
	ID          string
	Type        Type
	Status      Status
	Description string
	ToolUseID   string
	StartTime   time.Time
	EndTime     time.Time
	OutputFile  string
	RetryCount  int
	MaxRetries  int
	Error       string
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewState creates a new task state.
func NewState(taskType Type, description string, toolUseID string) *State {
	ctx, cancel := context.WithCancel(context.Background())
	return &State{
		ID:          generateID(taskType),
		Type:        taskType,
		Status:      StatusPending,
		Description: description,
		ToolUseID:   toolUseID,
		StartTime:   time.Now(),
		MaxRetries:  3,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Context returns the task's context (cancelled on kill).
func (s *State) Context() context.Context {
	return s.ctx
}

// Cancel cancels the task's context.
func (s *State) Cancel() {
	s.cancel()
}

// SetStatus updates the task status.
func (s *State) SetStatus(status Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
	if IsTerminal(status) {
		s.EndTime = time.Now()
	}
}

// GetStatus returns the current status.
func (s *State) GetStatus() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status
}

// Duration returns the elapsed time.
func (s *State) Duration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.EndTime.IsZero() {
		return time.Since(s.StartTime)
	}
	return s.EndTime.Sub(s.StartTime)
}

// Metrics collects per-task monitoring data.
type Metrics struct {
	TaskID     string
	TaskType   Type
	Status     Status
	Duration   time.Duration
	RetryCount int
	ToolCalls  int
	TokenUsed  int
	ExitCode   int
	ErrorMsg   string
}

// ToMetrics converts a task state to metrics.
func (s *State) ToMetrics() Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Metrics{
		TaskID:     s.ID,
		TaskType:   s.Type,
		Status:     s.Status,
		Duration:   s.Duration(),
		RetryCount: s.RetryCount,
	}
}

// ID prefixes and generation
var idPrefixes = map[Type]string{
	TypeLocalBash:  "b",
	TypeLocalAgent: "a",
	TypeRemoteAgent: "r",
	TypeCron:       "c",
	TypeMonitor:    "m",
}

const idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

func generateID(t Type) string {
	prefix := idPrefixes[t]
	if prefix == "" {
		prefix = "x"
	}
	b := make([]byte, 8)
	rand.Read(b)
	id := prefix
	for i := 0; i < 8; i++ {
		id += string(idAlphabet[int(b[i])%len(idAlphabet)])
	}
	return id
}
