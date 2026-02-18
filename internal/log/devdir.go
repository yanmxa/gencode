package log

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
)

// DevRequest represents the request data saved to JSON file
type DevRequest struct {
	Turn         int                `json:"turn"`
	Timestamp    time.Time          `json:"timestamp"`
	Provider     string             `json:"provider"`
	Model        string             `json:"model"`
	MaxTokens    int                `json:"max_tokens"`
	Temperature  float64            `json:"temperature"`
	SystemPrompt string             `json:"system_prompt,omitempty"`
	Tools        []provider.Tool    `json:"tools,omitempty"`
	Messages     []message.Message  `json:"messages"`
}

// DevResponse represents the response data saved to JSON file
type DevResponse struct {
	Turn       int                `json:"turn"`
	Timestamp  time.Time          `json:"timestamp"`
	Provider   string             `json:"provider"`
	StopReason string             `json:"stop_reason"`
	Content    string             `json:"content,omitempty"`
	Thinking   string             `json:"thinking,omitempty"`
	ToolCalls  []message.ToolCall `json:"tool_calls,omitempty"`
	Usage      message.Usage      `json:"usage"`
}

// turnPrefix returns the file prefix for a given tracker and turn.
// If tracker is nil, uses the main loop prefix.
func turnPrefix(tracker *AgentTurnTracker, turn int) string {
	if tracker != nil {
		return tracker.GetTurnPrefix(turn)
	}
	return GetTurnPrefix(turn)
}

// writeDevRequest writes request data to JSON file in DEV_DIR.
// tracker may be nil for main-loop requests.
func writeDevRequest(tracker *AgentTurnTracker, providerName, model string, opts provider.CompletionOptions, turn int) {
	if !devEnabled {
		return
	}
	req := DevRequest{
		Turn:         turn,
		Timestamp:    time.Now().UTC(),
		Provider:     providerName,
		Model:        model,
		MaxTokens:    opts.MaxTokens,
		Temperature:  opts.Temperature,
		SystemPrompt: opts.SystemPrompt,
		Tools:        opts.Tools,
		Messages:     opts.Messages,
	}
	writeJSON(filepath.Join(devDir, turnPrefix(tracker, turn)+"-request.json"), req)
}

// writeDevResponse writes response data to JSON file in DEV_DIR.
// tracker may be nil for main-loop responses.
func writeDevResponse(tracker *AgentTurnTracker, providerName string, resp message.CompletionResponse, turn int) {
	if !devEnabled {
		return
	}
	res := DevResponse{
		Turn:       turn,
		Timestamp:  time.Now().UTC(),
		Provider:   providerName,
		StopReason: resp.StopReason,
		Content:    resp.Content,
		Thinking:   resp.Thinking,
		ToolCalls:  resp.ToolCalls,
		Usage:      resp.Usage,
	}
	writeJSON(filepath.Join(devDir, turnPrefix(tracker, turn)+"-response.json"), res)
}

func writeJSON(filename string, data any) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filename, jsonData, 0644)
}
