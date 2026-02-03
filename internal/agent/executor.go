package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/ui"
	"go.uber.org/zap"
)

// Executor runs agent LLM loops
type Executor struct {
	provider      provider.LLMProvider
	cwd           string
	parentModelID string // Parent conversation's model ID (used when inheriting)
}

// NewExecutor creates a new agent executor
// parentModelID is the model used by the parent conversation (for inheritance)
func NewExecutor(llmProvider provider.LLMProvider, cwd string, parentModelID string) *Executor {
	return &Executor{
		provider:      llmProvider,
		cwd:           cwd,
		parentModelID: parentModelID,
	}
}

// GetParentModelID returns the parent model ID
func (e *Executor) GetParentModelID() string {
	return e.parentModelID
}

// Run executes an agent request and returns the result
// For background agents, this should be called in a goroutine
func (e *Executor) Run(ctx context.Context, req AgentRequest) (*AgentResult, error) {
	start := time.Now()

	// Get agent configuration
	config, ok := DefaultRegistry.Get(req.Agent)
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", req.Agent)
	}

	// Determine model to use (priority: request > parent > fallback)
	modelID := e.resolveModelID(req.Model)

	// Determine max turns
	maxTurns := config.MaxTurns
	if req.MaxTurns > 0 {
		maxTurns = req.MaxTurns
	}
	if maxTurns <= 0 {
		maxTurns = DefaultMaxTurns
	}

	// Build system prompt
	systemPrompt := e.buildSystemPrompt(config, req)

	// Get filtered tools
	tools := e.filterTools(config)

	// Initialize messages
	messages := []provider.Message{
		{Role: "user", Content: req.Prompt},
	}

	// Track usage
	var totalInputTokens, totalOutputTokens int
	var turnCount int
	var lastContent string

	log.Logger().Info("Starting agent execution",
		zap.String("agent", config.Name),
		zap.String("description", req.Description),
		zap.Int("maxTurns", maxTurns),
	)

	// Main agent loop
	for turnCount < maxTurns {
		select {
		case <-ctx.Done():
			return &AgentResult{
				AgentName:  config.Name,
				Success:    false,
				Content:    lastContent,
				Messages:   messages,
				TurnCount:  turnCount,
				TokenUsage: TokenUsage{InputTokens: totalInputTokens, OutputTokens: totalOutputTokens, TotalTokens: totalInputTokens + totalOutputTokens},
				Duration:   time.Since(start),
				Error:      "agent cancelled",
			}, ctx.Err()
		default:
		}

		turnCount++

		log.Logger().Debug("Agent turn",
			zap.Int("turn", turnCount),
			zap.Int("maxTurns", maxTurns),
		)

		// Stream response from LLM
		response, err := e.streamCompletion(ctx, modelID, systemPrompt, messages, tools)
		if err != nil {
			return nil, fmt.Errorf("LLM completion failed: %w", err)
		}

		// Update token usage
		totalInputTokens += response.Usage.InputTokens
		totalOutputTokens += response.Usage.OutputTokens
		lastContent = response.Content

		// Add assistant message to history
		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})

		// Check if done
		if response.StopReason == "end_turn" || len(response.ToolCalls) == 0 {
			log.Logger().Info("Agent completed",
				zap.Int("turns", turnCount),
				zap.Int("inputTokens", totalInputTokens),
				zap.Int("outputTokens", totalOutputTokens),
			)

			return &AgentResult{
				AgentName:  config.Name,
				Success:    true,
				Content:    response.Content,
				Messages:   messages,
				TurnCount:  turnCount,
				TokenUsage: TokenUsage{InputTokens: totalInputTokens, OutputTokens: totalOutputTokens, TotalTokens: totalInputTokens + totalOutputTokens},
				Duration:   time.Since(start),
			}, nil
		}

		// Execute tool calls
		for _, tc := range response.ToolCalls {
			select {
			case <-ctx.Done():
				return &AgentResult{
					AgentName:  config.Name,
					Success:    false,
					Content:    lastContent,
					Messages:   messages,
					TurnCount:  turnCount,
					TokenUsage: TokenUsage{InputTokens: totalInputTokens, OutputTokens: totalOutputTokens, TotalTokens: totalInputTokens + totalOutputTokens},
					Duration:   time.Since(start),
					Error:      "agent cancelled during tool execution",
				}, ctx.Err()
			default:
			}

			result := e.executeTool(ctx, tc, config, req.OnProgress)

			// Add tool result to messages
			messages = append(messages, provider.Message{
				Role: "user",
				ToolResult: &provider.ToolResult{
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
					Content:    result.FormatForLLM(),
					IsError:    !result.Success,
				},
			})
		}
	}

	// Max turns reached
	log.Logger().Warn("Agent reached max turns",
		zap.Int("maxTurns", maxTurns),
	)

	return &AgentResult{
		AgentName:  config.Name,
		Success:    false,
		Content:    lastContent,
		Messages:   messages,
		TurnCount:  turnCount,
		TokenUsage: TokenUsage{InputTokens: totalInputTokens, OutputTokens: totalOutputTokens, TotalTokens: totalInputTokens + totalOutputTokens},
		Duration:   time.Since(start),
		Error:      fmt.Sprintf("reached maximum turns (%d)", maxTurns),
	}, nil
}

