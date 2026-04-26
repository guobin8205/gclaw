package autonomous

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// AgentLoop is the interface the scheduler uses to drive the agent.
type AgentLoop interface {
	Submit(ctx context.Context, message string) (string, error)
	IsBusy() bool
	Interrupt(message string)
}

// Scheduler orchestrates the autonomous agent lifecycle.
type Scheduler struct {
	cfg     Config
	bus     *EventBus
	ticker  *Ticker
	sleeper *Sleeper

	agent AgentLoop

	ticksSinceLastWork int
	totalTicks         int64
	mu                 sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewScheduler creates an autonomous scheduler.
func NewScheduler(cfg Config, agent AgentLoop) *Scheduler {
	bus := NewEventBus()
	ctx, cancel := context.WithCancel(context.Background())

	s := &Scheduler{
		cfg:     cfg,
		bus:     bus,
		sleeper: NewSleeper(bus),
		agent:   agent,
		ctx:     ctx,
		cancel:  cancel,
	}

	if cfg.Level >= Semi && cfg.TickInterval > 0 {
		s.ticker = NewTicker(cfg.TickInterval, bus)
	}

	return s
}

// Start begins the autonomous loop.
func (s *Scheduler) Start() {
	slog.Info("autonomous scheduler starting",
		"level", s.cfg.Level,
		"tick_interval", s.cfg.TickInterval,
		"idle_sleep", s.cfg.IdleSleep,
	)

	events := s.bus.Subscribe()
	if events == nil {
		slog.Error("failed to subscribe to event bus")
		return
	}

	if s.ticker != nil {
		s.ticker.Start()
		slog.Debug("ticker started", "interval", s.ticker.Interval())
	}

	s.wg.Add(1)
	go s.run(events)
}

func (s *Scheduler) run(events <-chan Event) {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			slog.Debug("scheduler context done, stopping")
			return

		case event, ok := <-events:
			if !ok {
				slog.Debug("event channel closed, stopping")
				return
			}
			s.handleEvent(event)

		case sleepDuration := <-s.sleeper.SleepRequest():
			slog.Debug("agent requested sleep", "duration", sleepDuration)
			s.handleSleep(sleepDuration)
		}
	}
}

func (s *Scheduler) handleEvent(event Event) {
	s.mu.Lock()
	s.totalTicks++
	s.mu.Unlock()

	slog.Debug("handling event",
		"type", event.Type,
		"source", event.Source,
	)

	prompt := s.buildPrompt(event)
	if prompt == "" {
		return
	}

	// If agent is busy, interrupt for high-priority events instead of skipping
	if s.agent.IsBusy() {
		if event.Type == EventUser || event.Type == EventWebhook {
			s.agent.Interrupt(prompt)
			slog.Info("agent busy, interrupting with event", "type", event.Type)
			return
		}
		slog.Debug("agent busy, skipping event", "type", event.Type)
		return
	}

	s.sleeper.Wake()

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	resp, err := s.agent.Submit(ctx, prompt)
	if err != nil {
		slog.Error("agent submit failed", "event", event.Type, "error", err)
		return
	}

	if resp != "" {
		s.mu.Lock()
		s.ticksSinceLastWork = 0
		s.mu.Unlock()
	} else {
		s.mu.Lock()
		s.ticksSinceLastWork++
		s.mu.Unlock()
	}
}

func (s *Scheduler) buildPrompt(event Event) string {
	switch event.Type {
	case EventTick:
		return s.buildTickPrompt()
	case EventFile:
		return "File system change detected. Check if any actions are needed."
	case EventWebhook:
		if event.Payload != "" {
			return "Webhook event received: " + event.Payload
		}
		return "External webhook event received. Review and act if needed."
	case EventCron:
		if event.Payload != "" {
			return "Scheduled task trigger: " + event.Payload
		}
		return "Scheduled task time reached. Execute the scheduled work."
	case EventWake:
		return "Waking from sleep. Check current state and continue pending work."
	case EventUser:
		return event.Payload
	}
	return ""
}

func (s *Scheduler) buildTickPrompt() string {
	s.mu.Lock()
	idleTicks := s.ticksSinceLastWork
	s.mu.Unlock()

	_ = idleTicks // used in the condition below

	if s.cfg.Level == Full {
		if idleTicks > 5 {
			return "tick: You have been idle for several ticks. If there is no pending work, consider calling SleepTool to conserve resources."
		}
		return "tick: Check for pending work. If nothing needs attention, call SleepTool."
	}

	return "tick: Review progress and determine next steps."
}

func (s *Scheduler) handleSleep(duration time.Duration) {
	reason, completed := s.sleeper.Sleep(duration)
	slog.Debug("sleeper done", "reason", reason, "completed", completed)

	if !completed {
		slog.Debug("agent woken early, waiting for event")
	} else {
		s.bus.Publish(Event{
			Type:   EventWake,
			Source: "sleeper",
			Time:   time.Now(),
		})
	}
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	slog.Info("stopping autonomous scheduler")
	s.cancel()
	if s.ticker != nil {
		s.ticker.Stop()
	}
	s.wg.Wait()
	s.bus.Close()
}

// Publish injects an external event into the scheduler.
func (s *Scheduler) Publish(event Event) {
	s.bus.Publish(event)
}

// Stats returns current scheduler statistics.
func (s *Scheduler) Stats() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	return map[string]any{
		"level":            s.cfg.Level.String(),
		"total_ticks":      s.totalTicks,
		"ticks_since_work": s.ticksSinceLastWork,
		"tick_interval":    s.cfg.TickInterval.String(),
		"idle_sleep":       s.cfg.IdleSleep.String(),
		"is_sleeping":      s.sleeper.IsSleeping(),
		"agent_busy":       s.agent.IsBusy(),
	}
}

// Sleeper returns the sleeper instance for SleepTool wiring.
func (s *Scheduler) Sleeper() *Sleeper {
	return s.sleeper
}
