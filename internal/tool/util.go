package tool

import (
	"crypto/rand"
	"encoding/hex"
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
