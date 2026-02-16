package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
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

	mu            sync.Mutex
	alive         bool
	notifyHandler NotificationHandler
	sessionID     string
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
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range t.config.Headers {
		req.Header.Set(k, v)
	}
	// Include session ID for MCP Streamable HTTP session tracking
	t.mu.Lock()
	if t.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", t.sessionID)
	}
	t.mu.Unlock()
	return req, nil
}

// doRequest sends an HTTP request with retry on 429 (rate limit).
// The buildReq function is called for each attempt to create a fresh request.
func (t *HTTPTransport) doRequest(ctx context.Context, buildReq func() (*http.Request, error)) (*http.Response, error) {
	const maxRetries = 5
	backoff := 2 * time.Second

	for attempt := range maxRetries {
		req, err := buildReq()
		if err != nil {
			return nil, err
		}

		resp, err := t.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("HTTP request failed: %w", err)
		}

		if resp.StatusCode != http.StatusTooManyRequests || attempt == maxRetries-1 {
			// Capture session ID from MCP Streamable HTTP
			if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
				t.mu.Lock()
				t.sessionID = sid
				t.mu.Unlock()
			}
			return resp, nil
		}

		// Use Retry-After header if present, otherwise exponential backoff
		wait := backoff
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				wait = time.Duration(secs) * time.Second
			}
		}

		resp.Body.Close()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		backoff *= 2
	}

	// unreachable
	return nil, fmt.Errorf("exhausted retries")
}

// Send sends a request and waits for response.
// Supports MCP Streamable HTTP: accepts both JSON and SSE responses.
func (t *HTTPTransport) Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	if !t.IsAlive() {
		return nil, fmt.Errorf("transport is not connected")
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := t.doRequest(ctx, func() (*http.Request, error) {
		return t.newJSONRequest(ctx, data)
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	// Check Content-Type to determine response format
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		return t.parseSSEResponse(resp.Body, req.ID)
	}

	// Standard JSON response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var jsonResp JSONRPCResponse
	if err := json.Unmarshal(body, &jsonResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &jsonResp, nil
}

// parseSSEResponse reads an SSE stream and returns the JSON-RPC response matching the request ID.
func (t *HTTPTransport) parseSSEResponse(r io.Reader, requestID uint64) (*JSONRPCResponse, error) {
	scanner := bufio.NewScanner(r)
	var data string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// Empty line = end of event, process accumulated data
			if data != "" {
				var resp JSONRPCResponse
				if err := json.Unmarshal([]byte(data), &resp); err == nil {
					if resp.ID == requestID {
						return &resp, nil
					}
				}
				// Dispatch notifications
				t.mu.Lock()
				handler := t.notifyHandler
				t.mu.Unlock()
				if handler != nil {
					ParseAndDispatchNotification([]byte(data), handler)
				}
			}
			data = ""
			continue
		}

		if after, found := strings.CutPrefix(line, "data:"); found {
			data = strings.TrimSpace(after)
		}
	}

	// Try to parse any remaining data
	if data != "" {
		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(data), &resp); err == nil {
			return &resp, nil
		}
	}

	return nil, fmt.Errorf("SSE stream ended without response for request %d", requestID)
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

	resp, err := t.doRequest(ctx, func() (*http.Request, error) {
		return t.newJSONRequest(ctx, data)
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}
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

// SetNotificationHandler stores a handler for notifications received in SSE streams.
func (t *HTTPTransport) SetNotificationHandler(handler NotificationHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.notifyHandler = handler
}

