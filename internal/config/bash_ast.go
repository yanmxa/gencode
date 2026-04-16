package config

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// parsedCommand represents a single command extracted from an AST.
type parsedCommand struct {
	Name       string   // Base command name (path-stripped)
	Args       []string // Command arguments
	HasPipe    bool     // Part of a pipeline
	RedirPaths []string // Output redirection target paths
	InSubshell bool     // Inside $() or backticks
}

// String returns the reconstructed command string.
func (p parsedCommand) String() string {
	if len(p.Args) == 0 {
		return p.Name
	}
	return p.Name + " " + strings.Join(p.Args, " ")
}

// safeWrapperCommands are commands that just wrap execution without changing semantics.
var safeWrapperCommands = map[string]bool{
	"timeout": true,
	"time":    true,
	"nice":    true,
	"nohup":   true,
	"ionice":  true,
}

// parseBashAST parses a bash command string into an AST.
// Returns nil on parse failure (caller should fall back to regex).
func parseBashAST(cmd string) *syntax.File {
	reader := strings.NewReader(cmd)
	parser := syntax.NewParser(syntax.KeepComments(false), syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(reader, "")
	if err != nil {
		return nil
	}
	return file
}

// extractCommandsAST walks the AST and extracts individual simple commands.
// Handles &&, ||, ;, |, subshells, and command substitution.
func extractCommandsAST(file *syntax.File) []parsedCommand {
	var commands []parsedCommand

	for _, stmt := range file.Stmts {
		commands = append(commands, extractFromStmt(stmt, false, false)...)
	}

	return commands
}

func extractFromStmt(stmt *syntax.Stmt, inPipe, inSubshell bool) []parsedCommand {
	var commands []parsedCommand

	// Collect redirections
	var redirPaths []string
	for _, redir := range stmt.Redirs {
		if redir.Op == syntax.RdrOut || redir.Op == syntax.AppOut ||
			redir.Op == syntax.RdrAll || redir.Op == syntax.AppAll {
			if redir.Word != nil {
				path := wordToString(redir.Word)
				if path != "" {
					redirPaths = append(redirPaths, path)
				}
			}
		}
	}

	if stmt.Cmd == nil {
		return commands
	}

	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		parsed := extractFromCall(cmd, inPipe, inSubshell)
		parsed.RedirPaths = append(parsed.RedirPaths, redirPaths...)
		if parsed.Name != "" {
			commands = append(commands, parsed)
		}

	case *syntax.BinaryCmd:
		commands = append(commands, extractFromBinary(cmd, inSubshell)...)

	case *syntax.Subshell:
		for _, s := range cmd.Stmts {
			commands = append(commands, extractFromStmt(s, false, true)...)
		}

	case *syntax.Block:
		for _, s := range cmd.Stmts {
			commands = append(commands, extractFromStmt(s, false, inSubshell)...)
		}

	case *syntax.IfClause:
		commands = append(commands, extractFromIfClause(cmd, inSubshell)...)

	case *syntax.WhileClause:
		for _, s := range cmd.Cond {
			commands = append(commands, extractFromStmt(s, false, inSubshell)...)
		}
		for _, s := range cmd.Do {
			commands = append(commands, extractFromStmt(s, false, inSubshell)...)
		}

	case *syntax.ForClause:
		for _, s := range cmd.Do {
			commands = append(commands, extractFromStmt(s, false, inSubshell)...)
		}

	case *syntax.CaseClause:
		for _, item := range cmd.Items {
			for _, s := range item.Stmts {
				commands = append(commands, extractFromStmt(s, false, inSubshell)...)
			}
		}

	case *syntax.FuncDecl:
		if cmd.Body != nil {
			commands = append(commands, extractFromStmt(cmd.Body, false, inSubshell)...)
		}

	case *syntax.TimeClause:
		if cmd.Stmt != nil {
			commands = append(commands, extractFromStmt(cmd.Stmt, false, inSubshell)...)
		}

	case *syntax.CoprocClause:
		// coproc runs arbitrary commands as a coprocess — must be walked
		if cmd.Stmt != nil {
			commands = append(commands, extractFromStmt(cmd.Stmt, false, inSubshell)...)
		}

	case *syntax.DeclClause:
		// declare, local, export, readonly, typeset, nameref
		name := ""
		if cmd.Variant != nil {
			name = cmd.Variant.Value
		}
		if name != "" {
			commands = append(commands, parsedCommand{
				Name:       name,
				HasPipe:    inPipe,
				InSubshell: inSubshell,
				RedirPaths: redirPaths,
			})
		}

	case *syntax.TestDecl:
		// Bats test declaration — walk the body
		if cmd.Body != nil {
			commands = append(commands, extractFromStmt(cmd.Body, false, inSubshell)...)
		}

		// ArithmCmd, TestClause, LetClause: pure arithmetic/test expressions,
		// no command execution — nothing to extract.
	}

	return commands
}

