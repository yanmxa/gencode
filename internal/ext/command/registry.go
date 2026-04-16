// Package command provides slash-command metadata, parsing, and matching logic.
// Handler dispatch remains in the tui package since handlers reference the tui model.
package command

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/yanmxa/gencode/internal/util/markdown"
	"github.com/yanmxa/gencode/internal/plugin"

	"gopkg.in/yaml.v3"
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
		{Name: "model", Description: "Select model and manage provider connections"},
		{Name: "clear", Description: "Clear chat history"},
		{Name: "fork", Description: "Fork current conversation into a new session"},
		{Name: "resume", Description: "Resume a previous session (opens session selector)"},
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
		{Name: "plugin", Description: "Manage plugins (list/install/marketplace/enable/disable/info)"},
		{Name: "reload-plugins", Description: "Reload plugins and refresh plugin-backed skills, agents, MCP, and hooks"},
		{Name: "think", Description: "Toggle thinking level (off/think/think+/ultrathink)"},
		{Name: "loop", Description: "Schedule recurring or one-shot prompts and manage loop jobs"},
		{Name: "search", Description: "Select search engine for web search"},
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

// GetMatchingCommands returns all commands (builtin + skills + plugin commands)
// whose names fuzzy-match the given query. Results are sorted alphabetically.
func GetMatchingCommands(query string) []Info {
	query = strings.ToLower(strings.TrimPrefix(query, "/"))
	matches := make([]Info, 0)
	seen := make(map[string]bool)

	builtins := BuiltinNames()
	for name, cmd := range builtins {
		if fuzzyMatch(name, query) {
			matches = append(matches, cmd)
			seen[name] = true
		}
	}

	for _, provider := range getDynamicInfoProviders() {
		for _, cmd := range provider() {
			if fuzzyMatch(strings.ToLower(cmd.Name), query) && !seen[cmd.Name] {
				matches = append(matches, cmd)
				seen[cmd.Name] = true
			}
		}
	}

	customCmds := GetCustomCommands()
	for _, cmd := range customCmds {
		if fuzzyMatch(strings.ToLower(cmd.Name), query) {
			if !seen[cmd.Name] {
				matches = append(matches, cmd)
				seen[cmd.Name] = true
			}
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Name < matches[j].Name
	})

	return matches
}

// commandScope represents where a custom command was loaded from.
// Higher values have higher priority.
type commandScope int

const (
	scopeUser         commandScope = iota // ~/.gen/commands/
	scopeUserPlugin                        // ~/.gen/plugins/*/commands/
	scopeProjectPlugin                     // .gen/plugins/*/commands/
	scopeProject                          // .gen/commands/
)

// CustomCommand represents a user-defined slash command from
// ~/.gen/commands/, .gen/commands/, or a plugin's commands/ directory.
// Unlike active skills, custom commands are never injected into the system
// prompt — they only execute when the user explicitly invokes /name.
type CustomCommand struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Namespace   string `yaml:"namespace"`
	FilePath    string
	Scope       commandScope
}

// FullName returns the namespaced command name (namespace:name or just name).
func (cc *CustomCommand) FullName() string {
	if cc.Namespace != "" {
		return cc.Namespace + ":" + cc.Name
	}
	return cc.Name
}

// GetInstructions reads the markdown body (excluding frontmatter) from disk.
func (cc *CustomCommand) GetInstructions() string {
	if cc.FilePath == "" {
		return ""
	}
	_, body, _ := markdown.ParseFrontmatterFile(cc.FilePath)
	return body
}

// commandCwd stores the working directory for resolving project-level commands.
var (
	commandMu            sync.RWMutex
	commandCwd           string
	cachedCustomCommands []CustomCommand
	dynamicInfoProviders []func() []Info
)

// Initialize sets the working directory for resolving project-level commands
// and invalidates the cached command list.
// Sources: ~/.gen/commands/, .gen/commands/, and plugin command paths.
func Initialize(cwd string) error {
	commandMu.Lock()
	defer commandMu.Unlock()
	commandCwd = cwd
	cachedCustomCommands = nil
	return nil
}

// SetDynamicInfoProviders configures additional command metadata sources that
// are composed above this package, such as skill-backed slash commands.
func SetDynamicInfoProviders(providers ...func() []Info) {
	commandMu.Lock()
	defer commandMu.Unlock()
	dynamicInfoProviders = append([]func() []Info(nil), providers...)
}

func getDynamicInfoProviders() []func() []Info {
	commandMu.RLock()
	defer commandMu.RUnlock()
	return append([]func() []Info(nil), dynamicInfoProviders...)
}

