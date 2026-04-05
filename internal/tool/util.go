package tool

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// ToolError represents a tool-specific validation or execution error.
// It is used by tools to signal structured failures during PreparePermission.
type ToolError struct {
	Message string
}

func (e *ToolError) Error() string {
	return e.Message
}

// generateRequestID generates a unique request ID using cryptographic randomness.
// This avoids collisions that could occur with time-based IDs in high-speed scenarios.
func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based if crypto/rand fails (unlikely)
		return "req_" + strconv.FormatInt(time.Now().UnixNano()%1000000, 10)
	}
	return "req_" + hex.EncodeToString(b)
}

// requireString extracts a required string parameter from params.
// Returns a ToolError if the key is absent or the value is empty.
func requireString(params map[string]any, key string) (string, error) {
	v, ok := params[key].(string)
	if !ok || v == "" {
		return "", &ToolError{Message: fmt.Sprintf("%s is required", key)}
	}
	return v, nil
}

// getString extracts an optional string parameter. Returns "" if absent.
func getString(params map[string]any, key string) string {
	v, _ := params[key].(string)
	return v
}

// getBool extracts an optional bool parameter. Returns false if absent.
func getBool(params map[string]any, key string) bool {
	v, _ := params[key].(bool)
	return v
}

// getFloat64 extracts a numeric parameter that may arrive as float64 or int
// (JSON numbers unmarshal to float64; some callers pass int directly).
// Returns defaultVal if the key is absent or zero.
func getFloat64(params map[string]any, key string, defaultVal float64) float64 {
	switch v := params[key].(type) {
	case float64:
		if v > 0 {
			return v
		}
	case int:
		if v > 0 {
			return float64(v)
		}
	}
	return defaultVal
}

// getInt is like getFloat64 but returns an int.
func getInt(params map[string]any, key string, defaultVal int) int {
	switch v := params[key].(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	}
	return defaultVal
}
