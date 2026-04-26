package weixin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/openclaw/gclaw/internal/autonomous"
	"github.com/openclaw/gclaw/internal/channel"
)

// AgentLoop is the minimal agent interface the channel needs.
type AgentLoop interface {
	Submit(ctx context.Context, message string) (string, error)
	IsBusy() bool
}

// Config holds weixin channel configuration.
type Config struct {
	Verbose          bool
	OnMessageHandled func() // called after a WeChat message is fully processed
}

// Channel implements the channel.Channel interface for WeChat.
type Channel struct {
	cfg     Config
	store   *Store
	client  *Client
	agent   AgentLoop
	bus     *autonomous.EventBus

	accountID    string
	userID       string
	connected    bool
	lastMsgAt    time.Time
	lastFromUser    string
	lastContextToken string
	msgCount     int64

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// New creates a new WeChat channel.
func New(cfg Config) (*Channel, error) {
	store, err := NewStore()
	if err != nil {
		return nil, err
	}
	return &Channel{
		cfg:   cfg,
		store: store,
	}, nil
}

// ID returns the channel identifier.
func (ch *Channel) ID() string { return "weixin" }

// Start initializes the weixin channel: loads stored token, logs in via QR if needed,
// then begins the long-poll message loop.
func (ch *Channel) Start(ctx context.Context, bus *autonomous.EventBus) error {
	ch.bus = bus
	ch.ctx, ch.cancel = context.WithCancel(context.Background())

	// Look for an existing account.
	ids, err := ch.store.ListAccountIDs()
	if err != nil {
		return fmt.Errorf("weixin: list accounts: %w", err)
	}

	if len(ids) == 0 {
		fmt.Println("\n=== 微信通道 ===")
		fmt.Println("未找到已绑定的微信账号，请扫描二维码登录：")
		return ch.startLogin()
	}

	// Use the first registered account.
	acc, err := ch.store.LoadAccount(ids[0])
	if err != nil || acc == nil || acc.Token == "" {
		fmt.Println("\n=== 微信通道 ===")
		fmt.Println("已保存的微信账号凭证无效，请重新登录：")
		return ch.startLogin()
	}

	ch.accountID = ids[0]
	ch.userID = acc.UserID
		ch.lastFromUser = acc.ChatUserID
	return ch.startPolling(acc.BaseURL, acc.Token)
}

func (ch *Channel) startLogin() error {
	result, err := LoginWithQR(context.Background(), DefaultBaseURL, func(qrURL string) {
		fmt.Println("\n请用微信扫描以下二维码：")
		qr, qrErr := qrcode.New(qrURL, qrcode.Medium)
			if qrErr == nil {
				fmt.Println(qr.ToSmallString(false))
			} else {
				fmt.Println(qrURL)
			}
		fmt.Println("\n等待扫码确认...")
	})
	if err != nil {
		return fmt.Errorf("weixin login: %w", err)
	}

	// Persist credentials.
	ch.accountID = result.AccountID
	ch.userID = result.UserID

	acc := &AccountData{
		Token:   result.Token,
		BaseURL: result.BaseURL,
		UserID:  result.UserID,
		SavedAt: time.Now().Format(time.RFC3339),
	}
	if err := ch.store.SaveAccount(result.AccountID, acc); err != nil {
		slog.Warn("weixin: save account failed", "error", err)
	}
	if err := ch.store.RegisterAccount(result.AccountID); err != nil {
		slog.Warn("weixin: register account failed", "error", err)
	}

	fmt.Println("\n微信绑定成功！")
	return ch.startPolling(result.BaseURL, result.Token)
}

func (ch *Channel) startPolling(baseURL, token string) error {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	ch.client = NewClient(baseURL, DefaultCDNBaseURL, token)

	// Notify server that we're starting.
	notifyCtx, cancel := context.WithTimeout(ch.ctx, 10*time.Second)
	defer cancel()
	if err := ch.client.NotifyStart(notifyCtx); err != nil {
		slog.Warn("weixin: notifyStart failed", "error", err)
	}

	ch.setConnected(true)
	slog.Info("weixin: polling started", "account", ch.accountID)

	ch.wg.Add(1)
	go ch.pollLoop()

	return nil
}

func (ch *Channel) pollLoop() {
	defer ch.wg.Done()

	buf := ""
	for {
		select {
		case <-ch.ctx.Done():
			slog.Debug("weixin: poll loop stopped")
			return
		default:
		}

		resp, err := ch.client.GetUpdates(ch.ctx, buf, DefaultLongPollMs)
		if err != nil {
			if ch.ctx.Err() != nil {
				return
			}
			slog.Error("weixin: getUpdates failed", "error", err)
			time.Sleep(3 * time.Second)
			continue
		}

		buf = resp.GetUpdatesBuf

		for _, msg := range resp.Msgs {
			ch.handleMessage(msg)
		}
	}
}

func (ch *Channel) handleMessage(msg WeixinMessage) {
	if msg.FromUserID == "" {
		return
	}

	text := extractText(msg.ItemList)
	if text == "" {
		return
	}

	ch.mu.Lock()
	ch.lastMsgAt = time.Now()
	ch.lastFromUser = msg.FromUserID
		ch.lastContextToken = msg.ContextToken
	ch.msgCount++
	ch.mu.Unlock()

	go func() {
		if err := ch.store.SaveChatUserID(ch.accountID, msg.FromUserID); err != nil {
			slog.Warn("weixin: save chat user id failed", "error", err)
		}
	}()

	slog.Info("weixin: message received", "from", msg.FromUserID, "text_len", len(text))

	// If agent is busy, skip — don't queue (simplest approach).
	if ch.agent != nil && ch.agent.IsBusy() {
		slog.Debug("weixin: agent busy, skipping message")
		return
	}

	// Build prompt and submit to agent.
	prompt := fmt.Sprintf("[微信] %s: %s", msg.FromUserID, text)

	ctx, cancel := context.WithTimeout(ch.ctx, 5*time.Minute)
	defer cancel()

	var response string
	var err error
	if ch.agent != nil {
		slog.Debug("weixin: submitting to agent", "prompt_len", len(prompt))
		response, err = ch.agent.Submit(ctx, prompt)
		slog.Debug("weixin: agent submit returned", "response_len", len(response), "error", err)
	} else {
		response = "agent not available"
	}

	if err != nil {
		slog.Error("weixin: agent submit failed", "error", err)
		response = "处理消息时出错，请稍后重试。"
	}

	// Send response back to WeChat.
	if response != "" {
		slog.Info("weixin: sending reply", "to", msg.FromUserID, "len", len(response))
		if err := ch.client.SendMessage(ctx, msg.FromUserID, response, msg.ContextToken); err != nil {
			slog.Error("weixin: sendMessage failed", "to", msg.FromUserID, "error", err)
		} else {
			slog.Info("weixin: reply sent", "to", msg.FromUserID)
		}
	} else {
		slog.Warn("weixin: empty response, not sending", "from", msg.FromUserID)
	}

	if ch.cfg.OnMessageHandled != nil {
		ch.cfg.OnMessageHandled()
	}
}

// Stop gracefully shuts down the weixin channel.
func (ch *Channel) Stop() error {
	if ch.cancel != nil {
		ch.cancel()
	}
	ch.wg.Wait()

	if ch.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ch.client.NotifyStop(ctx); err != nil {
			slog.Warn("weixin: notifyStop failed", "error", err)
		}
	}

	ch.setConnected(false)
	slog.Info("weixin: channel stopped")
	return nil
}

