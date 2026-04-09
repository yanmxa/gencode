package hooks

import (
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/config"
)

type matchedHook struct {
	Event   EventType
	Matcher string
	Source  string
	Command *config.HookCmd
	Func    *FunctionHook
}

func (e *Engine) getMatchingHooks(event EventType, input *HookInput) []matchedHook {
	e.populateInputFields(input, event)
	matchValue := GetMatchValue(event, *input)

	var matched []matchedHook
	for _, source := range e.collectHooks(event) {
		if !MatchesEvent(source.Matcher, matchValue) {
			continue
		}
		for _, cmd := range source.Hooks {
			if !e.matchesIfCondition(cmd, *input) {
				continue
			}
			cmdCopy := cmd
			hook := matchedHook{
				Event:   event,
				Matcher: source.Matcher,
				Command: &cmdCopy,
				Source:  source.Source,
			}
			if !e.shouldRunHook(hook) {
				continue
			}
			matched = append(matched, hook)
		}
	}
	for _, source := range e.collectFunctionHooks(event) {
		if !MatchesEvent(source.Matcher, matchValue) {
			continue
		}
		for _, fn := range source.Hooks {
			fnCopy := fn
			hook := matchedHook{
				Event:   event,
				Matcher: source.Matcher,
				Func:    &fnCopy,
				Source:  source.Source,
			}
			if !e.shouldRunHook(hook) {
				continue
			}
			matched = append(matched, hook)
		}
	}
	return matched
}

func (e *Engine) collectHooks(event EventType) []hookSource {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var hooks []hookSource
	if e.settings != nil {
		for _, hook := range e.settings.Hooks[string(event)] {
			hooks = append(hooks, hookSource{
				Matcher: hook.Matcher,
				Hooks:   hook.Hooks,
				Source:  "settings",
			})
		}
	}
	for _, hook := range e.runtimeHooks[event] {
		hooks = append(hooks, hookSource{
			Matcher: hook.Matcher,
			Hooks:   hook.Hooks,
			Source:  "runtime",
		})
	}
	for _, hook := range e.sessionHooks[event] {
		hooks = append(hooks, hookSource{
			Matcher: hook.Matcher,
			Hooks:   hook.Hooks,
			Source:  "session",
		})
	}
	return hooks
}

type hookSource struct {
	Matcher string
	Hooks   []config.HookCmd
	Source  string
}

type functionHookSource struct {
	Matcher string
	Hooks   []FunctionHook
	Source  string
}

func (e *Engine) populateInputFields(input *HookInput, event EventType) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	input.SessionID = e.sessionID
	input.TranscriptPath = e.transcriptPath
	input.Cwd = e.cwd
	input.HookEventName = string(event)

	switch event {
	case SessionStart, SessionEnd, Notification, SubagentStart, PreCompact:
	default:
		input.PermissionMode = e.permissionMode
	}
}

func (e *Engine) matchesIfCondition(cmd config.HookCmd, input HookInput) bool {
	if cmd.If == "" {
		return true
	}
	switch input.HookEventName {
	case string(PreToolUse), string(PostToolUse), string(PostToolUseFailure), string(PermissionRequest), string(PermissionDenied):
		rule := config.BuildRule(input.ToolName, input.ToolInput)
		if config.MatchesToolPattern(input.ToolName, input.ToolInput, rule, cmd.If) {
			return true
		}
		if input.ToolName == "Bash" {
			if raw, ok := input.ToolInput["command"].(string); ok {
				return config.MatchRule("Bash("+strings.TrimSpace(raw)+")", cmd.If)
			}
		}
		return false
	default:
		return false
	}
}

func (e *Engine) shouldRunHook(hook matchedHook) bool {
	if !hookRunsOnce(hook) {
		return true
	}

	key := fmt.Sprintf("%s|%s|%s|%s|%s", hook.Event, hook.Source, hook.Matcher, matchedHookType(hook), matchedHookIdentity(hook))
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.executedOnce[key]; ok {
		return false
	}
	e.executedOnce[key] = struct{}{}
	return true
}

func (e *Engine) collectFunctionHooks(event EventType) []functionHookSource {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var hooks []functionHookSource
	for _, hook := range e.runtimeFuncs[event] {
		hookCopy := hook.Hook
		hooks = append(hooks, functionHookSource{
			Matcher: hook.Matcher,
			Hooks:   []FunctionHook{hookCopy},
			Source:  "runtime",
		})
	}
	for _, hook := range e.sessionFuncs[event] {
		hookCopy := hook.Hook
		hooks = append(hooks, functionHookSource{
			Matcher: hook.Matcher,
			Hooks:   []FunctionHook{hookCopy},
			Source:  "session",
		})
	}
	return hooks
}

func hookRunsOnce(hook matchedHook) bool {
	if hook.Command != nil {
		return hook.Command.Once
	}
	if hook.Func != nil {
		return hook.Func.Once
	}
	return false
}

func matchedHookType(hook matchedHook) string {
	if hook.Func != nil {
		return "function"
	}
	if hook.Command != nil {
		return normalizedHookType(*hook.Command)
	}
	return ""
}

func matchedHookIdentity(hook matchedHook) string {
	if hook.Func != nil {
		return hook.Func.ID
	}
	if hook.Command != nil {
		return hookIdentity(*hook.Command)
	}
	return ""
}

func matchedHookStatusMessage(hook matchedHook) string {
	if hook.Func != nil {
		return hook.Func.StatusMessage
	}
	if hook.Command != nil {
		return hook.Command.StatusMessage
	}
	return ""
}

func normalizedHookType(cmd config.HookCmd) string {
	if cmd.Type == "" {
		return "command"
	}
	return cmd.Type
}

func hookIdentity(cmd config.HookCmd) string {
	switch normalizedHookType(cmd) {
	case "prompt", "agent":
		return cmd.Prompt + "|" + cmd.If + "|" + cmd.Model
	case "http":
		return cmd.URL + "|" + cmd.If
	default:
		return cmd.Command + "|" + cmd.If + "|" + cmd.Shell
	}
}
