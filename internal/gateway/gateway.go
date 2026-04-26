package gateway

import (
	"context"
	"sync"

	"github.com/openclaw/gclaw/internal/autonomous"
	"github.com/openclaw/gclaw/internal/channel"
)

// Gateway routes messages between platforms and agents.
type Gateway struct {
	channels map[string]channel.Channel
	bus      *autonomous.EventBus
	mu       sync.RWMutex
}

// New creates a new Gateway.
func New(bus *autonomous.EventBus) *Gateway {
	return &Gateway{
		channels: make(map[string]channel.Channel),
		bus:      bus,
	}
}

// Register adds a platform channel.
func (g *Gateway) Register(name string, ch channel.Channel) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.channels[name] = ch
}

// Start starts all registered channels.
func (g *Gateway) Start(ctx context.Context) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, ch := range g.channels {
		go func(ch channel.Channel) {
			_ = ch.Start(ctx, g.bus)
		}(ch)
	}
	return nil
}

// Stop stops all registered channels.
func (g *Gateway) Stop() {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, ch := range g.channels {
		_ = ch.Stop()
	}
}

// Send sends a message to a specific platform.
func (g *Gateway) Send(ctx context.Context, target SessionSource, text string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	ch, ok := g.channels[target.Platform]
	if !ok {
		return nil
	}
	return ch.Send(ctx, target.UserID, text)
}

// Statuses returns status for all channels.
func (g *Gateway) Statuses() map[string]channel.Status {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make(map[string]channel.Status)
	for name, ch := range g.channels {
		result[name] = ch.Status()
	}
	return result
}
