package hooks

import (
	"regexp"
	"sync"

	"github.com/yanmxa/gencode/internal/core"
)

// matcherCache caches compiled regexes for matcher patterns.
// Matchers come from settings.json and are reused across many hook dispatches.
var matcherCache sync.Map // map[string]*regexp.Regexp (nil value = compile error)

// compileCached returns a cached compiled regex for the given matcher pattern,
// or nil if the pattern is invalid.
func compileCached(matcher string) *regexp.Regexp {
	if v, ok := matcherCache.Load(matcher); ok {
		re, _ := v.(*regexp.Regexp)
		return re
	}
	re, err := regexp.Compile("^(" + matcher + ")$")
	if err != nil {
		matcherCache.Store(matcher, (*regexp.Regexp)(nil))
		return nil
	}
	matcherCache.Store(matcher, re)
	return re
}

// matchesEvent checks if a matcher pattern matches the given value.
// Empty or "*" matches everything. Matcher is regex-anchored at both ends.
func matchesEvent(matcher, matchValue string) bool {
	switch matcher {
	case "", "*":
		return true
	default:
		if re := compileCached(matcher); re != nil {
			return re.MatchString(matchValue)
		}
		return matcher == matchValue
	}
}

// getMatchValue extracts the value to match against based on event type.
func getMatchValue(event EventType, input HookInput) string {
	switch event {
	case core.PreToolUse, core.PostToolUse, core.PostToolUseFailure, core.PermissionRequest, core.PermissionDenied:
		return input.ToolName
	case core.SessionStart:
		return input.Source
	case core.SessionEnd:
		return input.Reason
	case core.Notification:
		return input.NotificationType
	case core.Setup:
		return input.Trigger
	case core.SubagentStart, core.SubagentStop:
		return input.AgentType
	case core.TaskCreated, core.TaskCompleted:
		return input.TaskSubject
	case core.ConfigChange:
		return input.Source
	case core.InstructionsLoaded:
		return input.FilePath
	case core.CwdChanged:
		return input.NewCwd
	case core.FileChanged:
		return input.FilePath
	case core.PreCompact, core.PostCompact:
		return input.Trigger
	case core.WorktreeCreate:
		return input.Name
	case core.WorktreeRemove:
		return input.WorktreePath
	default:
		return ""
	}
}

// eventSupportsMatcher returns true if the event type supports matcher filtering.
func eventSupportsMatcher(event EventType) bool {
	switch event {
	case core.UserPromptSubmit, core.Stop, core.StopFailure:
		return false
	default:
		return true
	}
}
