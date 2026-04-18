package subagent

import (
	"context"
	"time"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/orchestration"
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
		hookID:           "a" + generateShortID(),
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
	var onToolExec func(string, map[string]any)
	if run.req.OnProgress != nil {
		onToolExec = func(name string, params map[string]any) {
			msg := formatToolProgress(name, params)
			run.progress = append(run.progress, msg)
			run.req.OnProgress(msg)
		}
	}
	ag, cleanupAgent, err := e.buildAgent(ctx, run.cfg, run.cwd, onToolExec)
	if err != nil {
		return nil, err
	}
	defer cleanupAgent()

	if err := e.loadConversation(ag, ctx, run.req); err != nil {
		return nil, err
	}

	// Inject pending messages from orchestration (for background agents)
	drainFn := e.buildDrainInjectedInputs(run.req)
	if drainFn != nil {
		for _, injected := range drainFn() {
			if injected != "" {
				ag.Append(ctx, core.UserMessage(injected, nil))
			}
		}
	}

	result, err := ag.ThinkAct(ctx)
	if err != nil {
		if result != nil {
			return result, err
		}
		return nil, err
	}

	return result, nil
}

func (e *Executor) buildDrainInjectedInputs(req AgentRequest) func() []string {
	if req.LiveTaskID == "" && req.ResumeID == "" {
		return nil
	}
	return func() []string {
		return orchestration.Default().DrainPendingMessages(req.LiveTaskID, req.ResumeID)
	}
}

func (e *Executor) logRunCompletion(run *preparedRun, result *core.Result, success bool) {
	logFields := []zap.Field{
		zap.String("agent", run.cfg.displayName),
		zap.String("stopReason", string(result.StopReason)),
		zap.Int("turns", result.Turns),
		zap.Int("inputTokens", result.TokensIn),
		zap.Int("outputTokens", result.TokensOut),
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
		AgentID:        agentSessionID,
		AgentName:      run.cfg.displayName,
		TranscriptPath: agentTranscriptPath,
		Model:          run.cfg.modelID,
		Success:        success,
		Content:        result.Content,
		Messages:       result.Messages,
		TurnCount:      result.Turns,
		ToolUses:       result.ToolUses,
		TokenUsage:     llm.TokenUsage{InputTokens: result.TokensIn, OutputTokens: result.TokensOut, TotalTokens: result.TokensIn + result.TokensOut},
		Duration:       time.Since(run.startedAt),
		Progress:       append([]string(nil), run.progress...),
		Error:          errMsg,
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
		TokenUsage: llm.TokenUsage{InputTokens: result.TokensIn, OutputTokens: result.TokensOut, TotalTokens: result.TokensIn + result.TokensOut},
		Duration:   time.Since(run.startedAt),
		Progress:   append([]string(nil), run.progress...),
		Error:      "agent cancelled",
	}
}
