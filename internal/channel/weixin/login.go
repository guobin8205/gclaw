package weixin

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const (
	qrPollInterval   = 1 * time.Second
	maxQRRefresh     = 3
	loginTimeout     = 5 * time.Minute
)

// LoginResult holds a successful login outcome.
type LoginResult struct {
	AccountID string
	Token     string
	BaseURL   string
	UserID    string
}

// LoginWithQR performs the QR code login flow.
// onQR is called with the QR image URL so the caller can display it.
func LoginWithQR(ctx context.Context, baseURL string, onQR func(qrURL string)) (*LoginResult, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	botType := DefaultBotType
	refreshCount := 0

	// Use a timeout context for the initial QR fetch.
	qrCtx, qrCancel := context.WithTimeout(ctx, 15*time.Second)
	defer qrCancel()

	qrResp, err := GetQRCode(qrCtx, baseURL, botType)
	if err != nil {
		return nil, fmt.Errorf("获取二维码失败: %w", err)
	}

	qrURL := qrResp.QRCodeImgContent
	if qrURL == "" {
		return nil, fmt.Errorf("服务器返回的二维码地址为空")
	}

	onQR(qrURL)

	slog.Info("weixin login: QR code ready, waiting for scan")

	deadline := time.Now().Add(loginTimeout)
	currentBaseURL := baseURL

	for time.Now().Before(deadline) {
		statusCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		statusResp, err := PollQRStatus(statusCtx, currentBaseURL, qrResp.QRCode)
		cancel()

		if err != nil {
			slog.Debug("weixin login: poll error, retrying", "error", err)
			time.Sleep(qrPollInterval)
			continue
		}

		switch statusResp.Status {
		case "confirmed":
			if statusResp.ILinkBotID == "" {
				return nil, fmt.Errorf("登录确认但未返回 bot ID")
			}
			slog.Info("weixin login: confirmed", "bot_id", statusResp.ILinkBotID)
			return &LoginResult{
				AccountID: statusResp.ILinkBotID,
				Token:     statusResp.BotToken,
				BaseURL:   statusResp.BaseURL,
				UserID:    statusResp.ILinkUserID,
			}, nil

		case "expired":
			refreshCount++
			if refreshCount > maxQRRefresh {
				return nil, fmt.Errorf("二维码多次过期，请重新开始登录")
			}
			fmt.Printf("\n二维码已过期，正在刷新 (%d/%d)...\n", refreshCount, maxQRRefresh)
			slog.Info("weixin login: QR expired, refreshing", "attempt", refreshCount)

			qrResp, err = GetQRCode(ctx, baseURL, botType)
			if err != nil {
				return nil, fmt.Errorf("刷新二维码失败: %w", err)
			}
			qrURL = qrResp.QRCodeImgContent
			onQR(qrURL)

		case "scaned_but_redirect":
			if statusResp.RedirectHost != "" {
				currentBaseURL = "https://" + statusResp.RedirectHost
				slog.Info("weixin login: redirect", "host", statusResp.RedirectHost)
			}

		case "scaned":
			// User scanned but hasn't confirmed yet.
			time.Sleep(qrPollInterval)

		case "wait":
			time.Sleep(qrPollInterval)
		}
	}

	return nil, fmt.Errorf("登录超时，请重试")
}
