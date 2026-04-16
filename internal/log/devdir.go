package log

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/yanmxa/gencode/internal/core"
)

// devRequest represents the request data saved to JSON file.
// Domain-typed fields use any so this package avoids domain imports.
type devRequest struct {
	Turn         int       `json:"turn"`
	Timestamp    time.Time `json:"timestamp"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	MaxTokens    int       `json:"max_tokens"`
	Temperature  float64   `json:"temperature"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
	Tools        any       `json:"tools,omitempty"`
	Messages     any       `json:"messages"`
}

// devResponse represents the response data saved to JSON file.
// Domain-typed fields use any so this package avoids domain imports.
type devResponse struct {
	Turn       int       `json:"turn"`
	Timestamp  time.Time `json:"timestamp"`
	Provider   string    `json:"provider"`
	StopReason string    `json:"stop_reason"`
	Content    string    `json:"content,omitempty"`
	Thinking   string    `json:"thinking,omitempty"`
	ToolCalls  any       `json:"tool_calls,omitempty"`
	Usage      any       `json:"usage"`
}

// turnFilePrefix returns the file prefix for a given tracker and turn.
// If tracker is nil, uses the main loop prefix.
func turnFilePrefix(tracker *AgentTurnTracker, turn int) string {
	if tracker != nil {
		return tracker.GetTurnPrefix(turn)
	}
	return getTurnPrefix(turn)
}

// writeDevRequest writes request data to JSON file in DEV_DIR.
// tracker may be nil for main-loop requests.
func writeDevRequest(tracker *AgentTurnTracker, providerName, model string, opts any, turn int) {
	if !devEnabled {
		return
	}

	req := devRequest{
		Turn:      turn,
		Timestamp: time.Now().UTC(),
		Provider:  providerName,
		Model:     model,
	}

	if rl, ok := opts.(requestLoggable); ok {
		req.MaxTokens = rl.LogMaxTokens()
		req.Temperature = rl.LogTemperature()
		req.SystemPrompt = rl.LogSystemPrompt()
	}
	if rd, ok := opts.(requestDevData); ok {
		req.Tools = rd.LogRawTools()
		req.Messages = rd.LogRawMessages()
	}

	writeJSON(filepath.Join(devDir, turnFilePrefix(tracker, turn)+"-request.json"), req)
}

// writeDevResponse writes response data to JSON file in DEV_DIR.
// tracker may be nil for main-loop responses.
func writeDevResponse(tracker *AgentTurnTracker, providerName string, resp any, turn int) {
	if !devEnabled {
		return
	}

	res := devResponse{
		Turn:      turn,
		Timestamp: time.Now().UTC(),
		Provider:  providerName,
	}

	switch r := resp.(type) {
	case core.CompletionResponse:
		res.StopReason = r.StopReason
		res.Content = r.Content
		res.Thinking = r.Thinking
		res.ToolCalls = r.ToolCalls
		res.Usage = r.Usage
	case *core.CompletionResponse:
		if r != nil {
			res.StopReason = r.StopReason
			res.Content = r.Content
			res.Thinking = r.Thinking
			res.ToolCalls = r.ToolCalls
			res.Usage = r.Usage
		}
	}

	writeJSON(filepath.Join(devDir, turnFilePrefix(tracker, turn)+"-response.json"), res)
}

func writeJSON(filename string, data any) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filename, jsonData, 0o600)
}