// extractFromIfClause recursively walks if/elif/else chains, extracting
// commands from both conditions and bodies.
func extractFromIfClause(ic *syntax.IfClause, inSubshell bool) []parsedCommand {
	var commands []parsedCommand
	for ic != nil {
		for _, s := range ic.Cond {
			commands = append(commands, extractFromStmt(s, false, inSubshell)...)
		}
		for _, s := range ic.Then {
			commands = append(commands, extractFromStmt(s, false, inSubshell)...)
		}
		ic = ic.Else
	}
	return commands
}

func extractFromCall(call *syntax.CallExpr, inPipe, inSubshell bool) parsedCommand {
	if len(call.Args) == 0 {
		// Pure assignment (e.g., FOO=bar with no command)
		return parsedCommand{}
	}

	// Collect words (assignments are already separated into call.Assigns by the parser)
	words := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		words = append(words, wordToString(word))
	}

	if len(words) == 0 {
		return parsedCommand{}
	}

	// Strip path prefix from command name
	name := filepath.Base(words[0])

	// Strip safe wrapper commands
	args := words[1:]
	for safeWrapperCommands[name] && len(args) > 0 {
		// Skip wrapper flags and their value arguments
		for len(args) > 0 && !looksLikeCommand(args[0]) {
			args = args[1:]
		}
		// The next command-like arg is the actual command
		if len(args) > 0 {
			name = filepath.Base(args[0])
			args = args[1:]
		} else {
			break
		}
	}

	return parsedCommand{
		Name:       name,
		Args:       args,
		HasPipe:    inPipe,
		InSubshell: inSubshell,
	}
}

func extractFromBinary(bin *syntax.BinaryCmd, inSubshell bool) []parsedCommand {
	var commands []parsedCommand

	isPipe := bin.Op == syntax.Pipe || bin.Op == syntax.PipeAll

	if bin.X != nil {
		commands = append(commands, extractFromStmt(bin.X, isPipe, inSubshell)...)
	}
	if bin.Y != nil {
		commands = append(commands, extractFromStmt(bin.Y, isPipe, inSubshell)...)
	}

	return commands
}

// wordToString converts a syntax.Word to its string representation.
func wordToString(word *syntax.Word) string {
	var sb strings.Builder
	for _, part := range word.Parts {
		partToString(part, &sb)
	}
	return sb.String()
}

func partToString(part syntax.WordPart, sb *strings.Builder) {
	switch p := part.(type) {
	case *syntax.Lit:
		sb.WriteString(p.Value)
	case *syntax.SglQuoted:
		sb.WriteString(p.Value)
	case *syntax.DblQuoted:
		for _, inner := range p.Parts {
			partToString(inner, sb)
		}
	case *syntax.ParamExp:
		sb.WriteString("$")
		if p.Param != nil {
			sb.WriteString(p.Param.Value)
		}
	case *syntax.CmdSubst:
		sb.WriteString("$(...)") // placeholder for command substitution
	default:
		// For other types, use a generic placeholder
		sb.WriteString("...")
	}
}

