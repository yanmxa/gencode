package config

import (
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

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

// Backward-compatible aliases for callers that use the old names.
// These will be removed after all callers are migrated.
const (
	PermissionAllow PermissionBehavior = Allow
	PermissionDeny  PermissionBehavior = Deny
	PermissionAsk   PermissionBehavior = Ask
)

// PermissionResult is the old name — alias kept for backward compatibility.
type PermissionResult = PermissionBehavior

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

// SafeTools is the allowlist of tools that can skip permission checks entirely.
// These tools are inherently safe (read-only, task management, UI).
// Inspired by Claude Code's auto-mode safe tool allowlist.
var SafeTools = map[string]bool{
	// Read-only tools
	"Read":      true,
	"Glob":      true,
	"Grep":      true,
	"WebFetch":  true,
	"WebSearch": true,
	"LSP":       true,
	// Task management
	"TaskCreate": true,
	"TaskGet":    true,
	"TaskList":   true,
	"TaskUpdate": true,
	// UI / plan
	"AskUserQuestion": true,
	"EnterPlanMode":   true,
	"ExitPlanMode":    true,
	// Team coordination
	"TeamCreate": true,
	"TeamDelete": true,
	// Cron (read-only listing)
	"CronList": true,
	// Tool discovery
	"ToolSearch": true,
}

// IsSafeTool returns true if the tool is on the safe allowlist and can skip
// permission checks entirely.
func IsSafeTool(toolName string) bool {
	return SafeTools[toolName]
}

// PermissionDecision carries a permission behavior together with the reason
// for the decision, enabling callers to log or display why access was
// granted, denied, or requires confirmation.
type PermissionDecision struct {
	Behavior PermissionBehavior
	Reason   string // e.g. "deny rule: Read(**/.env)", "bypass-immune: .git/ directory"
}

// Result returns the behavior (backward-compatible helper used by old callers).
func (d PermissionDecision) Result() PermissionBehavior { return d.Behavior }

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
//  1-2. Deny rules + bypass-immune safety checks + ask rules (via checkHardBlocks)
//       — deny rules cannot be bypassed; safety checks always prompt
//  3. BypassPermissions mode → allow (everything except steps 1-2)
//  4. Session permissions (runtime overrides)
//  5. Allow rules
//  6. Default (safe tools → allow, others → ask)
//  7. Mode transforms: DontAsk converts ask → deny

// HasPermissionToUseTool is the central permission gate that determines
// whether a tool invocation should be allowed, denied, or prompted.
func (s *Settings) HasPermissionToUseTool(toolName string, args map[string]any, session *SessionPermissions) PermissionDecision {
	rule := BuildRule(toolName, args)

	// ── Steps 1-2: Deny rules + bypass-immune safety checks ──
	if reason := s.checkHardBlocks(toolName, args, rule, session); reason != "" {
		if s.isDenyRule(reason) {
			return decide(Deny, reason)
		}
		return decide(Ask, reason)
	}

	// ── Step 3: BypassPermissions mode ──
	if session != nil && session.Mode == ModeBypassPermissions {
		return decide(Allow, "mode: bypass permissions")
	}

	// ── Step 4: Session permissions ──
	if session != nil {
		if session.IsToolAllowed(toolName) {
			return decide(Allow, "session: allow all "+toolName)
		}
		for pattern := range session.AllowedPatterns {
			if MatchRule(rule, pattern) {
				return decide(Allow, "session pattern: "+pattern)
			}
		}
		if toolName == "Bash" {
			if cmd, ok := args["command"].(string); ok {
				for _, subCmd := range extractBashCommands(cmd) {
					subRule := "Bash(" + normalizeBashCommand(subCmd) + ")"
					for pattern := range session.AllowedPatterns {
						if MatchRule(subRule, pattern) {
							return decide(Allow, "session pattern: "+pattern)
						}
					}
				}
			}
		}
	}

	// ── Step 5: Allow rules ──
	for _, pattern := range s.Permissions.Allow {
		if MatchRule(rule, pattern) {
			return decide(Allow, "allow rule: "+pattern)
		}
	}

	// Note: Ask rules are already checked in checkHardBlocks (Steps 1-2).

	// ── Step 7: Default ──
	result := decide(Ask, "default: requires confirmation")
	if IsSafeTool(toolName) {
		result = decide(Allow, "default: safe tool")
	}

	// ── Step 8: Mode transforms ──
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
		if MatchRule(rule, pattern) {
			return "deny rule: " + pattern
		}
	}

	// Bypass-immune: sensitive paths
	if toolName == "Edit" || toolName == "Write" {
		if fp, ok := args["file_path"].(string); ok {
			if reason := IsSensitivePath(fp); reason != "" {
				return "bypass-immune: " + reason
			}
		}
	}

	// Bypass-immune: destructive/dangerous bash
	if toolName == "Bash" {
		if cmd, ok := args["command"].(string); ok {
			if IsDestructiveCommand(cmd) {
				return "bypass-immune: destructive command"
			}
			if reason := CheckBashSecurity(cmd); reason != "" {
				return "bypass-immune: " + reason
			}
		}
	}

	// Working directory constraints
	if session != nil && len(session.WorkingDirectories) > 0 {
		if toolName == "Edit" || toolName == "Write" {
			if fp, ok := args["file_path"].(string); ok {
				if !IsInWorkingDirectory(fp, session.WorkingDirectories) {
					return "outside working directory"
				}
			}
		}
	}

	// Ask rules
	for _, pattern := range s.Permissions.Ask {
		if MatchRule(rule, pattern) {
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

// CheckPermissionWithReason is an alias for HasPermissionToUseTool (backward compat).
func (s *Settings) CheckPermissionWithReason(toolName string, args map[string]any, session *SessionPermissions) PermissionDecision {
	return s.HasPermissionToUseTool(toolName, args, session)
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

// ---------------------------------------------------------------------------
// Bypass-immune path safety checks
// Inspired by Claude Code's checkPathSafetyForAutoEdit — these checks cannot
// be bypassed by session permissions or allow rules.
// ---------------------------------------------------------------------------

// sensitiveDirectories are directory names that should always require
// confirmation when editing files within them. They contain configuration or
// metadata that, if tampered with, can execute code or break tooling.
var sensitiveDirectories = []string{
	".git",    // Git hooks can execute arbitrary code
	".claude", // Claude Code configuration
	".gen",    // GenCode configuration
	".vscode", // VS Code extensions, launch configs
	".idea",   // JetBrains IDE configs
	".ssh",    // SSH keys and config
	".aws",    // AWS credentials
	".gnupg",  // GPG keys
	".kube",   // Kubernetes configs
}

// sensitiveFiles are specific filenames (basenames) that should always require
// confirmation because they can execute code on shell startup or contain
// credentials.
var sensitiveFiles = map[string]string{
	".bashrc":       "shell startup script",
	".bash_profile": "shell startup script",
	".zshrc":        "shell startup script",
	".zprofile":     "shell startup script",
	".profile":      "shell startup script",
	".zshenv":       "shell startup script",
	".login":        "shell startup script",
	".gitconfig":    "git configuration (hooks, aliases)",
	".gitmodules":   "git submodule config",
	".npmrc":        "npm config (may contain auth tokens)",
	".pypirc":       "PyPI config (may contain auth tokens)",
	".netrc":        "network credentials",
	".docker/config.json": "Docker credentials",
}

// IsSensitivePath checks if a file path points to a sensitive location that
// should always require user confirmation (bypass-immune).
// Returns a human-readable reason if sensitive, or empty string if safe.
func IsSensitivePath(filePath string) string {
	// Resolve symlinks to prevent bypass via symlink chains
	resolved, err := filepath.EvalSymlinks(filepath.Dir(filePath))
	if err == nil {
		filePath = filepath.Join(resolved, filepath.Base(filePath))
	}

	// Normalize to absolute path
	if !filepath.IsAbs(filePath) {
		if abs, err := filepath.Abs(filePath); err == nil {
			filePath = abs
		}
	}

	// Check each path component for sensitive directories
	parts := strings.Split(filePath, string(os.PathSeparator))
	for _, part := range parts {
		for _, dir := range sensitiveDirectories {
			if part == dir {
				return dir + "/ directory"
			}
		}
	}

	// Check basename against sensitive files
	basename := filepath.Base(filePath)
	if reason, ok := sensitiveFiles[basename]; ok {
		return basename + " (" + reason + ")"
	}

	// Check two-level paths like ".docker/config.json"
	if len(parts) >= 2 {
		twoLevel := parts[len(parts)-2] + "/" + basename
		if reason, ok := sensitiveFiles[twoLevel]; ok {
			return twoLevel + " (" + reason + ")"
		}
	}

	return ""
}

// ---------------------------------------------------------------------------
// Enhanced bash security checks
// Inspired by Claude Code's bashSecurity.ts — detects obfuscation, injection,
// and other shell security issues beyond simple destructive patterns.
// ---------------------------------------------------------------------------

// zshDangerousCommands are Zsh-specific builtins that can bypass restrictions
// or access system resources directly.
var zshDangerousCommands = []string{
	"zmodload",  // Load kernel modules
	"emulate",   // Change shell emulation mode
	"sysopen",   // Direct file descriptor access
	"sysread",   // Direct system read
	"syswrite",  // Direct system write
	"sysseek",   // Direct seek
	"zpty",      // Pseudo-terminal control
	"ztcp",      // Raw TCP connections
	"zsocket",   // Unix socket access
	"zf_rm",     // Bypass safe rm
	"zf_mv",     // Bypass safe mv
	"zf_ln",     // Bypass safe ln
	"zf_chmod",  // Direct chmod
	"zf_chown",  // Direct chown
}

// bashSecurityPatterns defines patterns that indicate potential shell injection
// or obfuscation attempts.
var bashSecurityPatterns = []struct {
	check   func(string) bool
	reason  string
}{
	{hasCommandSubstitution, "command substitution detected"},
	{hasObfuscatedFlags, "obfuscated flags detected"},
	{hasControlCharacters, "control characters detected"},
	{hasIFSInjection, "IFS injection detected"},
	{hasZshDangerousCommand, "zsh dangerous command"},
	{hasProcEnvironAccess, "/proc/environ access"},
	{hasSuspiciousRedirection, "suspicious redirection"},
}

// CheckBashSecurity performs security analysis on a bash command beyond simple
// destructive pattern matching. Returns a reason string if the command is
// suspicious, or empty string if it appears safe.
func CheckBashSecurity(cmd string) string {
	// AST-based checks first (more accurate, structural analysis)
	if file := ParseBashAST(cmd); file != nil {
		if reason := CheckASTSecurity(file); reason != "" {
			return reason
		}
	}

	// Regex-based checks as fallback / catch-all
	for _, p := range bashSecurityPatterns {
		if p.check(cmd) {
			return p.reason
		}
	}
	return ""
}

func hasCommandSubstitution(cmd string) bool {
	// Detect $() and backtick substitution in dangerous contexts
	// Allow simple $(cmd) but flag nested/complex patterns
	depth := 0
	for i := 0; i < len(cmd)-1; i++ {
		if cmd[i] == '$' && cmd[i+1] == '(' {
			depth++
			if depth > 1 {
				return true // Nested command substitution
			}
		}
		if cmd[i] == ')' && depth > 0 {
			depth--
		}
	}
	// Backtick substitution inside variable assignments
	if strings.Contains(cmd, "eval ") && (strings.Contains(cmd, "$(") || strings.Contains(cmd, "`")) {
		return true
	}
	return false
}

func hasObfuscatedFlags(cmd string) bool {
	// Detect backslash-escaped whitespace between flag characters
	// e.g., "r\m -r\f" to bypass pattern matching
	for i := 0; i < len(cmd)-1; i++ {
		if cmd[i] == '\\' {
			next := cmd[i+1]
			// Backslash followed by a letter mid-word (obfuscation attempt)
			if next >= 'a' && next <= 'z' || next >= 'A' && next <= 'Z' {
				// Check if this is within a flag-like context (after -)
				before := strings.TrimRight(cmd[:i], " \t")
				if len(before) > 0 && before[len(before)-1] == '-' {
					return true
				}
			}
		}
	}
	return false
}

func hasControlCharacters(cmd string) bool {
	for _, r := range cmd {
		// ASCII control chars except common ones (tab, newline, carriage return)
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
		// Unicode zero-width characters used for obfuscation
		if r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFEFF {
			return true
		}
	}
	return false
}

func hasIFSInjection(cmd string) bool {
	return strings.Contains(cmd, "IFS=") || strings.Contains(cmd, "IFS =")
}

func hasZshDangerousCommand(cmd string) bool {
	// Split on all separators: &&, ;, and |
	segments := extractBashCommands(cmd)
	// extractBashCommands handles && and ; but not pipes — add pipe segments
	for _, seg := range strings.Split(cmd, "|") {
		segments = append(segments, strings.TrimSpace(seg))
	}

	for _, c := range segments {
		parts := strings.Fields(c)
		if len(parts) == 0 {
			continue
		}
		if slices.Contains(zshDangerousCommands, filepath.Base(parts[0])) {
			return true
		}
	}
	return false
}

func hasProcEnvironAccess(cmd string) bool {
	return strings.Contains(cmd, "/proc/") && strings.Contains(cmd, "environ")
}

func hasSuspiciousRedirection(cmd string) bool {
	// Detect output redirection to sensitive system paths
	suspiciousPaths := []string{
		"> /etc/", ">> /etc/",
		"> /dev/sd", ">> /dev/sd",
		"> /dev/nvme", ">> /dev/nvme",
		"> ~/.ssh/", ">> ~/.ssh/",
		"> ~/.bashrc", ">> ~/.bashrc",
		"> ~/.zshrc", ">> ~/.zshrc",
		"> ~/.profile", ">> ~/.profile",
	}
	lower := strings.ToLower(cmd)
	for _, p := range suspiciousPaths {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Denial tracking — prevents infinite denial loops and surfaces potential
// classifier or rule misconfiguration.
// ---------------------------------------------------------------------------

// DenialLimits configures when the system falls back to prompting the user
// instead of auto-denying.
var DenialLimits = struct {
	MaxConsecutive int // Fall back to prompting after N consecutive denials
	MaxTotal       int // Fall back to prompting after N total denials in session
}{
	MaxConsecutive: 3,
	MaxTotal:       20,
}

// DenialTracking tracks permission denials during a session.
type DenialTracking struct {
	ConsecutiveDenials int
	TotalDenials       int
}

// RecordDenial records a denial and returns true if the system should fall
// back to prompting the user.
func (d *DenialTracking) RecordDenial() bool {
	d.ConsecutiveDenials++
	d.TotalDenials++
	return d.ShouldFallbackToPrompting()
}

// RecordSuccess resets the consecutive denial counter.
func (d *DenialTracking) RecordSuccess() {
	d.ConsecutiveDenials = 0
}

// ShouldFallbackToPrompting returns true if denial limits are exceeded.
func (d *DenialTracking) ShouldFallbackToPrompting() bool {
	return d.ConsecutiveDenials >= DenialLimits.MaxConsecutive ||
		d.TotalDenials >= DenialLimits.MaxTotal
}
