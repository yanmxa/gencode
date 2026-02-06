package transport

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

// HTTPConfig contains configuration for HTTP transport
type HTTPConfig struct {
	URL     string
	Headers map[string]string
}

// HTTPTransport implements Transport for HTTP-based MCP servers.
type HTTPTransport struct {
	config  HTTPConfig
	client  *http.Client
	baseURL string

	mu    sync.Mutex
	alive bool
}

// NewHTTPTransport creates a new HTTP transport
func NewHTTPTransport(config HTTPConfig) *HTTPTransport {
	return &HTTPTransport{
		config: config,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		alive: false,
	}
}

// Start initializes the HTTP transport
func (t *HTTPTransport) Start(ctx context.Context) error {
	// Expand environment variables in config
	t.baseURL = ExpandEnv(t.config.URL)
	t.config.Headers = ExpandEnvMap(t.config.Headers)

	// Validate URL
	if t.baseURL == "" {
		return fmt.Errorf("URL is required for HTTP transport")
	}

	t.mu.Lock()
	t.alive = true
	t.mu.Unlock()

	return nil
}

// newJSONRequest creates an HTTP POST request with JSON body and configured headers
func (t *HTTPTransport) newJSONRequest(ctx context.Context, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.config.Headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// Send sends a request and waits for response
func (t *HTTPTransport) Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	if !t.IsAlive() {
		return nil, fmt.Errorf("transport is not connected")
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := t.newJSONRequest(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	var jsonResp JSONRPCResponse
	if err := json.Unmarshal(body, &jsonResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &jsonResp, nil
}

// SendNotification sends a notification (no response expected)
func (t *HTTPTransport) SendNotification(ctx context.Context, notif *JSONRPCNotification) error {
	if !t.IsAlive() {
		return fmt.Errorf("transport is not connected")
	}

	data, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	httpReq, err := t.newJSONRequest(ctx, data)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return nil
}

// Close closes the transport
func (t *HTTPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.alive = false
	return nil
}

// IsAlive returns true if the transport is connected
func (t *HTTPTransport) IsAlive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.alive
}

// SetNotificationHandler is a no-op for HTTP transport (stateless, no server-push)
func (t *HTTPTransport) SetNotificationHandler(_ NotificationHandler) {
	// HTTP is stateless - notifications are not supported
}