// Send sends a text message to a WeChat user.
func (ch *Channel) Send(ctx context.Context, to, text string) error {
	if ch.client == nil {
		return fmt.Errorf("weixin: not connected")
	}
	ch.mu.RLock()
	token := ch.lastContextToken
	ch.mu.RUnlock()
	return ch.client.SendMessage(ctx, to, text, token)
}

// Status returns current channel state.
func (ch *Channel) Status() channel.Status {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return channel.Status{
		Connected: ch.connected,
		AccountID: ch.accountID,
		UserID:    ch.userID,
		LastMsgAt: ch.lastMsgAt,
		MsgCount:  ch.msgCount,
	}
}

// LastUserID returns the most recent WeChat user who sent a message.
func (ch *Channel) LastUserID() string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.lastFromUser
}

// SetAgent sets the agent reference for message routing.
func (ch *Channel) SetAgent(a AgentLoop) {
	ch.agent = a
}

// HasStoredAccount checks if a saved account exists (without connecting).
func (ch *Channel) HasStoredAccount() bool {
	ids, err := ch.store.ListAccountIDs()
	if err != nil || len(ids) == 0 {
		return false
	}
	acc, err := ch.store.LoadAccount(ids[0])
	return err == nil && acc != nil && acc.Token != ""
}

// IsConnected returns whether the channel is currently connected.
func (ch *Channel) IsConnected() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.connected
}

// Login triggers a new QR login (even if already connected).
func (ch *Channel) Login() error {
	if ch.connected {
		_ = ch.Stop()
	}

	// Wait for polling to fully stop.
	ch.wg.Wait()

	// Clear old account.
	if ch.accountID != "" {
		_ = ch.store.DeleteAccount(ch.accountID)
		ch.accountID = ""
	}

	// Re-create context for a fresh start.
	ch.ctx, ch.cancel = context.WithCancel(context.Background())

	return ch.startLogin()
}

// Logout disconnects and clears stored credentials.
func (ch *Channel) Logout() error {
	_ = ch.Stop()

	ch.wg.Wait()

	if ch.accountID != "" {
		_ = ch.store.DeleteAccount(ch.accountID)
	}

	ch.mu.Lock()
	ch.accountID = ""
	ch.userID = ""
	ch.connected = false
	ch.lastMsgAt = time.Time{}
	ch.msgCount = 0
	ch.mu.Unlock()

	ch.ctx, ch.cancel = context.WithCancel(context.Background())
	fmt.Println("微信账号已解绑。")
	return nil
}

func (ch *Channel) setConnected(v bool) {
	ch.mu.Lock()
	ch.connected = v
	ch.mu.Unlock()
}

func extractText(items []MessageItem) string {
	var parts []string
	for _, item := range items {
		switch item.Type {
		case ItemTypeText:
			if item.TextItem != nil && item.TextItem.Text != "" {
				parts = append(parts, item.TextItem.Text)
			}
		case ItemTypeVoice:
			if item.VoiceItem != nil && item.VoiceItem.Text != "" {
				parts = append(parts, item.VoiceItem.Text)
			}
		}
	}
	return strings.Join(parts, "")
}
