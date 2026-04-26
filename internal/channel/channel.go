package channel

import (
	"context"
	"time"

	"github.com/openclaw/gclaw/internal/autonomous"
)

// Channel is a messaging channel that bridges GClaw to external platforms.
type Channel interface {
	ID() string
	Start(ctx context.Context, bus *autonomous.EventBus) error
	Stop() error
	Send(ctx context.Context, to, text string) error
	Status() Status
}

// Status holds runtime state for a channel.
type Status struct {
	Connected bool
	AccountID string
	UserID    string
	LastMsgAt time.Time
	MsgCount  int64
}
