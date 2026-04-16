package config

import (
	"net/url"
	"path/filepath"
	"strings"
)

// safeTools is the allowlist of tools that skip permission checks.
// This is a local copy to avoid importing the higher-layer tool package.
// IMPORTANT: keep in sync with tool.safeTools (tool/classification.go) and
// permission.safeTools (permission/permission.go). The config/permission_test.go
// TestSafeToolAllowlist test validates a subset of these.
var safeTools = map[string]bool{
	"Read": true, "Glob": true, "Grep": true,
	"WebFetch": true, "WebSearch": true, "LSP": true,
	"TaskCreate": true, "TaskGet": true, "TaskList": true, "TaskUpdate": true,
	"AskUserQuestion": true, "EnterPlanMode": true, "ExitPlanMode": true,
	"CronList": true, "ToolSearch": true,
}

// PermissionBehavior represents the outcome of a permission check.
type PermissionBehavior int

const (
	// Allow means the action is automatically allowed.
	Allow PermissionBehavior = iota

	// Deny means the action is automatically denied.
	Deny

	// Ask means the action requires user confirmation.
	Ask

	// Passthrough means the tool has no opinion — defer to mode/default logic.
	// This is used when a tool's custom checkPermissions returns no decision.
	Passthrough
)

// String returns a human-readable representation of the permission behavior.
func (p PermissionBehavior) String() string {
	switch p {
	case Allow:
		return "allow"
	case Deny:
		return "deny"
	case Ask:
		return "ask"
	case Passthrough:
		return "passthrough"
	default:
		return "unknown"
	}
}

// PermissionDecision carries a permission behavior together with the reason
// for the decision, enabling callers to log or display why access was
// granted, denied, or requires confirmation.
type PermissionDecision struct {
	Behavior PermissionBehavior
	Reason   string // e.g. "deny rule: Read(**/.env)", "bypass-immune: .git/ directory"
}

// decide is a shorthand for building a PermissionDecision.
func decide(b PermissionBehavior, reason string) PermissionDecision {
	return PermissionDecision{Behavior: b, Reason: reason}
}

// ---------------------------------------------------------------------------
// Main permission gate — HasPermissionToUseTool
// ---------------------------------------------------------------------------
//
// Decision pipeline (inspired by Claude Code's hasPermissionsToUseTool):
//
//  1. Deny rules + bypass-immune safety checks + ask rules (via checkHardBlocks)
//     — deny rules cannot be bypassed; safety checks always prompt
//  2. BypassPermissions mode → allow (everything except step 1)
//  3. Session permissions (runtime overrides)
//  4. Allow rules
//  5. Default (safe tools → allow, others → ask)
//  6. Mode transforms: DontAsk converts ask → deny

// HasPermissionToUseTool is the central permission gate that determines
// whether a tool invocation should be allowed, denied, or prompted.
func (s *Settings) HasPermissionToUseTool(toolName string, args map[string]any, session *SessionPermissions) PermissionDecision {
	rule := BuildRule(toolName, args)

	// ── Step 1: Deny rules + bypass-immune safety checks ──
	if reason := s.checkHardBlocks(toolName, args, rule, session); reason != "" {
		if s.isDenyRule(reason) {
			return decide(Deny, reason)
		}
		return decide(Ask, reason)
	}

	// ── Step 2: BypassPermissions mode ──
	if session != nil && session.Mode == ModeBypassPermissions {
		return decide(Allow, "mode: bypass permissions")
	}

	// ── Step 3: Session permissions ──
	if session != nil {
		if session.IsToolAllowed(toolName) {
			return decide(Allow, "session: allow all "+toolName)
		}
		for pattern := range session.AllowedPatterns {
			if MatchesToolPattern(toolName, args, rule, pattern) {
				return decide(Allow, "session pattern: "+pattern)
			}
		}
	}

	// ── Step 4: Allow rules ──
	for _, pattern := range s.Permissions.Allow {
		if MatchesToolPattern(toolName, args, rule, pattern) {
			return decide(Allow, "allow rule: "+pattern)
		}
	}

	// Note: Ask rules are already checked in checkHardBlocks (Step 1).

	// ── Step 5: Default ──
	result := decide(Ask, "default: requires confirmation")
	if safeTools[toolName] {
		result = decide(Allow, "default: safe tool")
	}

	// ── Step 6: Mode transforms ──
	if result.Behavior == Ask && session != nil && session.Mode == ModeDontAsk {
		return decide(Deny, "mode: don't ask (auto-deny)")
	}

	return result
}

