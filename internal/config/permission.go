package config

import (
	"net/url"
	"path/filepath"
	"strings"
)

// PermissionResult represents the result of a permission check.
type PermissionResult int

const (
	// PermissionAllow means the action is automatically allowed.
	PermissionAllow PermissionResult = iota

	// PermissionDeny means the action is automatically denied.
	PermissionDeny

	// PermissionAsk means the action requires user confirmation.
	PermissionAsk
)

// String returns a human-readable representation of the permission result.
func (p PermissionResult) String() string {
	switch p {
	case PermissionAllow:
		return "allow"
	case PermissionDeny:
		return "deny"
	case PermissionAsk:
		return "ask"
	default:
		return "unknown"
	}
}

// ReadOnlyTools is a list of tools that are considered read-only.
// These tools don't modify any files or state.
var ReadOnlyTools = map[string]bool{
	"Read":      true,
	"Glob":      true,
	"Grep":      true,
	"WebFetch":  true,
	"WebSearch": true,
}

// IsReadOnlyTool returns true if the tool is read-only.
func IsReadOnlyTool(toolName string) bool {
	return ReadOnlyTools[toolName]
}

// CheckPermission checks if a tool action is allowed based on settings and session permissions.
// Priority:
//  1. Deny rules (highest priority - cannot be bypassed by session permissions)
//  2. Destructive command protection (always ask for dangerous bash commands)
//  3. Session permissions (runtime, e.g., "allow all edits this session")
//  4. Allow rules
//  5. Ask rules
//  6. Default behavior (read-only tools allowed, others need confirmation)
func (s *Settings) CheckPermission(toolName string, args map[string]any, session *SessionPermissions) PermissionResult {
	// Build the rule string for this tool invocation
	rule := BuildRule(toolName, args)

	// SECURITY: Check deny rules FIRST - deny rules cannot be bypassed by session permissions
	for _, pattern := range s.Permissions.Deny {
		if MatchRule(rule, pattern) {
			return PermissionDeny
		}
	}

	// SECURITY: Check for destructive Bash commands - always require confirmation
	if toolName == "Bash" {
		if cmd, ok := args["command"].(string); ok {
			if IsDestructiveCommand(cmd) {
				return PermissionAsk // Always ask for destructive commands
			}
		}
	}

	// Check session permissions (after security checks)
	if session != nil {
		if session.IsToolAllowed(toolName) {
			return PermissionAllow
		}
		// Check session allowed patterns using MatchRule
		for pattern := range session.AllowedPatterns {
			if MatchRule(rule, pattern) {
				return PermissionAllow
			}
		}
		// For Bash commands, also check each command in a chained command
		if toolName == "Bash" {
			if cmd, ok := args["command"].(string); ok {
				commands := extractBashCommands(cmd)
				for _, subCmd := range commands {
					subRule := "Bash(" + normalizeBashCommand(subCmd) + ")"
					for pattern := range session.AllowedPatterns {
						if MatchRule(subRule, pattern) {
							return PermissionAllow
						}
					}
				}
			}
		}
	}

	// Check allow rules
	for _, pattern := range s.Permissions.Allow {
		if MatchRule(rule, pattern) {
			return PermissionAllow
		}
	}

	// Check ask rules
	for _, pattern := range s.Permissions.Ask {
		if MatchRule(rule, pattern) {
			return PermissionAsk
		}
	}

	// Default behavior
	if IsReadOnlyTool(toolName) {
		return PermissionAllow
	}
	return PermissionAsk
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
			// Extract command prefix (e.g., "npm install" -> "npm:install")
			// This allows patterns like "Bash(npm:*)"
			argStr = normalizeBashCommand(cmd)
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

// extractBashCommands extracts individual commands from a chained bash command.
// It splits on && and ; to get each command separately.
func extractBashCommands(cmd string) []string {
	var commands []string

	// Split on && first, then on ;
	parts := strings.Split(cmd, "&&")
	for _, part := range parts {
		subParts := strings.Split(part, ";")
		for _, subPart := range subParts {
			trimmed := strings.TrimSpace(subPart)
			if trimmed != "" {
				commands = append(commands, trimmed)
			}
		}
	}

	return commands
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

// CommonDenyPatterns contains commonly denied patterns for security.
var CommonDenyPatterns = []string{
	"Read(**/.env)",
	"Read(**/.env.*)",
	"Read(**/secrets/**)",
	"Read(**/*credentials*)",
	"Read(**/*password*)",
	"Read(**/.aws/**)",
	"Read(**/.ssh/**)",
	"Edit(**/.env)",
	"Edit(**/.env.*)",
	"Write(**/.env)",
	"Write(**/.env.*)",
}

// DestructiveCommands are patterns that should always require user confirmation,
// even when session permissions like AllowAllBash are enabled.
// These commands can cause irreversible data loss or system damage.
var DestructiveCommands = []string{
	"rm:-rf",
	"rm:-fr",
	"rm:-r",
	"git:reset --hard",
	"git:clean -fd",
	"git:clean -f",
	"git:push --force",
	"git:push -f",
	"chmod:777",
	"chmod:-R 777",
	":(){ :|:& };:", // fork bomb
	"> /dev/",       // device writes
	"dd:if=",        // direct disk access
	"mkfs",          // filesystem creation
	"fdisk",         // disk partitioning
}

// IsDestructiveCommand checks if a bash command matches any destructive pattern.
// Returns true if the command should always require user confirmation.
func IsDestructiveCommand(cmd string) bool {
	normalized := normalizeBashCommand(cmd)
	for _, pattern := range DestructiveCommands {
		if strings.Contains(normalized, pattern) {
			return true
		}
	}
	return false
}

// CommonAllowPatterns contains commonly allowed patterns.
var CommonAllowPatterns = []string{
	"Bash(git:*)",
	"Bash(npm:*)",
	"Bash(yarn:*)",
	"Bash(pnpm:*)",
	"Bash(go:*)",
	"Bash(make:*)",
	"Bash(ls:*)",
	"Bash(cat:*)",
	"Bash(head:*)",
	"Bash(tail:*)",
	"Bash(pwd)",
}
