package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/openclaw/gclaw/internal/autonomous"
	"github.com/openclaw/gclaw/internal/channel"
)

// REPLAdapter wraps stdin/stdout as a Channel, making REPL a first-class platform.
// It is essentially a no-op channel: Start/Stop are no-ops, Send writes to stdout.
type REPLAdapter struct {
	connected bool
	userID    string
	lastMsgAt time.Time
	msgCount  int64
}

// NewREPL creates a REPL adapter.
func NewREPL() *REPLAdapter {
	return &REPLAdapter{connected: true, userID: "repl-user"}
}

func (r *REPLAdapter) ID() string { return "repl" }

func (r *REPLAdapter) Start(ctx context.Context, bus *autonomous.EventBus) error {
	r.connected = true
	return nil
}

func (r *REPLAdapter) Stop() error {
	r.connected = false
	return nil
}

func (r *REPLAdapter) Send(ctx context.Context, to, text string) error {
	r.lastMsgAt = time.Now()
	r.msgCount++
	fmt.Println(text)
	return nil
}

func (r *REPLAdapter) Status() channel.Status {
	return channel.Status{
		Connected: r.connected,
		AccountID: "repl",
		UserID:    r.userID,
		LastMsgAt: r.lastMsgAt,
		MsgCount:  r.msgCount,
	}
}