// checkHardBlocks checks deny rules, bypass-immune safety checks, working
// directory constraints, and ask rules. Returns a reason string if the action
// should be blocked/prompted, or empty string if none of these apply.
// Used by both HasPermissionToUseTool and ResolveHookAllow.
func (s *Settings) checkHardBlocks(toolName string, args map[string]any, rule string, session *SessionPermissions) string {
	// Deny rules
	for _, pattern := range s.Permissions.Deny {
		if MatchesToolPattern(toolName, args, rule, pattern) {
			return "deny rule: " + pattern
		}
	}

	// Bypass-immune: sensitive paths
	if toolName == "Edit" || toolName == "Write" {
		if fp, ok := args["file_path"].(string); ok {
			if reason := isSensitivePath(fp); reason != "" {
				return "bypass-immune: " + reason
			}
		}
	}

	// Bypass-immune: destructive/dangerous bash
	if toolName == "Bash" {
		if cmd, ok := args["command"].(string); ok {
			if isDestructiveCommand(cmd) {
				return "bypass-immune: destructive command"
			}
			if reason := checkBashSecurity(cmd); reason != "" {
				return "bypass-immune: " + reason
			}
		}
	}

	// Working directory constraints
	if session != nil && len(session.WorkingDirectories) > 0 {
		if toolName == "Edit" || toolName == "Write" {
			if fp, ok := args["file_path"].(string); ok {
				if !isInWorkingDirectory(fp, session.WorkingDirectories) {
					return "outside working directory"
				}
			}
		}
	}

	// Ask rules
	for _, pattern := range s.Permissions.Ask {
		if MatchesToolPattern(toolName, args, rule, pattern) {
			return "ask rule: " + pattern
		}
	}

	return ""
}

// isDenyRule returns true if the reason string indicates a deny rule (vs ask/bypass-immune).
func (s *Settings) isDenyRule(reason string) bool {
	return strings.HasPrefix(reason, "deny rule:")
}

// ResolveHookAllow checks if a hook's "allow" decision should be honored.
// Returns false if a deny rule, bypass-immune safety check, or explicit ask
// rule overrides the hook's decision. This implements the safety invariant:
// deny rules > bypass-immune checks > ask rules > hook allow.
func (s *Settings) ResolveHookAllow(toolName string, args map[string]any, session *SessionPermissions) bool {
	rule := BuildRule(toolName, args)
	return s.checkHardBlocks(toolName, args, rule, session) == ""
}

// CheckPermission is a convenience wrapper returning just the behavior.
func (s *Settings) CheckPermission(toolName string, args map[string]any, session *SessionPermissions) PermissionBehavior {
	return s.HasPermissionToUseTool(toolName, args, session).Behavior
}


// BuildRule builds a rule string from a tool name and arguments.
// Format: "Tool(args)"
//
// Different tools extract different parts of args:
//   - Bash: "Bash(command)" where command is the shell command
//   - Read/Edit/Write: "Read(file_path)"
//   - Glob/Grep: "Glob(pattern)" or "Grep(pattern)"
//   - WebFetch: "WebFetch(domain:hostname)"
func BuildRule(toolName string, args map[string]any) string {
	var argStr string

	switch toolName {
	case "Bash":
		// For Bash, use the command with prefix matching support
		if cmd, ok := args["command"].(string); ok {
			// Prefer the first meaningful subcommand so compound commands like
			// "cd repo && git status" build reusable rules.
			if normalized := meaningfulBashCommands(cmd); len(normalized) > 0 {
				argStr = normalized[0]
			} else {
				argStr = normalizeBashCommand(cmd)
			}
		}

	case "Read", "Edit", "Write":
		// For file tools, use the file path
		if fp, ok := args["file_path"].(string); ok {
			argStr = fp
		}

	case "Glob":
		// For Glob, use the pattern
		if p, ok := args["pattern"].(string); ok {
			argStr = p
		}

	case "Grep":
		// For Grep, use the pattern
		if p, ok := args["pattern"].(string); ok {
			argStr = p
		}

	case "WebFetch":
		// For WebFetch, extract domain from URL
		if u, ok := args["url"].(string); ok {
			if parsed, err := url.Parse(u); err == nil {
				argStr = "domain:" + parsed.Host
			} else {
				argStr = u
			}
		}

	case "Skill":
		// For Skill, use the skill name
		// Supports patterns like "Skill(git:*)", "Skill(test-skill)"
		if s, ok := args["skill"].(string); ok {
			argStr = s
		}

	default:
		// Generic: try common field names
		if fp, ok := args["file_path"].(string); ok {
			argStr = fp
		} else if p, ok := args["path"].(string); ok {
			argStr = p
		} else if p, ok := args["pattern"].(string); ok {
			argStr = p
		}
	}

	return toolName + "(" + argStr + ")"
}

