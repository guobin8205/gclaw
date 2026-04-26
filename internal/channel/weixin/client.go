package weixin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	DefaultBaseURL    = "https://ilinkai.weixin.qq.com"
	DefaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
	DefaultBotType    = "3"
	DefaultLongPollMs = 35_000
	DefaultTimeout    = 60 * time.Second

	// iLink headers
	ilinkAppID           = "bot"
	channelVersion       = "1.0.0"
	ilinkAppClientVer    = 0x00010000 // 1.0.0 encoded
)

// Client communicates with the ilink WeChat bot API.
type Client struct {
	baseURL string
	cdnURL  string
	token   string
	http    *http.Client
}

// NewClient creates a new ilink API client.
func NewClient(baseURL, cdnURL, token string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if cdnURL == "" {
		cdnURL = DefaultCDNBaseURL
	}
	return &Client{
		baseURL: baseURL,
		cdnURL:  cdnURL,
		token:   token,
		http: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// SetToken updates the auth token.
func (c *Client) SetToken(token string) {
	c.token = token
}

// buildHeaders returns common headers for all API requests.
func (c *Client) buildHeaders(bodyLen int) map[string]string {
	h := map[string]string{
		"Content-Type":            "application/json",
		"iLink-App-Id":            ilinkAppID,
		"iLink-App-ClientVersion": fmt.Sprintf("%d", ilinkAppClientVer),
		"AuthorizationType":       "ilink_bot_token",
		"X-WECHAT-UIN":            randomWeChatUIN(),
	}
	if c.token != "" {
		h["Authorization"] = "Bearer " + c.token
	}
	return h
}

// postJSON sends a POST request with JSON body and returns the raw response.
func (c *Client) postJSON(ctx context.Context, endpoint string, body any) ([]byte, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	for k, v := range c.buildHeaders(len(bodyBytes)) {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, string(data))
	}

	return data, nil
}

// GetUpdates long-polls for new messages. Returns empty response on timeout.
func (c *Client) GetUpdates(ctx context.Context, buf string, timeoutMs int) (*GetUpdatesResp, error) {
	if timeoutMs <= 0 {
		timeoutMs = DefaultLongPollMs
	}

	pollCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	slog.Debug("weixin: getUpdates request", "buf_len", len(buf))
	data, err := c.postJSON(pollCtx, "/ilink/bot/getupdates", map[string]any{
		"get_updates_buf": buf,
		"base_info":       map[string]string{"channel_version": channelVersion},
	})

	if err != nil {
		// Timeout is normal for long-poll; return empty.
		if pollCtx.Err() != nil {
			slog.Debug("weixin: getUpdates timeout, returning empty")
			return &GetUpdatesResp{GetUpdatesBuf: buf}, nil
		}
		return nil, fmt.Errorf("getUpdates: %w", err)
	}

	if len(data) == 0 {
		slog.Debug("weixin: getUpdates empty response")
		return &GetUpdatesResp{GetUpdatesBuf: buf}, nil
	}

	var result GetUpdatesResp
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal getUpdates: %w", err)
	}

	if result.ErrCode != 0 {
		return nil, fmt.Errorf("getUpdates error code=%d msg=%s", result.ErrCode, result.ErrMsg)
	}

	return &result, nil
}

// SendMessage sends a text message to a user.
func (c *Client) SendMessage(ctx context.Context, toUserID, text, contextToken string) error {
	_, err := c.postJSON(ctx, "/ilink/bot/sendmessage", map[string]any{
		"msg": map[string]any{
			"from_user_id":  "",
			"to_user_id":    toUserID,
			"client_id":     generateClientID(),
			"message_type":  MsgTypeBot,
			"message_state": StateFinish,
			"item_list": []map[string]any{
				{
					"type":      ItemTypeText,
					"text_item": map[string]string{"text": text},
				},
			},
			"context_token": contextToken,
		},
		"base_info": map[string]string{"channel_version": channelVersion},
	})
	return err
}

// generateClientID creates a random client message ID.
func generateClientID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NotifyStart tells the server the channel is starting.
func (c *Client) NotifyStart(ctx context.Context) error {
	_, err := c.postJSON(ctx, "/ilink/bot/msg/notifystart", map[string]any{
		"base_info": map[string]string{"channel_version": channelVersion},
	})
	return err
}

// NotifyStop tells the server the channel is stopping.
func (c *Client) NotifyStop(ctx context.Context) error {
	_, err := c.postJSON(ctx, "/ilink/bot/msg/notifystop", map[string]any{
		"base_info": map[string]string{"channel_version": channelVersion},
	})
	return err
}

// GetQRCode fetches a login QR code.
func GetQRCode(ctx context.Context, baseURL, botType string) (*QRCodeResponse, error) {
	if botType == "" {
		botType = DefaultBotType
	}
	url := fmt.Sprintf("%s/ilink/bot/get_bot_qrcode?bot_type=%s", baseURL, botType)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("getQRCode: %w", err)
	}
	req.Header.Set("iLink-App-Id", ilinkAppID)
	req.Header.Set("iLink-App-ClientVersion", fmt.Sprintf("%d", ilinkAppClientVer))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getQRCode: %w", err)
	}
	defer resp.Body.Close()

	var result QRCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode QRCode: %w", err)
	}
	return &result, nil
}

// PollQRStatus checks the QR code login status.
func PollQRStatus(ctx context.Context, baseURL, qrcode string) (*QRStatusResponse, error) {
	url := fmt.Sprintf("%s/ilink/bot/get_qrcode_status?qrcode=%s", baseURL, qrcode)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("pollQR: %w", err)
	}
	req.Header.Set("iLink-App-Id", ilinkAppID)
	req.Header.Set("iLink-App-ClientVersion", fmt.Sprintf("%d", ilinkAppClientVer))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pollQR: %w", err)
	}
	defer resp.Body.Close()

	var result QRStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode QRStatus: %w", err)
	}
	return &result, nil
}

// randomWeChatUIN generates a random base64-encoded uint32 for X-WECHAT-UIN.
func randomWeChatUIN() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "AAAA"
	}
	n := binary.BigEndian.Uint32(b)
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", n)))
}
