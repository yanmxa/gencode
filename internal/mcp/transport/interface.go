// Package transport provides transport implementations for MCP protocol.
package transport

import (
	"context"
	"encoding/json"
)

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      uint64      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// JSONRPCNotification represents a JSON-RPC 2.0 notification (no ID)
type JSONRPCNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Transport defines the interface for MCP transport implementations.
// All transports must be able to send JSON-RPC requests and receive responses.
type Transport interface {
	// Start initializes and starts the transport connection.
	Start(ctx context.Context) error

	// Send sends a JSON-RPC request and waits for a response.
	// The context can be used to cancel the request.
	Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error)

	// SendNotification sends a JSON-RPC notification (no response expected).
	SendNotification(ctx context.Context, notif *JSONRPCNotification) error

	// Close closes the transport connection and releases resources.
	Close() error

	// IsAlive returns true if the transport connection is still active.
	IsAlive() bool

	// SetNotificationHandler sets a handler for incoming notifications from the server.
	SetNotificationHandler(handler NotificationHandler)
}

// NotificationHandler handles incoming notifications from the MCP server
type NotificationHandler func(method string, params []byte)

// ParseAndDispatchNotification parses a JSON message and dispatches it to the handler if it's a notification.
// Returns true if it was a notification, false otherwise.
func ParseAndDispatchNotification(data []byte, handler NotificationHandler) bool {
	if handler == nil {
		return false
	}

	var notif struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(data, &notif); err != nil {
		return false
	}

	if notif.Method == "" {
		return false
	}

	handler(notif.Method, notif.Params)
	return true
}