// normalizeBashCommand normalizes a bash command for pattern matching.
// Examples:
//   - "npm install lodash" -> "npm:install lodash"
//   - "git commit -m 'msg'" -> "git:commit -m 'msg'"
//   - "ls -la" -> "ls:-la"
//   - "/bin/rm -rf foo" -> "rm:-rf foo" (strips path prefix)
func normalizeBashCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	parts := strings.SplitN(cmd, " ", 2)

	// Get the base command (without path)
	baseCmd := filepath.Base(parts[0])

	if len(parts) == 1 {
		return baseCmd
	}

	// Return "command:rest"
	return baseCmd + ":" + parts[1]
}

func normalizeParsedCommand(cmd parsedCommand) string {
	if cmd.Name == "" {
		return ""
	}
	if len(cmd.Args) == 0 {
		return cmd.Name
	}
	return cmd.Name + ":" + strings.Join(cmd.Args, " ")
}

func normalizedBashCommands(cmd string) []string {
	if file := parseBashAST(cmd); file != nil {
		parsed := extractCommandsAST(file)
		normalized := make([]string, 0, len(parsed))
		for _, subCmd := range parsed {
			if n := normalizeParsedCommand(subCmd); n != "" {
				normalized = append(normalized, n)
			}
		}
		if len(normalized) > 0 {
			return normalized
		}
	}

	rawCommands := extractBashCommands(cmd)
	normalized := make([]string, 0, len(rawCommands))
	for _, subCmd := range rawCommands {
		if n := normalizeBashCommand(subCmd); n != "" {
			normalized = append(normalized, n)
		}
	}
	return normalized
}

func meaningfulBashCommands(cmd string) []string {
	normalized := normalizedBashCommands(cmd)
	if len(normalized) <= 1 {
		return normalized
	}

	filtered := make([]string, 0, len(normalized))
	for _, subCmd := range normalized {
		if subCmd == "cd" || strings.HasPrefix(subCmd, "cd:") {
			continue
		}
		filtered = append(filtered, subCmd)
	}
	if len(filtered) > 0 {
		return filtered
	}
	return normalized
}

// extractBashCommands extracts individual commands from a chained bash command.
// It splits on && and ; to get each command separately.
func extractBashCommands(cmd string) []string {
	var commands []string

	// Split on && first, then on ;
	for part := range strings.SplitSeq(cmd, "&&") {
		for subPart := range strings.SplitSeq(part, ";") {
			trimmed := strings.TrimSpace(subPart)
			if trimmed != "" {
				commands = append(commands, trimmed)
			}
		}
	}

	return commands
}

// MatchesToolPattern reports whether a tool invocation matches a permission
// pattern. Bash commands match if any extracted subcommand matches.
func MatchesToolPattern(toolName string, args map[string]any, rule, pattern string) bool {
	if MatchRule(rule, pattern) {
		return true
	}

	if toolName != "Bash" {
		return false
	}

	cmd, ok := args["command"].(string)
	if !ok || cmd == "" {
		return false
	}

	for _, subCmd := range normalizedBashCommands(cmd) {
		subRule := "Bash(" + subCmd + ")"
		if MatchRule(subRule, pattern) {
			return true
		}
	}

	return false
}

