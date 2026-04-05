package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/log"
	"go.uber.org/zap"
)

type preparedRun struct {
	req              AgentRequest
	cfg              *runConfig
	cwd              string
	startedAt        time.Time
	hookID           string
	progress         []string
	cleanupWorkspace func()
}

func (r *preparedRun) close() {
	if r != nil && r.cleanupWorkspace != nil {
		r.cleanupWorkspace()
	}
}

func (e *Executor) prepareRun(req AgentRequest) (*preparedRun, error) {
	if err := e.validateRequest(req); err != nil {
		return nil, err
	}

	agentCwd, cleanupWorkspace, err := e.prepareWorkspace(req)
	if err != nil {
		return nil, err
	}

	cfg, err := e.prepareRunConfig(req)
	if err != nil {
		cleanupWorkspace()
		return nil, err
	}

	return &preparedRun{
		req:              req,
		cfg:              cfg,
		cwd:              agentCwd,
		startedAt:        time.Now(),
		hookID:           fmt.Sprintf("a%016x", time.Now().UnixNano()),
		progress:         make([]string, 0, 16),
		cleanupWorkspace: cleanupWorkspace,
	}, nil
}

func (e *Executor) attachRunContext(ctx context.Context, displayName string) context.Context {
	tracker := log.NewAgentTurnTracker(displayName, nil)
	return log.WithAgentTracker(ctx, tracker)
}

func (e *Executor) logRunStart(run *preparedRun) {
	log.Logger().Info("Starting agent execution",
		zap.String("agent", run.cfg.displayName),
		zap.String("description", run.req.Description),
		zap.Int("maxTurns", run.cfg.maxTurns),
	)
}

func (e *Executor) executePreparedRun(ctx context.Context, run *preparedRun) (*core.Result, error) {
	loop, cleanupLoop, err := e.buildLoop(ctx, run.cfg, run.cwd)
	if err != nil {
		return nil, err
	}
	defer cleanupLoop()

	loop.AgentID = run.hookID
	loop.AgentType = run.req.Agent

	if err := e.loadConversation(loop, run.req); err != nil {
		return nil, err
	}

	onToolStart := e.buildOnToolStart(run.req, &run.progress)
	return loop.Run(ctx, core.RunOptions{
		MaxTurns:    run.cfg.maxTurns,
		OnToolStart: onToolStart,
	})
}

func (e *Executor) logRunCompletion(run *preparedRun, result *core.Result, success bool) {
	logFields := []zap.Field{
		zap.String("agent", run.cfg.displayName),
		zap.String("stopReason", result.StopReason),
		zap.Int("turns", result.Turns),
		zap.Int("inputTokens", result.Tokens.InputTokens),
		zap.Int("outputTokens", result.Tokens.OutputTokens),
	}
	if success {
		log.Logger().Info("Agent completed", logFields...)
		return
	}
	log.Logger().Warn("Agent completed", logFields...)
}

func (e *Executor) buildAgentResult(run *preparedRun, result *core.Result) *AgentResult {
	success, errMsg := interpretStopReason(result, run.cfg.maxTurns)
	e.logRunCompletion(run, result, success)

	agentSessionID, agentTranscriptPath := e.persistSubagentSession(
		run.cfg.displayName,
		run.cfg.modelID,
		run.req.Description,
		result.Messages,
	)
	e.fireSubagentStop(run.req, run.hookID, agentSessionID, agentTranscriptPath, result.Content)

	return &AgentResult{
		AgentID:    agentSessionID,
		AgentName:  run.cfg.displayName,
		Model:      run.cfg.modelID,
		Success:    success,
		Content:    result.Content,
		Messages:   result.Messages,
		TurnCount:  result.Turns,
		ToolUses:   result.ToolUses,
		TokenUsage: result.Tokens,
		Duration:   time.Since(run.startedAt),
		Progress:   append([]string(nil), run.progress...),
		Error:      errMsg,
	}
}

func (e *Executor) buildCancelledAgentResult(run *preparedRun, result *core.Result) *AgentResult {
	if result == nil || result.StopReason != core.StopCancelled {
		return nil
	}

	return &AgentResult{
		AgentName:  run.cfg.displayName,
		Model:      run.cfg.modelID,
		Success:    false,
		Content:    result.Content,
		Messages:   result.Messages,
		TurnCount:  result.Turns,
		ToolUses:   result.ToolUses,
		TokenUsage: result.Tokens,
		Duration:   time.Since(run.startedAt),
		Progress:   append([]string(nil), run.progress...),
		Error:      "agent cancelled",
	}
}
