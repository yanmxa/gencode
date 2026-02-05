package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/log"
	"go.uber.org/zap"
)

// DefaultTimeout is the default timeout for hook commands in seconds.
const DefaultTimeout = 600

// Engine executes hooks based on events.
type Engine struct {
	settings       *config.Settings
	sessionID      string
	cwd            string
	transcriptPath string
	permissionMode string
}

// NewEngine creates a new hook execution engine.
func NewEngine(settings *config.Settings, sessionID, cwd, transcriptPath string) *Engine {
	return &Engine{
		settings:       settings,
		sessionID:      sessionID,
		cwd:            cwd,
		transcriptPath: transcriptPath,
		permissionMode: "normal",
	}
}

// SetPermissionMode sets the current permission mode (normal, auto, plan).
func (e *Engine) SetPermissionMode(mode string) {
	e.permissionMode = mode
}

// Execute runs all matching hooks for an event synchronously.
func (e *Engine) Execute(ctx context.Context, event EventType, input HookInput) HookOutcome {
	outcome := HookOutcome{ShouldContinue: true}

	hooks := e.getMatchingHooks(event, &input)
	if len(hooks) == 0 {
		return outcome
	}

	for _, cmd := range hooks {
		if cmd.Async {
			go e.executeCommand(context.Background(), cmd, input)
			continue
		}

		result := e.executeCommand(ctx, cmd, input)
		if result.Error != nil {
			log.Logger().Warn("hook execution failed",
				zap.String("event", string(event)),
				zap.String("command", cmd.Command),
				zap.Error(result.Error))
			continue
		}

		if !result.ShouldContinue {
			return result
		}

		outcome = e.mergeOutcome(outcome, result)
	}

	return outcome
}

// mergeOutcome merges result into outcome.
func (e *Engine) mergeOutcome(outcome, result HookOutcome) HookOutcome {
	outcome.AdditionalContext = appendContext(outcome.AdditionalContext, result.AdditionalContext)
	if result.UpdatedInput != nil {
		outcome.UpdatedInput = result.UpdatedInput
	}
	return outcome
}

// ExecuteAsync runs all matching hooks asynchronously (fire-and-forget).
func (e *Engine) ExecuteAsync(event EventType, input HookInput) {
	hooks := e.getMatchingHooks(event, &input)
	for _, cmd := range hooks {
		cmdCopy, inputCopy := cmd, input
		go e.executeCommand(context.Background(), cmdCopy, inputCopy)
	}
}

// HasHooks returns true if there are any hooks configured for the given event.
func (e *Engine) HasHooks(event EventType) bool {
	if e.settings == nil {
		return false
	}
	hooks, ok := e.settings.Hooks[string(event)]
	return ok && len(hooks) > 0
}

// getMatchingHooks returns all hook commands that match the event and input.
func (e *Engine) getMatchingHooks(event EventType, input *HookInput) []config.HookCmd {
	if e.settings == nil {
		return nil
	}

	hooks, ok := e.settings.Hooks[string(event)]
	if !ok {
		return nil
	}

	e.populateInputFields(input, event)
	matchValue := GetMatchValue(event, *input)

	var cmds []config.HookCmd
	for _, hook := range hooks {
		if MatchesEvent(hook.Matcher, matchValue) {
			cmds = append(cmds, e.extractCommands(hook.Hooks)...)
		}
	}
	return cmds
}

// populateInputFields fills common fields in hook input.
func (e *Engine) populateInputFields(input *HookInput, event EventType) {
	input.SessionID = e.sessionID
	input.TranscriptPath = e.transcriptPath
	input.Cwd = e.cwd
	input.PermissionMode = e.permissionMode
	input.HookEventName = string(event)
}