// MatchRule checks if a rule matches a pattern.
// Rule format: "Tool(args)"
// Pattern format: "Tool(pattern)" where pattern supports:
//   - "*" matches any sequence of characters
//   - "**" matches any sequence including path separators
//   - "domain:" prefix for WebFetch domain matching
func MatchRule(rule, pattern string) bool {
	// Parse rule
	toolRule, argsRule := parseRule(rule)
	toolPat, argsPat := parseRule(pattern)

	// Tool names must match exactly
	if toolRule != toolPat {
		return false
	}

	// Match arguments using glob-like patterns
	return matchGlob(argsRule, argsPat)
}

// parseRule parses a rule string into tool name and arguments.
// "Bash(npm install)" -> ("Bash", "npm install")
func parseRule(s string) (tool, args string) {
	tool, args, found := strings.Cut(s, "(")
	if !found {
		return s, ""
	}
	return tool, strings.TrimSuffix(args, ")")
}

// matchGlob performs glob-like pattern matching.
// Supports:
//   - "*" matches any sequence of non-separator characters
//   - "**" matches any sequence including separators (path components)
//   - "?" matches a single character
//   - Exact string matching
func matchGlob(str, pattern string) bool {
	// Empty pattern matches empty string
	if pattern == "" {
		return str == ""
	}

	// Handle "**" pattern (matches everything)
	if pattern == "**" {
		return true
	}

	// Handle patterns with "**" (double star - matches any path)
	if strings.Contains(pattern, "**") {
		// Split on "**" and match each segment
		segments := strings.Split(pattern, "**")

		if len(segments) == 2 {
			prefix := segments[0]
			suffix := segments[1]

			// Remove leading/trailing slashes from segments for flexibility
			prefix = strings.TrimSuffix(prefix, "/")
			suffix = strings.TrimPrefix(suffix, "/")

			// Check prefix matches start
			if prefix != "" && !strings.HasPrefix(str, prefix) {
				return false
			}

			// Check suffix matches end (using simple glob for suffix like "*.go")
			if suffix != "" {
				// For suffix matching, we need to find if any suffix of the string matches the pattern
				// e.g., "*.go" should match "file.go" in "/path/to/file.go"
				// e.g., ".env.*" should match ".env.local" in "/path/to/.env.local"

				// Get the filename or last component if suffix looks like a filename pattern
				// For patterns like ".env.*" or "*.go", match against the basename
				if strings.Contains(suffix, "*") {
					// If the pattern looks like a filename (starts with . or contains .), try matching against the last path component
					// Get the last path component of the string
					lastSlash := strings.LastIndex(str, "/")
					var filename string
					if lastSlash >= 0 {
						filename = str[lastSlash+1:]
					} else {
						filename = str
					}

					// Try matching the filename against the suffix pattern
					if matchSimpleWildcard(filename, suffix) {
						return true
					}

					// Also try matching the whole remaining path (for patterns like "test/*.go")
					remaining := str
					if prefix != "" {
						remaining = strings.TrimPrefix(str, prefix)
						remaining = strings.TrimPrefix(remaining, "/")
					}
					return matchSimpleWildcard(remaining, suffix)
				}
				return strings.HasSuffix(str, suffix)
			}

			return true
		}
	}

	// Handle simple wildcard patterns
	if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
		return matchSimpleWildcard(str, pattern)
	}

	// Exact match
	return str == pattern
}

// matchSimpleWildcard matches a string against a pattern with * and ? wildcards.
// * matches any sequence of characters (including empty)
// ? matches exactly one character
func matchSimpleWildcard(str, pattern string) bool {
	// Use dynamic programming approach
	s, p := 0, 0
	starIdx, matchIdx := -1, 0

	for s < len(str) {
		if p < len(pattern) && (pattern[p] == '?' || pattern[p] == str[s]) {
			// Characters match or pattern has ?
			s++
			p++
		} else if p < len(pattern) && pattern[p] == '*' {
			// Found *, mark the position
			starIdx = p
			matchIdx = s
			p++
		} else if starIdx != -1 {
			// Mismatch after *, backtrack
			p = starIdx + 1
			matchIdx++
			s = matchIdx
		} else {
			// Hard mismatch
			return false
		}
	}

	// Check remaining pattern characters are all *
	for p < len(pattern) {
		if pattern[p] != '*' {
			return false
		}
		p++
	}

	return true
}
