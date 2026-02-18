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
	Messages     []message.Message `json:"messages"`
}

// DevResponse represents the response data saved to JSON file
type DevResponse struct {
	Turn       int                 `json:"turn"`
	Timestamp  time.Time           `json:"timestamp"`
	Provider   string              `json:"provider"`
	StopReason string              `json:"stop_reason"`
	Content    string              `json:"content,omitempty"`
	Thinking   string              `json:"thinking,omitempty"`
	ToolCalls  []message.ToolCall `json:"tool_calls,omitempty"`
	Usage      message.Usage      `json:"usage"`
}

// WriteDevRequest writes request data to JSON file in DEV_DIR
func WriteDevRequest(providerName, model string, opts provider.CompletionOptions, turn int) {
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
	filename := filepath.Join(devDir, GetTurnPrefix(turn)+"-request.json")
	writeJSON(filename, req)
}

// WriteDevResponse writes response data to JSON file in DEV_DIR
func WriteDevResponse(providerName string, resp message.CompletionResponse, turn int) {
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
	filename := filepath.Join(devDir, GetTurnPrefix(turn)+"-response.json")
	writeJSON(filename, res)
}

// WriteAgentDevRequest writes agent request data with hierarchical naming
func WriteAgentDevRequest(tracker *AgentTurnTracker, providerName, model string, opts provider.CompletionOptions, turn int) {
	if !devEnabled || tracker == nil {
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
	filename := filepath.Join(devDir, tracker.GetTurnPrefix(turn)+"-request.json")
	writeJSON(filename, req)
}

// WriteAgentDevResponse writes agent response data with hierarchical naming
func WriteAgentDevResponse(tracker *AgentTurnTracker, providerName string, resp message.CompletionResponse, turn int) {
	if !devEnabled || tracker == nil {
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
	filename := filepath.Join(devDir, tracker.GetTurnPrefix(turn)+"-response.json")
	writeJSON(filename, res)
}

func writeJSON(filename string, data any) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filename, jsonData, 0644)
}
