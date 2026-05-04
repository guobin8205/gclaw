package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// httpTransport communicates with an MCP server via HTTP POST.
type httpTransport struct {
	url     string
	client  *http.Client
	headers map[string]string
	mu      sync.Mutex
}

// newHTTPTransport creates an HTTP transport for the given URL.
func newHTTPTransport(url string) (*httpTransport, error) {
	if url == "" {
		return nil, fmt.Errorf("empty URL")
	}
	return &httpTransport{
		url: url,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		headers: make(map[string]string),
	}, nil
}

// send sends a JSON-RPC request via HTTP POST.
func (t *httpTransport) send(ctx context.Context, request jsonRPCRequest) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d: %s", resp.StatusCode, string(body))
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}

	return rpcResp.Result, nil
}

// sendNotification sends a JSON-RPC notification via HTTP POST (no response expected to be meaningful).
func (t *httpTransport) sendNotification(ctx context.Context, notif jsonRPCRequest) error {
	data, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// close is a no-op for HTTP transport.
func (t *httpTransport) close() error {
	return nil
}
