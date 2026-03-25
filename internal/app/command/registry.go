// Package command provides slash-command metadata, parsing, and matching logic.
// Handler dispatch remains in the tui package since handlers reference the tui model.
package command

import (
	"sort"
	"strings"

	"github.com/yanmxa/gencode/internal/skill"
)

// Info holds the metadata for a slash command (name, description, visibility).
type Info struct {
	Name        string
	Description string
	Hidden      bool
}

// builtinCommands returns the static set of built-in command metadata.
// This is the single source of truth for command names and descriptions.
func builtinCommands() []Info {
	return []Info{
		{Name: "provider", Description: "List and connect to LLM providers"},
		{Name: "model", Description: "List and select models"},
		{Name: "clear", Description: "Clear chat history"},
		{Name: "fork", Description: "Fork current conversation into a new session"},
		{Name: "help", Description: "Show available commands"},
		{Name: "glob", Description: "Find files matching a pattern"},
		{Name: "tools", Description: "Manage available tools (enable/disable)"},
		{Name: "plan", Description: "Enter plan mode to explore and plan before execution"},
		{Name: "skills", Description: "Manage skills (enable/disable/activate)"},
		{Name: "agents", Description: "Manage available agents (enable/disable)"},
		{Name: "tokenlimit", Description: "View or set token limits for current model"},
		{Name: "compact", Description: "Summarize conversation to reduce context size"},
		{Name: "init", Description: "Initialize memory files (GEN.md, local, rules)"},
		{Name: "memory", Description: "View and manage memory files (list/show/edit) with @import support"},
		{Name: "mcp", Description: "Manage MCP servers (add/edit/remove/connect/list)"},
		{Name: "plugin", Description: "Manage plugins (list/enable/disable/info)"},
		{Name: "think", Description: "Toggle thinking level (off/think/think+/ultrathink)"},
	}
}

// BuiltinNames returns the set of built-in command names for registry lookup.
func BuiltinNames() map[string]Info {
	cmds := builtinCommands()
	m := make(map[string]Info, len(cmds))
	for _, c := range cmds {
		m[c.Name] = c
	}
	return m
}

// ParseCommand splits a slash-command input into the command name, arguments,
// and a boolean indicating whether the input was a command at all.
func ParseCommand(input string) (cmd string, args string, isCmd bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}

	input = strings.TrimPrefix(input, "/")
	parts := strings.SplitN(input, " ", 2)
	cmd = strings.ToLower(parts[0])
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	return cmd, args, true
}

// GetMatchingCommands returns all commands (builtin + skills) whose names
// fuzzy-match the given query. Results are sorted alphabetically by name.
func GetMatchingCommands(query string) []Info {
	query = strings.ToLower(strings.TrimPrefix(query, "/"))
	matches := make([]Info, 0)

	builtins := BuiltinNames()
	for name, cmd := range builtins {
		if fuzzyMatch(name, query) {
			matches = append(matches, cmd)
		}
	}

	skillCmds := GetSkillCommands()
	for _, cmd := range skillCmds {
		if fuzzyMatch(strings.ToLower(cmd.Name), query) {
			if _, exists := builtins[cmd.Name]; !exists {
				matches = append(matches, cmd)
			}
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Name < matches[j].Name
	})

	return matches
}

// IsSkillCommand checks whether the given command name matches an enabled skill.
func IsSkillCommand(cmd string) (*skill.Skill, bool) {
	if skill.DefaultRegistry == nil {
		return nil, false
	}

	s, ok := skill.DefaultRegistry.Get(cmd)
	if !ok {
		return nil, false
	}

	if !s.IsEnabled() {
		return nil, false
	}

	return s, true
}

// GetSkillCommands returns Info entries for all enabled skills.
func GetSkillCommands() []Info {
	if skill.DefaultRegistry == nil {
		return nil
	}

	var cmds []Info
	for _, s := range skill.DefaultRegistry.GetEnabled() {
		hint := ""
		if s.ArgumentHint != "" {
			hint = " " + s.ArgumentHint
		}
		cmds = append(cmds, Info{
			Name:        s.FullName(),
			Description: s.Description + hint,
		})
	}
	return cmds
}

// fuzzyMatch returns true if every character in pattern appears in str in order.
func fuzzyMatch(str, pattern string) bool {
	pi := 0
	for si := 0; si < len(str) && pi < len(pattern); si++ {
		if str[si] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}