// RunBackground executes an agent in the background and returns the task
func (e *Executor) RunBackground(req AgentRequest) (*task.AgentTask, error) {
	// Get agent configuration for validation
	config, ok := DefaultRegistry.Get(req.Agent)
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", req.Agent)
	}

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Create agent task
	agentTask := task.NewAgentTask(
		task.GenerateID(),
		config.Name,
		req.Description,
		ctx,
		cancel,
	)

	// Register with task manager
	task.DefaultManager.RegisterTask(agentTask)

	// Set up progress callback to forward to task
	req.OnProgress = func(msg string) {
		agentTask.AppendProgress(msg)
	}

	// Start goroutine to run agent
	go func() {
		defer cancel()

		result, err := e.Run(ctx, req)

		if err != nil {
			agentTask.AppendOutput([]byte(fmt.Sprintf("Error: %v\n", err)))
			agentTask.Complete(err)
			return
		}

		// Append result content to output
		if result.Content != "" {
			agentTask.AppendOutput([]byte(result.Content))
		}

		// Update progress
		agentTask.UpdateProgress(result.TurnCount, result.TokenUsage.TotalTokens)

		if result.Success {
			agentTask.Complete(nil)
		} else {
			agentTask.Complete(fmt.Errorf("%s", result.Error))
		}
	}()

	return agentTask, nil
}

// resolveModelID determines the model to use based on priority:
// 1. Explicit request override (highest priority)
// 2. Parent conversation model (inherited)
// 3. Fallback default (lowest priority)
func (e *Executor) resolveModelID(requestModel string) string {
	if requestModel != "" {
		return requestModel
	}
	if e.parentModelID != "" {
		return e.parentModelID
	}
	return FallbackModel
}

// streamCompletion streams a completion and collects the full response
func (e *Executor) streamCompletion(ctx context.Context, modelID string, systemPrompt string, messages []provider.Message, tools []provider.Tool) (*provider.CompletionResponse, error) {
	streamChan := e.provider.Stream(ctx, provider.CompletionOptions{
		Model:        modelID,
		Messages:     messages,
		MaxTokens:    8192,
		Tools:        tools,
		SystemPrompt: systemPrompt,
	})

	var response provider.CompletionResponse

	for chunk := range streamChan {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		switch chunk.Type {
		case provider.ChunkTypeText:
			response.Content += chunk.Text
		case provider.ChunkTypeToolStart:
			// Tool call started
			response.ToolCalls = append(response.ToolCalls, provider.ToolCall{
				ID:   chunk.ToolID,
				Name: chunk.ToolName,
			})
		case provider.ChunkTypeToolInput:
			// Add input to last tool call
			if len(response.ToolCalls) > 0 {
				idx := len(response.ToolCalls) - 1
				response.ToolCalls[idx].Input += chunk.Text
			}
		case provider.ChunkTypeDone:
			if chunk.Response != nil {
				return chunk.Response, nil
			}
			return &response, nil
		case provider.ChunkTypeError:
			return nil, chunk.Error
		}
	}

	return &response, nil
}

// executeTool executes a single tool call
func (e *Executor) executeTool(ctx context.Context, tc provider.ToolCall, config *AgentConfig, onProgress ProgressCallback) ui.ToolResult {
	// Parse input
	var params map[string]any
	if err := json.Unmarshal([]byte(tc.Input), &params); err != nil {
		return ui.NewErrorResult(tc.Name, fmt.Sprintf("Error parsing tool input: %v", err))
	}

	// Report progress before execution
	if onProgress != nil {
		progressMsg := e.formatToolProgress(tc.Name, params)
		onProgress(progressMsg)
	}

	// Get tool
	t, ok := tool.Get(tc.Name)
	if !ok {
		return ui.NewErrorResult(tc.Name, fmt.Sprintf("Unknown tool: %s", tc.Name))
	}

	// Check if tool requires permission
	// In agent context, we handle permission based on agent's permission mode
	if pat, ok := t.(tool.PermissionAwareTool); ok && pat.RequiresPermission() {
		switch config.PermissionMode {
		case PermissionPlan:
			// Plan mode - only allow read-only tools
			if !e.isReadOnlyTool(tc.Name) {
				return ui.NewErrorResult(tc.Name, fmt.Sprintf("Tool %s requires write permission but agent is in plan mode", tc.Name))
			}
		case PermissionDontAsk:
			// Auto-accept all
			return pat.ExecuteApproved(ctx, params, e.cwd)
		case PermissionAcceptEdits:
			// Accept edits but check bash commands
			if tc.Name == "Bash" {
				// For simplicity, allow bash in acceptEdits mode
				// In a full implementation, this would prompt the user
			}
			return pat.ExecuteApproved(ctx, params, e.cwd)
		default:
			// Default - execute with approval (for agents we auto-approve in this simplified version)
			return pat.ExecuteApproved(ctx, params, e.cwd)
		}
	}

	// Execute read-only tool
	return t.Execute(ctx, params, e.cwd)
}

