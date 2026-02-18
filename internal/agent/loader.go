package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/plugin"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// agentSearchPath represents an agent search location with optional namespace.
type agentSearchPath struct {
	path      string
	namespace string // Default namespace for agents in this path (from plugin)
}

// additionalAgentPaths stores plugin agent paths.
var additionalAgentPaths []agentSearchPath

// AddPluginAgentPath adds a plugin agent path to be searched.
func AddPluginAgentPath(path, namespace string) {
	additionalAgentPaths = append(additionalAgentPaths, agentSearchPath{
		path:      path,
		namespace: namespace,
	})
}

// ClearPluginAgentPaths clears all plugin agent paths.
func ClearPluginAgentPaths() {
	additionalAgentPaths = nil
}

// LoadCustomAgents loads custom agent definitions from standard locations.
// Note: .claude/plugins/ loading is removed - plugins are handled by the plugin system.
// Search order (priority):
//  1. .gen/agents/*.md (project level, preferred)
//  2. ~/.gen/agents/*.md (user level, preferred)
//  3. .claude/agents/*.md (project level, Claude Code compatible)
//  4. ~/.claude/agents/*.md (user level, Claude Code compatible)
//  5. Plugin agent paths
func LoadCustomAgents(cwd string) {
	homeDir, _ := os.UserHomeDir()

	// Define search paths in order of priority
	searchPaths := []agentSearchPath{
		{path: filepath.Join(cwd, ".gen", "agents")},
		{path: filepath.Join(homeDir, ".gen", "agents")},
		{path: filepath.Join(cwd, ".claude", "agents")},
		{path: filepath.Join(homeDir, ".claude", "agents")},
	}

	// Add plugin paths
	searchPaths = append(searchPaths, additionalAgentPaths...)

	for _, sp := range searchPaths {
		loadAgentsFromDirWithNamespace(sp.path, sp.namespace)
	}
}

// loadAgentsFromDirWithNamespace loads agents with an optional namespace prefix.
// The path can be either a directory (scanned for .md files) or a direct file path.
func loadAgentsFromDirWithNamespace(path string, namespace string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	// If path is a file, load it directly
	if !info.IsDir() {
		if strings.HasSuffix(path, ".md") {
			loadAgentFromFileWithNamespace(path, namespace)
		}
		return
	}

	// Path is a directory, scan for .md files
	entries, err := os.ReadDir(path)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}

		filePath := filepath.Join(path, name)
		loadAgentFromFileWithNamespace(filePath, namespace)
	}
}

// loadAgentFromFileWithNamespace loads an agent with optional namespace.
func loadAgentFromFileWithNamespace(filePath string, namespace string) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		log.Logger().Debug("Failed to read agent file",
			zap.String("path", filePath),
			zap.Error(err))
		return
	}

	config, err := parseAgentFile(string(content), filePath)
	if err != nil {
		log.Logger().Debug("Failed to parse agent file",
			zap.String("path", filePath),
			zap.Error(err))
		return
	}

	if config != nil {
		// Apply namespace if provided (from plugin)
		if namespace != "" && !strings.Contains(config.Name, ":") {
			config.Name = namespace + ":" + config.Name
		}

		// Register with the default registry
		DefaultRegistry.Register(config)
		log.Logger().Info("Loaded custom agent",
			zap.String("name", config.Name),
			zap.String("source", filePath))
	}
}

// parseAgentFile parses an AGENT.md file with YAML frontmatter
func parseAgentFile(content, filePath string) (*AgentConfig, error) {
	// Extract YAML frontmatter
	frontmatter, body := extractFrontmatter(content)
	if frontmatter == "" {
		return nil, nil // No frontmatter, skip
	}

	// Parse YAML frontmatter
	var config AgentConfig
	if err := yaml.Unmarshal([]byte(frontmatter), &config); err != nil {
		return nil, err
	}

	// Set defaults
	if config.Name == "" {
		// Derive name from filename
		base := filepath.Base(filePath)
		config.Name = strings.TrimSuffix(base, ".md")
	}

	if config.Model == "" {
		config.Model = "inherit"
	}

	if config.MaxTurns <= 0 {
		config.MaxTurns = DefaultMaxTurns
	}

	if config.PermissionMode == "" {
		config.PermissionMode = PermissionDefault
	}

	// Don't load the full system prompt at startup - it will be lazily loaded
	// when the agent is actually executed. Only store the SourceFile path.
	// The body content is intentionally not loaded here for progressive loading.
	_ = body // Body will be loaded lazily via GetSystemPrompt()

	config.SourceFile = filePath

	return &config, nil
}

// extractFrontmatter extracts YAML frontmatter from markdown content
// Frontmatter is content between --- markers at the start of the file
func extractFrontmatter(content string) (frontmatter, body string) {
	content = strings.TrimSpace(content)

	// Check for YAML frontmatter delimiters
	if !strings.HasPrefix(content, "---") {
		return "", content
	}

	// Find the ending delimiter
	rest := content[3:] // Skip initial ---
	endIndex := strings.Index(rest, "\n---")
	if endIndex == -1 {
		return "", content
	}

	frontmatter = strings.TrimSpace(rest[:endIndex])
	body = strings.TrimSpace(rest[endIndex+4:]) // Skip \n---

	return frontmatter, body
}

// LoadAgentSystemPrompt loads just the system prompt (body) from an agent file.
// This is used for lazy loading - the full prompt is only loaded when the agent is executed.
func LoadAgentSystemPrompt(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		log.Logger().Debug("Failed to read agent file for system prompt",
			zap.String("path", filePath),
			zap.Error(err))
		return ""
	}

	_, body := extractFrontmatter(string(content))
	return strings.TrimSpace(body)
}

// Init is called to initialize the agent system
// This should be called during application startup
// It loads custom agents and initializes the enabled/disabled state stores
func Init(cwd string) {
	// Clear previous plugin agent paths
	ClearPluginAgentPaths()

	// Add agent paths from enabled plugins
	for _, pp := range plugin.GetPluginAgentPaths() {
		AddPluginAgentPath(pp.Path, pp.Namespace)
	}

	LoadCustomAgents(cwd)

	// Initialize stores for enabled/disabled state persistence
	if err := DefaultRegistry.InitStores(cwd); err != nil {
		log.Logger().Warn("Failed to initialize agent stores", zap.Error(err))
	}
}

