package delegate

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type mockRunner struct {
	output string
	err    error
	delay  time.Duration
	reset  atomic.Bool
}

func (m *mockRunner) Run(ctx context.Context, prompt string) (string, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if m.err != nil {
		return "", m.err
	}
	return m.output + ":" + prompt, nil
}

func (m *mockRunner) Reset() {
	m.reset.Store(true)
}

func TestSingleDelegate(t *testing.T) {
	factory := func() AgentRunner {
		return &mockRunner{output: "done"}
	}
	d := NewDispatcher(factory, 2, 2)

	ctx := context.Background()
	res, err := d.Delegate(ctx, "test-goal", 1, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Output != "done:test-goal" {
		t.Errorf("expected output 'done:test-goal', got %q", res.Output)
	}
	if res.TaskID == "" {
		t.Error("expected TaskID to be set")
	}
}

func TestBatchDelegateParallel(t *testing.T) {
	factory := func() AgentRunner {
		return &mockRunner{output: "result", delay: 50 * time.Millisecond}
	}
	d := NewDispatcher(factory, 3, 2)

	tasks := []Task{
		{ID: "t1", Goal: "goal-1"},
		{ID: "t2", Goal: "goal-2"},
		{ID: "t3", Goal: "goal-3"},
	}

	ctx := context.Background()
	results, err := d.BatchDelegate(ctx, tasks, 1, time.Minute, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != len(tasks) {
		t.Fatalf("expected %d results, got %d", len(tasks), len(results))
	}

	for i, r := range results {
		expected := fmt.Sprintf("result:goal-%d", i+1)
		if r.Output != expected {
			t.Errorf("result[%d].Output = %q, want %q", i, r.Output, expected)
		}
		if r.TaskID != tasks[i].ID {
			t.Errorf("result[%d].TaskID = %q, want %q", i, r.TaskID, tasks[i].ID)
		}
		if r.Error != nil {
			t.Errorf("result[%d].Error = %v, want nil", i, r.Error)
		}
	}
}

func TestBatchDelegateMaxDepth(t *testing.T) {
	factory := func() AgentRunner {
		return &mockRunner{}
	}
	d := NewDispatcher(factory, 2, 2)

	tasks := []Task{{ID: "t1", Goal: "g1"}}
	ctx := context.Background()
	_, err := d.BatchDelegate(ctx, tasks, 3, time.Minute, nil)
	if err == nil {
		t.Fatal("expected max depth error, got nil")
	}
}

func TestBatchDelegateContextCancellation(t *testing.T) {
	factory := func() AgentRunner {
		return &mockRunner{delay: 5 * time.Second}
	}
	d := NewDispatcher(factory, 3, 2)

	tasks := []Task{
		{ID: "t1", Goal: "goal-1"},
		{ID: "t2", Goal: "goal-2"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	results, err := d.BatchDelegate(ctx, tasks, 1, time.Minute, nil)
	if err != nil {
		t.Fatalf("unexpected error from BatchDelegate: %v", err)
	}

	allCancelled := true
	for i, r := range results {
		if r.Error != context.Canceled {
			allCancelled = false
			t.Logf("result[%d].Error = %v (may be nil if task won the race)", i, r.Error)
		}
	}
	if !allCancelled {
		// It's possible some tasks complete before cancellation depending on goroutine scheduling.
		// As long as at least one was cancelled or we see context errors, the test is acceptable.
		found := false
		for _, r := range results {
			if r.Error == context.Canceled {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected at least one result to have context.Canceled error")
		}
	}
}

func TestBatchDelegateIndexAlignment(t *testing.T) {
	factory := func() AgentRunner {
		return &mockRunner{output: "ok"}
	}
	d := NewDispatcher(factory, 1, 2) // concurrency=1 to force sequential execution

	tasks := []Task{
		{ID: "a", Goal: "alpha"},
		{ID: "b", Goal: "beta"},
		{ID: "c", Goal: "gamma"},
	}

	ctx := context.Background()
	results, err := d.BatchDelegate(ctx, tasks, 1, time.Minute, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i, r := range results {
		if r.TaskID != tasks[i].ID {
			t.Errorf("result[%d].TaskID = %q, want %q", i, r.TaskID, tasks[i].ID)
		}
		if r.Output != "ok:" + tasks[i].Goal {
			t.Errorf("result[%d].Output = %q, want %q", i, r.Output, "ok:" + tasks[i].Goal)
		}
	}
}

func TestBatchDelegateProgress(t *testing.T) {
	factory := func() AgentRunner {
		return &mockRunner{output: "done", delay: 20 * time.Millisecond}
	}
	d := NewDispatcher(factory, 3, 2)

	tasks := []Task{
		{ID: "t1", Goal: "g1"},
		{ID: "t2", Goal: "g2"},
	}

	progressCh := make(chan Progress, 10)
	ctx := context.Background()
	_, err := d.BatchDelegate(ctx, tasks, 1, time.Minute, progressCh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(progressCh)

	started := 0
	completed := 0
	for p := range progressCh {
		switch p.Status {
		case "started":
			started++
		case "completed":
			completed++
		}
	}
	if started != 2 {
		t.Errorf("expected 2 started events, got %d", started)
	}
	if completed != 2 {
		t.Errorf("expected 2 completed events, got %d", completed)
	}
}

func TestBatchDelegatePartialFailure(t *testing.T) {
	callCount := 0
	factory := func() AgentRunner {
		callCount++
		if callCount%2 == 0 {
			return &mockRunner{err: fmt.Errorf("intentional error")}
		}
		return &mockRunner{output: "success"}
	}
	d := NewDispatcher(factory, 3, 2)

	tasks := []Task{
		{ID: "t1", Goal: "g1"},
		{ID: "t2", Goal: "g2"},
		{ID: "t3", Goal: "g3"},
	}

	ctx := context.Background()
	results, err := d.BatchDelegate(ctx, tasks, 1, time.Minute, nil)
	if err != nil {
		t.Fatalf("unexpected error from BatchDelegate: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	successCount := 0
	failCount := 0
	for _, r := range results {
		if r.Error != nil {
			failCount++
		} else {
			successCount++
		}
	}
	if successCount == 0 || failCount == 0 {
		t.Errorf("expected a mix of successes and failures, got %d success, %d failure", successCount, failCount)
	}
}