// extractCommands filters and returns command-type hooks.
func (e *Engine) extractCommands(hooks []config.HookCmd) []config.HookCmd {
	var cmds []config.HookCmd
	for _, cmd := range hooks {
		if cmd.Type == "" || cmd.Type == "command" {
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

// executeCommand runs a single hook command.
func (e *Engine) executeCommand(ctx context.Context, hookCmd config.HookCmd, input HookInput) HookOutcome {
	outcome := HookOutcome{ShouldContinue: true}

	if hookCmd.Command == "" {
		return outcome
	}

	timeout := DefaultTimeout
	if hookCmd.Timeout > 0 {
		timeout = hookCmd.Timeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	inputJSON, err := json.Marshal(input)
	if err != nil {
		outcome.Error = fmt.Errorf("failed to marshal input: %w", err)
		return outcome
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", hookCmd.Command)
	cmd.Dir = e.cwd
	cmd.Stdin = bytes.NewReader(inputJSON)
	cmd.Env = e.buildEnv(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := getExitCode(cmd.Run())
	if exitCode < 0 {
		outcome.Error = err
		return outcome
	}

	if exitCode == 2 {
		return e.handleBlockingExit(&stderr)
	}

	if exitCode != 0 {
		log.Logger().Debug("hook exited with non-zero code",
			zap.Int("exitCode", exitCode),
			zap.String("stderr", stderr.String()))
		return outcome
	}

	return e.parseOutput(strings.TrimSpace(stdout.String()), outcome)
}

// handleBlockingExit creates an outcome for exit code 2 (blocking error).
func (e *Engine) handleBlockingExit(stderr *bytes.Buffer) HookOutcome {
	reason := strings.TrimSpace(stderr.String())
	if reason == "" {
		reason = "Hook blocked execution"
	}
	return HookOutcome{
		ShouldContinue: false,
		ShouldBlock:    true,
		BlockReason:    reason,
	}
}

// buildEnv creates environment variables for the hook command.
func (e *Engine) buildEnv(input HookInput) []string {
	env := append(os.Environ(),
		"GEN_PROJECT_DIR="+e.cwd,
		"GEN_SESSION_ID="+e.sessionID,
		"GEN_EVENT_TYPE="+input.HookEventName,
		"CLAUDE_PROJECT_DIR="+e.cwd,
		"CLAUDE_SESSION_ID="+e.sessionID,
		"CLAUDE_EVENT_TYPE="+input.HookEventName,
	)
	if input.ToolName != "" {
		env = append(env,
			"GEN_TOOL_NAME="+input.ToolName,
			"CLAUDE_TOOL_NAME="+input.ToolName,
		)
	}
	return env
}

// getExitCode extracts exit code from error. Returns -1 for non-exit errors.
func getExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

// parseOutput parses hook JSON output and updates the outcome.
func (e *Engine) parseOutput(output string, outcome HookOutcome) HookOutcome {
	if output == "" {
		return outcome
	}

	var hookOutput HookOutput
	if err := json.Unmarshal([]byte(output), &hookOutput); err != nil {
		log.Logger().Debug("hook output not valid JSON", zap.String("output", output))
		return outcome
	}

	if hookOutput.Continue != nil && !*hookOutput.Continue {
		outcome.ShouldContinue = false
		outcome.ShouldBlock = true
		outcome.BlockReason = firstNonEmpty(hookOutput.StopReason, hookOutput.Reason)
	}

	if hookOutput.SystemMessage != "" {
		outcome.AdditionalContext = hookOutput.SystemMessage
	}

	if hso := hookOutput.HookSpecificOutput; hso != nil {
		outcome = e.applySpecificOutput(outcome, hso)
	}

	return outcome
}

// applySpecificOutput applies hook-specific output to the outcome.
func (e *Engine) applySpecificOutput(outcome HookOutcome, hso *HookSpecificOutput) HookOutcome {
	if hso.PermissionDecision == "deny" {
		outcome.ShouldContinue = false
		outcome.ShouldBlock = true
		outcome.BlockReason = hso.PermissionDecisionReason
	}

	if hso.UpdatedInput != nil {
		outcome.UpdatedInput = hso.UpdatedInput
	}

	outcome.AdditionalContext = appendContext(outcome.AdditionalContext, hso.AdditionalContext)

	if prd := hso.PermissionRequestDecision; prd != nil {
		outcome = e.applyPermissionDecision(outcome, prd)
	}

	return outcome
}

// applyPermissionDecision applies permission decision to the outcome.
func (e *Engine) applyPermissionDecision(outcome HookOutcome, prd *PermissionRequestDecision) HookOutcome {
	if prd.Behavior == "deny" || prd.Interrupt {
		outcome.ShouldContinue = false
		outcome.ShouldBlock = true
		if prd.Message != "" {
			outcome.BlockReason = prd.Message
		}
	}

	if prd.UpdatedInput != nil {
		outcome.UpdatedInput = prd.UpdatedInput
	}

	return outcome
}

// appendContext appends b to a with newline separator if both non-empty.
func appendContext(a, b string) string {
	if b == "" {
		return a
	}
	if a == "" {
		return b
	}
	return a + "\n" + b
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(strs ...string) string {
	for _, s := range strs {
		if s != "" {
			return s
		}
	}
	return ""
}