// formatToolProgress creates a progress message for a tool call
func (e *Executor) formatToolProgress(toolName string, params map[string]any) string {
	switch toolName {
	case "Read":
		if path, ok := params["file_path"].(string); ok {
			return fmt.Sprintf("Reading: %s", path)
		}
	case "Glob":
		if pattern, ok := params["pattern"].(string); ok {
			return fmt.Sprintf("Finding: %s", pattern)
		}
	case "Grep":
		if pattern, ok := params["pattern"].(string); ok {
			return fmt.Sprintf("Searching: %s", pattern)
		}
	case "WebFetch":
		if url, ok := params["url"].(string); ok {
			return fmt.Sprintf("Fetching: %s", url)
		}
	case "WebSearch":
		if query, ok := params["query"].(string); ok {
			return fmt.Sprintf("Searching web: %s", query)
		}
	case "Bash":
		if cmd, ok := params["command"].(string); ok {
			if len(cmd) > 50 {
				cmd = cmd[:47] + "..."
			}
			return fmt.Sprintf("Running: %s", cmd)
		}
	}
	return fmt.Sprintf("Executing: %s", toolName)
}

// isReadOnlyTool checks if a tool is read-only
func (e *Executor) isReadOnlyTool(name string) bool {
	readOnlyTools := map[string]bool{
		"Read":      true,
		"Glob":      true,
		"Grep":      true,
		"WebFetch":  true,
		"WebSearch": true,
	}
	return readOnlyTools[name]
}

// buildSystemPrompt builds the system prompt for the agent
func (e *Executor) buildSystemPrompt(config *AgentConfig, req AgentRequest) string {
	var sb strings.Builder

	// Base agent identity
	sb.WriteString("You are a specialized AI agent within GenCode, an AI coding assistant.\n\n")

	// Agent-specific prompt
	sb.WriteString(fmt.Sprintf("## Agent Type: %s\n", config.Name))
	sb.WriteString(config.Description)
	sb.WriteString("\n\n")

	// Task context
	sb.WriteString("## Your Task\n")
	sb.WriteString(req.Prompt)
	sb.WriteString("\n\n")

	// Mode-specific instructions
	switch config.PermissionMode {
	case PermissionPlan:
		sb.WriteString("## Mode: Read-Only\n")
		sb.WriteString("You are in read-only mode. You can only use tools that read information (Read, Glob, Grep, WebFetch, WebSearch). Do not attempt to modify any files.\n\n")
	case PermissionDontAsk:
		sb.WriteString("## Mode: Autonomous\n")
		sb.WriteString("You have full autonomy to complete your task. You can read and modify files, execute commands, and make changes as needed.\n\n")
	}

	// Custom system prompt from config
	if config.SystemPrompt != "" {
		sb.WriteString("## Additional Instructions\n")
		sb.WriteString(config.SystemPrompt)
		sb.WriteString("\n\n")
	}

	// Environment info
	sb.WriteString("## Environment\n")
	sb.WriteString(fmt.Sprintf("- Working directory: %s\n", e.cwd))
	sb.WriteString(fmt.Sprintf("- Platform: %s\n", runtime.GOOS))
	sb.WriteString(fmt.Sprintf("- Date: %s\n", time.Now().Format("2006-01-02")))
	sb.WriteString("\n")

	// Guidelines
	sb.WriteString("## Guidelines\n")
	sb.WriteString("- Focus on completing your assigned task efficiently\n")
	sb.WriteString("- Return a clear summary when your task is complete\n")
	sb.WriteString("- If you encounter errors, report them clearly\n")

	return sb.String()
}

// filterTools returns the tools available to the agent based on configuration
func (e *Executor) filterTools(config *AgentConfig) []provider.Tool {
	allTools := tool.GetToolSchemas()

	// Always filter out Task to prevent nested agent spawning
	filtered := make([]provider.Tool, 0, len(allTools))

	for _, t := range allTools {
		// Never allow Task tool in agents
		if t.Name == "Task" {
			continue
		}

		// Never allow EnterPlanMode or ExitPlanMode in agents
		if t.Name == "EnterPlanMode" || t.Name == "ExitPlanMode" {
			continue
		}

		// Apply tool access rules
		switch config.Tools.Mode {
		case ToolAccessAllowlist:
			allowed := false
			for _, name := range config.Tools.Allow {
				if strings.EqualFold(t.Name, name) {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		case ToolAccessDenylist:
			denied := false
			for _, name := range config.Tools.Deny {
				if strings.EqualFold(t.Name, name) {
					denied = true
					break
				}
			}
			if denied {
				continue
			}
		}

		filtered = append(filtered, t)
	}

	return filtered
}
