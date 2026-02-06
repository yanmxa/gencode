package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SSEConfig contains configuration for SSE transport
type SSEConfig struct {
	URL     string
	Headers map[string]string
}

// SSETransport implements Transport for SSE-based MCP servers.
// This implements the deprecated SSE transport for backward compatibility.
type SSETransport struct {
	config  SSEConfig
	client  *http.Client
	baseURL string

	mu            sync.Mutex
	pending       map[uint64]chan *JSONRPCResponse
	alive         bool
	notifyHandler NotificationHandler
	cancel        context.CancelFunc
	readLoopDone  chan struct{}
}

// NewSSETransport creates a new SSE transport
func NewSSETransport(config SSEConfig) *SSETransport {
	return &SSETransport{
		config: config,
		client: &http.Client{
			Timeout: 0, // No timeout for SSE
		},
		pending:      make(map[uint64]chan *JSONRPCResponse),
		readLoopDone: make(chan struct{}),
	}
}

// Start initializes the SSE transport
func (t *SSETransport) Start(ctx context.Context) error {
	// Expand environment variables in config
	t.baseURL = ExpandEnv(t.config.URL)
	t.config.Headers = ExpandEnvMap(t.config.Headers)

	// Validate URL
	if t.baseURL == "" {
		return fmt.Errorf("URL is required for SSE transport")
	}

	// Create SSE connection
	ctx, cancel := context.WithCancel(ctx)
	t.cancel = cancel

	sseURL := appendURLPath(t.baseURL, "sse")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sseURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create SSE request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	for k, v := range t.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to SSE endpoint: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("SSE connection failed with status %d", resp.StatusCode)
	}

	t.mu.Lock()
	t.alive = true
	t.mu.Unlock()

	// Start SSE read loop
	go t.readLoop(resp.Body)

	return nil
}

// readLoop reads SSE events from the connection
func (t *SSETransport) readLoop(r io.ReadCloser) {
	defer close(t.readLoopDone)
	defer r.Close()

	reader := bufio.NewReader(r)
	var event, data string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			// Empty line = end of event
			if data != "" {
				t.handleSSEEvent(event, data)
			}
			event = ""
			data = ""
			continue
		}

		if after, found := strings.CutPrefix(line, "event:"); found {
			event = strings.TrimSpace(after)
		} else if after, found := strings.CutPrefix(line, "data:"); found {
			data = strings.TrimSpace(after)
		}
	}

	t.mu.Lock()
	t.alive = false
	for id, ch := range t.pending {
		close(ch)
		delete(t.pending, id)
	}
	t.mu.Unlock()
}

// handleSSEEvent processes an SSE event
func (t *SSETransport) handleSSEEvent(_, data string) {
	if data == "" {
		return
	}

	// Try to parse as JSON-RPC response
	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		// Try as notification
		ParseAndDispatchNotification([]byte(data), t.notifyHandler)
		return
	}

	// Check if this is a response (has ID) or notification
	if resp.ID == 0 && resp.Result == nil && resp.Error == nil {
		ParseAndDispatchNotification([]byte(data), t.notifyHandler)
		return
	}

	// Find pending request
	t.mu.Lock()
	ch, ok := t.pending[resp.ID]
	if ok {
		delete(t.pending, resp.ID)
	}
	t.mu.Unlock()

	if ok {
		ch <- &resp
	}
}

// postMessage sends a JSON message to the /message endpoint
func (t *SSETransport) postMessage(ctx context.Context, data []byte) error {
	msgURL := appendURLPath(t.baseURL, "message")

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, msgURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.config.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return nil
}

// Send sends a request and waits for response
func (t *SSETransport) Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	if !t.IsAlive() {
		return nil, fmt.Errorf("transport is not connected")
	}

	respCh := make(chan *JSONRPCResponse, 1)

	t.mu.Lock()
	t.pending[req.ID] = respCh
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		delete(t.pending, req.ID)
		t.mu.Unlock()
	}()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if err := t.postMessage(ctx, data); err != nil {
		return nil, err
	}

	// Wait for response via SSE
	timeout := 60 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	select {
	case result := <-respCh:
		if result == nil {
			return nil, fmt.Errorf("connection closed")
		}
		return result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("request timeout")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SendNotification sends a notification (no response expected)
func (t *SSETransport) SendNotification(ctx context.Context, notif *JSONRPCNotification) error {
	if !t.IsAlive() {
		return fmt.Errorf("transport is not connected")
	}

	data, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	return t.postMessage(ctx, data)
}

// Close closes the transport
func (t *SSETransport) Close() error {
	t.mu.Lock()
	t.alive = false
	t.mu.Unlock()

	if t.cancel != nil {
		t.cancel()
	}

	select {
	case <-t.readLoopDone:
	case <-time.After(2 * time.Second):
	}

	return nil
}

// IsAlive returns true if the transport is connected
func (t *SSETransport) IsAlive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.alive
}

// SetNotificationHandler sets the handler for incoming notifications
func (t *SSETransport) SetNotificationHandler(handler NotificationHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.notifyHandler = handler
}

// appendURLPath appends a path segment to a base URL, handling trailing slashes.
func appendURLPath(base, segment string) string {
	if strings.HasSuffix(base, "/"+segment) {
		return base
	}
	if strings.HasSuffix(base, "/") {
		return base + segment
	}
	return base + "/" + segment
}

