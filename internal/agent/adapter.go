package agent

import (
	"context"
	"strings"

	"github.com/yanmxa/gencode/internal/tool"
)

// ExecutorAdapter adapts the Executor to implement tool.AgentExecutor
type ExecutorAdapter struct {
	*Executor
}

// NewExecutorAdapter creates a new adapter for the Executor
func NewExecutorAdapter(executor *Executor) *ExecutorAdapter {
	return &ExecutorAdapter{Executor: executor}
}

// Verify ExecutorAdapter implements tool.AgentExecutor
var _ tool.AgentExecutor = (*ExecutorAdapter)(nil)

// Run executes an agent and returns the result
func (a *ExecutorAdapter) Run(ctx context.Context, req tool.AgentExecRequest) (*tool.AgentExecResult, error) {
	// Convert request
	agentReq := AgentRequest{
		Agent:       req.Agent,
		Prompt:      req.Prompt,
		Description: req.Description,
		Background:  req.Background,
		ResumeID:    req.ResumeID,
		Model:       req.Model,
		MaxTurns:    req.MaxTurns,
		Cwd:         req.Cwd,
	}

	// Set up progress callback if provided
	if req.OnProgress != nil {
		agentReq.OnProgress = ProgressCallback(req.OnProgress)
	}

	// Run executor
	result, err := a.Executor.Run(ctx, agentReq)
	if err != nil {
		return nil, err
	}

	// Convert result
	return &tool.AgentExecResult{
		AgentName:   result.AgentName,
		Success:     result.Success,
		Content:     result.Content,
		TurnCount:   result.TurnCount,
		TotalTokens: result.TokenUsage.TotalTokens,
		Error:       result.Error,
	}, nil
}

// RunBackground executes an agent in background
func (a *ExecutorAdapter) RunBackground(req tool.AgentExecRequest) (tool.AgentTaskInfo, error) {
	// Convert request
	agentReq := AgentRequest{
		Agent:       req.Agent,
		Prompt:      req.Prompt,
		Description: req.Description,
		Background:  true,
		ResumeID:    req.ResumeID,
		Model:       req.Model,
		MaxTurns:    req.MaxTurns,
		Cwd:         req.Cwd,
	}

	// Run in background
	agentTask, err := a.Executor.RunBackground(agentReq)
	if err != nil {
		return tool.AgentTaskInfo{}, err
	}

	return tool.AgentTaskInfo{
		TaskID:    agentTask.GetID(),
		AgentName: agentTask.AgentName,
	}, nil
}

// GetParentModelID returns the parent conversation's model ID
func (a *ExecutorAdapter) GetParentModelID() string {
	return a.Executor.GetParentModelID()
}

// GetAgentConfig returns configuration for an agent type
// Returns false if agent is not found or is disabled
func (a *ExecutorAdapter) GetAgentConfig(agentType string) (tool.AgentConfigInfo, bool) {
	// Check if agent is enabled
	if !DefaultRegistry.IsEnabled(agentType) {
		return tool.AgentConfigInfo{}, false
	}

	config, ok := DefaultRegistry.Get(agentType)
	if !ok {
		return tool.AgentConfigInfo{}, false
	}

	// Build tool list
	var tools []string
	switch config.Tools.Mode {
	case ToolAccessAllowlist:
		tools = config.Tools.Allow
	case ToolAccessDenylist:
		tools = []string{"All except: " + strings.Join(config.Tools.Deny, ", ")}
	}

	return tool.AgentConfigInfo{
		Name:           config.Name,
		Description:    config.Description,
		PermissionMode: string(config.PermissionMode),
		Tools:          tools,
	}, true
}
