package autonomous

import (
	"sync"
	"time"
)

// EventType classifies what triggered the agent.
type EventType int

const (
	EventTick    EventType = iota // periodic heartbeat
	EventFile                     // file system change
	EventWebhook                  // external webhook
	EventCron                     // scheduled cron job
	EventUser                     // user input
	EventWake                     // wake from sleep
)

func (e EventType) String() string {
	switch e {
	case EventTick:
		return "tick"
	case EventFile:
		return "file"
	case EventWebhook:
		return "webhook"
	case EventCron:
		return "cron"
	case EventUser:
		return "user"
	case EventWake:
		return "wake"
	}
	return "unknown"
}

// Event represents a trigger that wakes or activates the agent.
type Event struct {
	Type    EventType
	Source  string // "ticker", "fsnotify", "webhook:github", etc.
	Payload string // the message to inject, empty for plain tick
	Time    time.Time
}

// EventBus is a pub/sub event system for autonomous triggers.
type EventBus struct {
	subscribers []chan Event
	mu          sync.RWMutex
	closed      bool
}

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	return &EventBus{}
}

// Subscribe returns a channel that receives events.
func (b *EventBus) Subscribe() <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	ch := make(chan Event, 64)
	b.subscribers = append(b.subscribers, ch)
	return ch
}

// Unsubscribe removes a subscriber channel.
func (b *EventBus) Unsubscribe(ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, sub := range b.subscribers {
		if sub == ch {
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			close(sub)
			return
		}
	}
}

// Publish sends an event to all subscribers.
func (b *EventBus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for _, sub := range b.subscribers {
		select {
		case sub <- event:
		default:
			// subscriber too slow, drop event
		}
	}
}

// Close shuts down the event bus and all subscriber channels.
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for _, sub := range b.subscribers {
		close(sub)
	}
	b.subscribers = nil
}

// Ticker wraps a time.Ticker for autonomous heartbeat.
type Ticker struct {
	ticker   *time.Ticker
	interval time.Duration
	bus      *EventBus
	stopCh   chan struct{}
	stopped  bool
	mu       sync.Mutex
}

// NewTicker creates a heartbeat ticker.
func NewTicker(interval time.Duration, bus *EventBus) *Ticker {
	return &Ticker{
		interval: interval,
		bus:      bus,
		stopCh:   make(chan struct{}),
	}
}

// Start begins ticking. Each tick publishes a Tick event on the bus.
func (t *Ticker) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ticker != nil {
		return
	}
	t.ticker = time.NewTicker(t.interval)
	go func() {
		for {
			select {
			case <-t.ticker.C:
				t.bus.Publish(Event{
					Type:   EventTick,
					Source: "ticker",
					Time:   time.Now(),
				})
			case <-t.stopCh:
				return
			}
		}
	}()
}

// Stop halts the ticker.
func (t *Ticker) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	t.stopped = true
	if t.ticker != nil {
		t.ticker.Stop()
	}
	close(t.stopCh)
}

// Interval returns the tick interval.
func (t *Ticker) Interval() time.Duration {
	return t.interval
}

// Sleeper manages idle sleep and wake-up.
type Sleeper struct {
	sleepCh   chan time.Duration // agent requests sleep
	wakeCh    chan struct{}      // external wake signal
	bus       *EventBus
	isSleeping bool
	mu        sync.Mutex
}

// NewSleeper creates a sleep manager.
func NewSleeper(bus *EventBus) *Sleeper {
	return &Sleeper{
		sleepCh: make(chan time.Duration, 1),
		wakeCh:  make(chan struct{}, 1),
		bus:     bus,
	}
}

// Sleep pauses for duration or until woken. Returns the reason for waking.
func (s *Sleeper) Sleep(d time.Duration) (string, bool) {
	s.mu.Lock()
	s.isSleeping = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.isSleeping = false
		s.mu.Unlock()
	}()

	timer := time.NewTimer(d)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			return "sleep completed", true
		case <-s.wakeCh:
			return "woken by external event", false
		}
	}
}

// Wake interrupts sleep if the agent is sleeping.
func (s *Sleeper) Wake() {
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}

// IsSleeping returns whether the agent is currently sleeping.
func (s *Sleeper) IsSleeping() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isSleeping
}

// RequestSleep channels a sleep request (used by SleepTool).
func (s *Sleeper) RequestSleep(d time.Duration) {
	select {
	case s.sleepCh <- d:
	default:
	}
}

// SleepRequest returns the channel for sleep requests.
func (s *Sleeper) SleepRequest() <-chan time.Duration {
	return s.sleepCh
}
