package hooks

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	promptCallback PromptCallback // optional; nil = one-shot stdin mode
}

// NewEngine creates a new hook execution engine.
func NewEngine(settings *config.Settings, sessionID, cwd, transcriptPath string) *Engine {
	return &Engine{
		settings:       settings,
		sessionID:      sessionID,
		cwd:            cwd,
		transcriptPath: transcriptPath,
		permissionMode: "default",
	}
}

// SetPermissionMode sets the current permission mode (normal, auto, plan).
func (e *Engine) SetPermissionMode(mode string) {
	e.permissionMode = mode
}

// SetPromptCallback sets the callback for bidirectional prompt exchanges.
// When set, hooks that write PromptRequest JSON to stdout will trigger the
// callback to collect user input, which is then written back to the hook's stdin.
// When nil (default), hooks use one-shot stdin mode (backward compatible).
func (e *Engine) SetPromptCallback(cb PromptCallback) {
	e.promptCallback = cb
}

// Execute runs all matching hooks for an event synchronously.
func (e *Engine) Execute(ctx context.Context, event EventType, input HookInput) HookOutcome {
	outcome := HookOutcome{ShouldContinue: true}

	hooks := e.getMatchingHooks(event, &input)
	log.Logger().Debug("Engine.Execute: matching hooks",
		zap.String("event", string(event)),
		zap.Int("count", len(hooks)))

	if len(hooks) == 0 {
		return outcome
	}

	for _, cmd := range hooks {
		log.Logger().Debug("Engine.Execute: running hook",
			zap.String("event", string(event)),
			zap.String("command", cmd.Command),
			zap.Bool("async", cmd.Async),
			zap.Int("timeout", cmd.Timeout))

		if cmd.Async {
			go e.executeCommand(context.Background(), cmd, input)
			continue
		}

		var result HookOutcome
		if e.promptCallback != nil {
			result = e.executeCommandBidirectional(ctx, cmd, input)
		} else {
			result = e.executeCommand(ctx, cmd, input)
		}
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
	if result.PermissionAllow {
		outcome.PermissionAllow = true
		outcome.HookSource = result.HookSource
	}
	if len(result.UpdatedPermissions) > 0 {
		outcome.UpdatedPermissions = append(outcome.UpdatedPermissions, result.UpdatedPermissions...)
	}
	return outcome
}

// ExecuteAsync runs all matching hooks asynchronously (fire-and-forget).
func (e *Engine) ExecuteAsync(event EventType, input HookInput) {
	hooks := e.getMatchingHooks(event, &input)
	if len(hooks) == 0 {
		return
	}
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

// SetTranscriptPath updates the transcript path after engine creation.
// This is useful when the session file path is determined lazily.
func (e *Engine) SetTranscriptPath(path string) {
	e.transcriptPath = path
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

	runErr := cmd.Run()
	exitCode := getExitCode(runErr)
	if exitCode < 0 {
		outcome.Error = runErr
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
	switch hso.PermissionDecision {
	case "deny":
		outcome.ShouldContinue = false
		outcome.ShouldBlock = true
		outcome.BlockReason = hso.PermissionDecisionReason
	case "allow":
		outcome.PermissionAllow = true
		outcome.HookSource = "PreToolUse"
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
	switch prd.Behavior {
	case "deny":
		outcome.ShouldContinue = false
		outcome.ShouldBlock = true
		outcome.BlockReason = "denied by hook"
		if prd.Message != "" {
			outcome.BlockReason = prd.Message
		}
	case "allow":
		outcome.PermissionAllow = true
		outcome.HookSource = "PermissionRequest"
		if prd.UpdatedInput != nil {
			outcome.UpdatedInput = prd.UpdatedInput
		}
		// Extract structured updatedPermissions
		for _, p := range prd.UpdatedPermissions {
			pu := parsePermissionUpdate(p)
			if pu.Type != "" {
				outcome.UpdatedPermissions = append(outcome.UpdatedPermissions, pu)
			}
		}
		return outcome
	}

	if prd.Interrupt {
		outcome.ShouldContinue = false
		outcome.ShouldBlock = true
	}

	if prd.UpdatedInput != nil {
		outcome.UpdatedInput = prd.UpdatedInput
	}

	return outcome
}

// executeCommandBidirectional runs a hook with stdin kept open for multi-turn
// prompt exchanges. The protocol:
//  1. Write input JSON + "\n" to stdin
//  2. Read stdout line-by-line:
//     a. First line: if {"async":true}, detach to goroutine and return
//     b. If line is a PromptRequest, call promptCallback, write response to stdin
//     c. Otherwise, treat as final HookOutput
//  3. Close stdin, wait for exit
func (e *Engine) executeCommandBidirectional(ctx context.Context, hookCmd config.HookCmd, input HookInput) HookOutcome {
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
	cmd.Env = e.buildEnv(input)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		outcome.Error = fmt.Errorf("failed to create stdin pipe: %w", err)
		return outcome
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		outcome.Error = fmt.Errorf("failed to create stdout pipe: %w", err)
		return outcome
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		outcome.Error = fmt.Errorf("failed to start hook: %w", err)
		return outcome
	}

	// Write input JSON to stdin
	if _, err := io.WriteString(stdinPipe, string(inputJSON)+"\n"); err != nil {
		outcome.Error = fmt.Errorf("failed to write to stdin: %w", err)
		_ = cmd.Wait()
		return outcome
	}

	// Read stdout line-by-line
	scanner := bufio.NewScanner(stdoutPipe)
	var finalOutput string
	firstLine := true

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// First line: check for async detection
		if firstLine {
			firstLine = false
			var async asyncFirstLine
			if json.Unmarshal([]byte(line), &async) == nil && async.Async {
				// Detach: close stdin and let process run in background
				stdinPipe.Close()
				go func() { _ = cmd.Wait() }()
				log.Logger().Debug("hook self-detected as async, backgrounded",
					zap.String("command", hookCmd.Command))
				return outcome
			}
		}

		// Try to parse as PromptRequest
		var promptReq PromptRequest
		if err := json.Unmarshal([]byte(line), &promptReq); err == nil && promptReq.Prompt != "" && promptReq.Message != "" {
			if e.promptCallback == nil {
				// No callback — cannot handle prompt requests in this mode
				log.Logger().Warn("hook sent prompt request but no callback is set",
					zap.String("command", hookCmd.Command))
				continue
			}
			resp, cancelled := e.promptCallback(promptReq)
			if cancelled {
				stdinPipe.Close()
				_ = cmd.Wait()
				return outcome
			}
			respJSON, err := json.Marshal(resp)
			if err != nil {
				log.Logger().Warn("failed to marshal prompt response", zap.Error(err))
				continue
			}
			if _, err := io.WriteString(stdinPipe, string(respJSON)+"\n"); err != nil {
				log.Logger().Warn("failed to write prompt response to stdin", zap.Error(err))
			}
			continue
		}

		// Not a prompt request — this is the final output line
		finalOutput = line
	}

	// Close stdin and wait for process to exit
	stdinPipe.Close()
	exitCode := getExitCode(cmd.Wait())

	if exitCode == 2 {
		return e.handleBlockingExit(&stderr)
	}

	if exitCode != 0 && exitCode >= 0 {
		log.Logger().Debug("hook exited with non-zero code",
			zap.Int("exitCode", exitCode),
			zap.String("stderr", stderr.String()))
		return outcome
	}

	return e.parseOutput(finalOutput, outcome)
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

// parsePermissionUpdate converts an untyped permission update (from JSON []any)
// into a structured PermissionUpdate. Handles both map objects and legacy strings.
func parsePermissionUpdate(v any) PermissionUpdate {
	m, ok := v.(map[string]any)
	if !ok {
		// Legacy string format: treat as an addRules with a single rule string
		if s, ok := v.(string); ok && s != "" {
			return PermissionUpdate{
				Type:        "addRules",
				Behavior:    "allow",
				Destination: "session",
				Rules:       []PermissionRule{{RuleContent: s}},
			}
		}
		return PermissionUpdate{}
	}

	pu := PermissionUpdate{
		Type:        getString(m, "type"),
		Mode:        getString(m, "mode"),
		Behavior:    getString(m, "behavior"),
		Destination: getString(m, "destination"),
	}

	// Parse rules array
	if rules, ok := m["rules"].([]any); ok {
		for _, r := range rules {
			if rm, ok := r.(map[string]any); ok {
				pu.Rules = append(pu.Rules, PermissionRule{
					ToolName:    getString(rm, "toolName"),
					RuleContent: getString(rm, "ruleContent"),
				})
			}
		}
	}

	// Parse directories array
	if dirs, ok := m["directories"].([]any); ok {
		for _, d := range dirs {
			if s, ok := d.(string); ok {
				pu.Directories = append(pu.Directories, s)
			}
		}
	}

	return pu
}

// getString safely extracts a string value from a map.
func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
