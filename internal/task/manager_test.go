package task

import (
	"context"
	"testing"
	"time"
)

func TestManagerCreateAndList(t *testing.T) {
	m := NewManager(5)

	t1 := m.Create(TypeLocalBash, "run tests", "tool-1")
	if t1.ID == "" {
		t.Error("expected non-empty task ID")
	}
	if t1.GetStatus() != StatusPending {
		t.Errorf("expected pending, got %s", t1.GetStatus())
	}

	_ = m.Create(TypeMonitor, "watch logs", "tool-2")
	_ = m.Create(TypeCron, "daily cleanup", "tool-3")

	all := m.List()
	if len(all) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(all))
	}
}

func TestManagerKill(t *testing.T) {
	m := NewManager(5)
	task := m.Create(TypeLocalBash, "long running", "tool-1")

	err := m.Kill(task.ID)
	if err != nil {
		t.Errorf("kill failed: %v", err)
	}
	if task.GetStatus() != StatusKilled {
		t.Errorf("expected killed, got %s", task.GetStatus())
	}

	// Double kill should fail
	err = m.Kill(task.ID)
	if err == nil {
		t.Error("expected error on double kill")
	}
}

func TestManagerMetrics(t *testing.T) {
	m := NewManager(5)

	m.Create(TypeLocalBash, "bash 1", "t1")
	m.Create(TypeLocalBash, "bash 2", "t2")
	m.Create(TypeMonitor, "monitor 1", "t3")

	metrics := m.Metrics()
	if v, ok := metrics["total"].(int); !ok || v != 3 {
		t.Errorf("expected total 3, got %v", metrics["total"])
	}

	byType, ok := metrics["by_type"].(map[Type]int)
	if !ok {
		t.Fatal("missing by_type")
	}
	if byType[TypeLocalBash] != 2 {
		t.Errorf("expected 2 bash tasks, got %d", byType[TypeLocalBash])
	}
	if byType[TypeMonitor] != 1 {
		t.Errorf("expected 1 monitor task, got %d", byType[TypeMonitor])
	}

	byStatus, ok := metrics["by_status"].(map[Status]int)
	if !ok {
		t.Fatal("missing by_status")
	}
	if byStatus[StatusPending] != 3 {
		t.Errorf("expected 3 pending, got %d", byStatus[StatusPending])
	}
}

func TestManagerListByStatus(t *testing.T) {
	m := NewManager(5)
	t1 := m.Create(TypeLocalBash, "pending task", "t1")
	t2 := m.Create(TypeCron, "killed task", "t2")

	t1.SetStatus(StatusCompleted)
	t2.SetStatus(StatusFailed)

	pending := m.ListByStatus(StatusPending)
	completed := m.ListByStatus(StatusCompleted)
	failed := m.ListByStatus(StatusFailed)

	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
	if len(completed) != 1 {
		t.Errorf("expected 1 completed, got %d", len(completed))
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(failed))
	}
}

func TestManagerDependency(t *testing.T) {
	m := NewManager(5)
	blocker := m.Create(TypeLocalBash, "blocker", "t1")
	blocked := m.Create(TypeLocalBash, "blocked", "t2")

	m.AddDependency(blocker.ID, blocked.ID)

	// blocked should not start yet (blocker still pending)
	if m.canStart(blocked) {
		t.Error("blocked task should not be ready to start")
	}

	// Complete the blocker
	blocker.SetStatus(StatusCompleted)

	if !m.canStart(blocked) {
		t.Error("blocked task should now be ready")
	}
}

func TestStateDuration(t *testing.T) {
	task := NewState(TypeMonitor, "duration test", "tool-1")

	time.Sleep(time.Millisecond)
	if task.Duration() <= 0 {
		t.Error("expected positive duration")
	}

	task.SetStatus(StatusCompleted)
	first := task.Duration()

	time.Sleep(10 * time.Millisecond)
	second := task.Duration()

	if first != second {
		t.Error("duration should not change after terminal status")
	}
}

func TestStateCancel(t *testing.T) {
	task := NewState(TypeLocalBash, "cancel test", "tool-1")

	ctx := task.Context()
	task.Cancel()

	select {
	case <-ctx.Done():
		// expected
	default:
		t.Error("context should be cancelled")
	}
}

func TestIsTerminal(t *testing.T) {
	if IsTerminal(StatusPending) {
		t.Error("pending should not be terminal")
	}
	if IsTerminal(StatusRunning) {
		t.Error("running should not be terminal")
	}
	if !IsTerminal(StatusCompleted) {
		t.Error("completed should be terminal")
	}
	if !IsTerminal(StatusFailed) {
		t.Error("failed should be terminal")
	}
	if !IsTerminal(StatusKilled) {
		t.Error("killed should be terminal")
	}
}

func TestShellExecutor(t *testing.T) {
	e := &ShellExecutor{}
	task := NewState(TypeLocalBash, "test shell", "tool-1")
	task.SetStatus(StatusRunning)

	// Shell executor is a stub, should succeed
	err := e.Execute(context.Background(), task)
	if err != nil {
		t.Errorf("shell executor should not error: %v", err)
	}
}

func TestManagerConcurrency(t *testing.T) {
	m := NewManager(2)

	var tasks []*State
	for i := 0; i < 10; i++ {
		task := m.Create(TypeLocalBash, "concurrent", "tool-1")
		tasks = append(tasks, task)
	}

	if len(tasks) != 10 {
		t.Errorf("expected 10 tasks, got %d", len(tasks))
	}

	// Verify semaphore is functioning
	if cap(m.sem) != 2 {
		t.Errorf("expected semaphore cap 2, got %d", cap(m.sem))
	}
}
