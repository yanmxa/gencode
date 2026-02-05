package hooks

import "regexp"

// MatchesEvent checks if a matcher pattern matches the given value.
// Empty or "*" matches everything. Matcher is regex-anchored at both ends.
func MatchesEvent(matcher, matchValue string) bool {
	switch matcher {
	case "", "*":
		return true
	default:
		if re, err := regexp.Compile("^(" + matcher + ")$"); err == nil {
			return re.MatchString(matchValue)
		}
		return matcher == matchValue
	}
}

// GetMatchValue extracts the value to match against based on event type.
func GetMatchValue(event EventType, input HookInput) string {
	switch event {
	case PreToolUse, PostToolUse, PostToolUseFailure, PermissionRequest:
		return input.ToolName
	case SessionStart:
		return input.Source
	case SessionEnd:
		return input.Reason
	case Notification:
		return input.NotificationType
	case SubagentStart, SubagentStop:
		return input.AgentType
	case PreCompact:
		return input.Trigger
	default:
		return ""
	}
}

// EventSupportsMatcher returns true if the event type supports matcher filtering.
func EventSupportsMatcher(event EventType) bool {
	return event != UserPromptSubmit && event != Stop
}
