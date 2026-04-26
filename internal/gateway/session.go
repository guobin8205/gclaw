package gateway

// SessionSource identifies where a message came from across platforms.
type SessionSource struct {
	Platform string // "repl" | "weixin" | "telegram" | ...
	ChatID   string
	ChatName string
	UserID   string
	ThreadID string
}

// Message is a cross-platform normalized message.
type Message struct {
	Source SessionSource
	Text   string
}