// GetCustomCommands returns Info entries for all custom commands
// (user, plugin, and project level).
func GetCustomCommands() []Info {
	cmds := loadAllCustomCommands()
	infos := make([]Info, 0, len(cmds))
	for _, c := range cmds {
		infos = append(infos, Info{
			Name:        c.FullName(),
			Description: c.Description,
		})
	}
	return infos
}

// IsCustomCommand checks whether the given command name matches a custom command.
func IsCustomCommand(cmd string) (*CustomCommand, bool) {
	for _, c := range loadAllCustomCommands() {
		if c.FullName() == cmd || c.Name == cmd {
			return &c, true
		}
	}
	return nil, false
}

// loadAllCustomCommands returns custom commands from all sources, using cache
// when available. The cache is invalidated by Initialize.
func loadAllCustomCommands() []CustomCommand {
	commandMu.RLock()
	if cachedCustomCommands != nil {
		defer commandMu.RUnlock()
		return cachedCustomCommands
	}
	commandMu.RUnlock()

	commandMu.Lock()
	defer commandMu.Unlock()
	if cachedCustomCommands != nil {
		return cachedCustomCommands
	}
	cachedCustomCommands = loadCustomCommandsFromDisk()
	return cachedCustomCommands
}

// loadCustomCommandsFromDisk loads custom commands from all sources in priority order:
// 1. ~/.gen/commands/        (user level, lowest priority)
// 2. ~/.gen/plugins/*/commands/ (user-plugin)
// 3. .gen/plugins/*/commands/   (project-plugin)
// 4. .gen/commands/          (project level, highest priority)
// Higher-priority commands override lower-priority ones with the same full name.
func loadCustomCommandsFromDisk() []CustomCommand {
	cmdMap := make(map[string]CustomCommand)

	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		userDir := filepath.Join(homeDir, ".gen", "commands")
		for _, pc := range loadCommandsFromDir(userDir, "", scopeUser) {
			cmdMap[pc.FullName()] = pc
		}
	}

	if plugin.DefaultRegistry != nil {
		paths := plugin.GetPluginCommandPaths()
		for _, pp := range paths {
			pc := loadCustomCommandFile(pp.Path, pp.Namespace)
			if pc != nil {
				pc.Scope = pluginScopeTocommandScope(pp.Scope)
				cmdMap[pc.FullName()] = *pc
			}
		}
	}

	if commandCwd != "" {
		projectDir := filepath.Join(commandCwd, ".gen", "commands")
		for _, pc := range loadCommandsFromDir(projectDir, "", scopeProject) {
			cmdMap[pc.FullName()] = pc
		}
	}

	cmds := make([]CustomCommand, 0, len(cmdMap))
	for _, c := range cmdMap {
		cmds = append(cmds, c)
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].FullName() < cmds[j].FullName()
	})
	return cmds
}

// pluginScopeTocommandScope maps plugin.Scope to commandScope.
func pluginScopeTocommandScope(s plugin.Scope) commandScope {
	switch s {
	case plugin.ScopeProject, plugin.ScopeLocal:
		return scopeProjectPlugin
	default:
		return scopeUserPlugin
	}
}

// loadCommandsFromDir scans a directory for markdown command files.
func loadCommandsFromDir(dir, defaultNamespace string, scope commandScope) []CustomCommand {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var cmds []CustomCommand
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		pc := loadCustomCommandFile(filepath.Join(dir, entry.Name()), defaultNamespace)
		if pc != nil {
			pc.Scope = scope
			cmds = append(cmds, *pc)
		}
	}
	return cmds
}

// loadCustomCommandFile loads a single custom command from a markdown file.
func loadCustomCommandFile(path, defaultNamespace string) *CustomCommand {
	fm, _, _ := markdown.ParseFrontmatterFile(path)
	if fm == "" {
		return defaultCustomCommand(path, defaultNamespace)
	}
	var cc CustomCommand
	if err := yaml.Unmarshal([]byte(fm), &cc); err != nil {
		return defaultCustomCommand(path, defaultNamespace)
	}
	cc.FilePath = path
	if cc.Name == "" {
		cc.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	if cc.Namespace == "" && defaultNamespace != "" {
		cc.Namespace = defaultNamespace
	}
	return &cc
}

// defaultCustomCommand creates a CustomCommand with defaults derived from the filename.
func defaultCustomCommand(path, defaultNamespace string) *CustomCommand {
	return &CustomCommand{
		Name:      strings.TrimSuffix(filepath.Base(path), ".md"),
		Namespace: defaultNamespace,
		FilePath:  path,
	}
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