// sensitiveRedirectPrefixes are path prefixes that should never be targets
// of output redirection. This complements isSensitivePath which checks for
// specific config directories/files.
var sensitiveRedirectPrefixes = []string{
	"/etc/",
	"/dev/sd", "/dev/nvme",
	"/boot/",
	"/usr/lib/", "/usr/bin/",
}

// isSensitiveRedirectTarget checks if a redirect path targets a sensitive
// system location that should not be written to.
func isSensitiveRedirectTarget(path string) bool {
	lower := strings.ToLower(path)
	for _, prefix := range sensitiveRedirectPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// looksLikeCommand returns true if a string looks like a command name
// (not a flag, not a number, not a flag value).
func looksLikeCommand(s string) bool {
	if s == "" {
		return false
	}
	// Flags start with -
	if s[0] == '-' {
		return false
	}
	// Pure numbers are likely duration/priority args (e.g., timeout 30)
	allDigit := true
	for _, c := range s {
		if c < '0' || c > '9' {
			allDigit = false
			break
		}
	}
	if allDigit {
		return false
	}
	// Duration-like patterns (e.g., "30s", "5m", "1h")
	if len(s) >= 2 {
		lastChar := s[len(s)-1]
		if lastChar == 's' || lastChar == 'm' || lastChar == 'h' || lastChar == 'd' {
			rest := s[:len(s)-1]
			allDigitRest := true
			for _, c := range rest {
				if c < '0' || c > '9' {
					allDigitRest = false
					break
				}
			}
			if allDigitRest {
				return false
			}
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// AST-based security checks
// ---------------------------------------------------------------------------

// dangerousBuiltins are shell builtins that can execute arbitrary code
// when used as a command word (not an argument).
var dangerousBuiltins = map[string]bool{
	"eval":   true,
	"source": true,
	".":      true,
}

var readOnlyGitSubcommands = map[string]bool{
	"blame":        true,
	"cat-file":     true,
	"describe":     true,
	"diff":         true,
	"grep":         true,
	"log":          true,
	"ls-files":     true,
	"ls-tree":      true,
	"merge-base":   true,
	"reflog":       true,
	"rev-parse":    true,
	"shortlog":     true,
	"show":         true,
	"show-ref":     true,
	"status":       true,
	"symbolic-ref": true,
}

// checkASTSecurity performs security checks on the parsed AST.
// Returns a reason string if dangerous, empty string if safe.
func checkASTSecurity(file *syntax.File) string {
	commands := extractCommandsAST(file)

	// Check 1: Excessive subcommand count (prevent explosion attacks)
	if len(commands) > 50 {
		return "excessive command count (>50 subcommands)"
	}

	// Check 2: Dangerous builtins in command position
	for _, cmd := range commands {
		if dangerousBuiltins[cmd.Name] {
			return "dangerous builtin: " + cmd.Name
		}
	}

	// Check 3: cd + mutating git compound (bare repo RCE vector)
	cdIntoLiteralPath := false
	for _, cmd := range commands {
		if cmd.Name == "cd" {
			cdIntoLiteralPath = isLiteralCdCommand(cmd)
			continue
		}
		if cmd.Name == "git" && cdIntoLiteralPath && !isReadOnlyGitCommand(cmd) {
			return "cd + git compound command (potential bare repo RCE)"
		}
	}

	// Check 4: Redirect targets to sensitive paths
	for _, cmd := range commands {
		for _, path := range cmd.RedirPaths {
			if reason := isSensitivePath(path); reason != "" {
				return "redirect to sensitive path: " + path
			}
			if isSensitiveRedirectTarget(path) {
				return "redirect to sensitive path: " + path
			}
		}
	}

	// Check 5: Nested command substitution (check AST depth)
	if reason := checkNestedSubstitution(file); reason != "" {
		return reason
	}

	return ""
}

func isLiteralCdCommand(cmd parsedCommand) bool {
	if len(cmd.Args) != 1 {
		return false
	}

	target := strings.TrimSpace(cmd.Args[0])
	if target == "" || strings.HasPrefix(target, "-") {
		return false
	}

	return !strings.ContainsAny(target, "$`;&|<>(){}")
}

func isReadOnlyGitCommand(cmd parsedCommand) bool {
	subcommand, rest := gitSubcommandAndArgs(cmd.Args)
	if readOnlyGitSubcommands[subcommand] {
		return true
	}

	switch subcommand {
	case "tag":
		return isReadOnlyGitTag(rest)
	case "branch":
		return isReadOnlyGitBranch(rest)
	case "remote":
		return isReadOnlyGitRemote(rest)
	default:
		return false
	}
}

func gitSubcommandAndArgs(args []string) (string, []string) {
	idx := gitSubcommandIndex(args)
	if idx == -1 {
		return "", nil
	}
	return args[idx], args[idx+1:]
}

func gitSubcommandIndex(args []string) int {
	for i, arg := range args {
		if arg == "" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return i
	}
	return -1
}

func isReadOnlyGitTag(args []string) bool {
	listMode := len(args) == 0
	for _, arg := range args {
		switch {
		case arg == "" || arg == "-v":
			continue
		case arg == "-l" || arg == "--list":
			listMode = true
		case arg == "-n" || strings.HasPrefix(arg, "-n") || strings.HasPrefix(arg, "--sort=") || strings.HasPrefix(arg, "--contains=") || strings.HasPrefix(arg, "--no-contains=") || strings.HasPrefix(arg, "--merged=") || strings.HasPrefix(arg, "--no-merged=") || strings.HasPrefix(arg, "--points-at=") || arg == "--column" || strings.HasPrefix(arg, "--column="):
			continue
		case strings.HasPrefix(arg, "-"):
			return false
		default:
			if !listMode {
				return false
			}
		}
	}
	return true
}

func isReadOnlyGitBranch(args []string) bool {
	if len(args) == 0 {
		return true
	}

	listMode := false
	for _, arg := range args {
		switch {
		case arg == "" || arg == "-a" || arg == "-r" || arg == "-vv" || arg == "-v" || arg == "--show-current":
			continue
		case arg == "--list" || strings.HasPrefix(arg, "--format=") || strings.HasPrefix(arg, "--sort=") ||
			arg == "--column" || strings.HasPrefix(arg, "--column=") ||
			strings.HasPrefix(arg, "--contains") || strings.HasPrefix(arg, "--no-contains") ||
			strings.HasPrefix(arg, "--merged") || strings.HasPrefix(arg, "--no-merged") ||
			strings.HasPrefix(arg, "--points-at"):
			listMode = true
		case strings.HasPrefix(arg, "-"):
			return false
		default:
			if !listMode {
				return false
			}
		}
	}
	return true
}

func isReadOnlyGitRemote(args []string) bool {
	if len(args) == 0 {
		return true
	}

	switch args[0] {
	case "-v", "show", "get-url":
		return true
	default:
		return false
	}
}

// checkNestedSubstitution walks the AST looking for nested $() patterns.
func checkNestedSubstitution(file *syntax.File) string {
	var found string
	syntax.Walk(file, func(node syntax.Node) bool {
		if found != "" {
			return false
		}
		if cs, ok := node.(*syntax.CmdSubst); ok {
			// Check if this command substitution contains another
			for _, stmt := range cs.Stmts {
				if hasNestedCmdSubst(stmt) {
					found = "nested command substitution detected"
					return false
				}
			}
		}
		return true
	})
	return found
}

func hasNestedCmdSubst(node syntax.Node) bool {
	found := false
	syntax.Walk(node, func(n syntax.Node) bool {
		if found {
			return false
		}
		if _, ok := n.(*syntax.CmdSubst); ok {
			found = true
			return false
		}
		return true
	})
	return found
}
