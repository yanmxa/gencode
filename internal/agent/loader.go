package agent

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yanmxa/gencode/internal/log"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// LoadCustomAgents loads custom agent definitions from standard locations.
// Search order (priority):
//  1. .gen/agents/*.md (project level, preferred)
//  2. ~/.gen/agents/*.md (user level, preferred)
//  3. .claude/agents/*.md (project level, Claude Code compatible)
//  4. ~/.claude/agents/*.md (user level, Claude Code compatible)
func LoadCustomAgents(cwd string) {
	homeDir, _ := os.UserHomeDir()

	// Define search paths in order of priority
	searchPaths := []string{
		filepath.Join(cwd, ".gen", "agents"),
		filepath.Join(homeDir, ".gen", "agents"),
		filepath.Join(cwd, ".claude", "agents"),
		filepath.Join(homeDir, ".claude", "agents"),
	}

	for _, path := range searchPaths {
		loadAgentsFromDir(path)
	}
}

// loadAgentsFromDir loads all AGENT.md or *.md files from a directory
func loadAgentsFromDir(dir string) {
	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}

	entries, err := os.ReadDir(dir)
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

		filePath := filepath.Join(dir, name)
		loadAgentFromFile(filePath)
	}
}

// loadAgentFromFile loads an agent configuration from a markdown file
func loadAgentFromFile(filePath string) {
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

	// Use the body as the system prompt
	if body != "" {
		config.SystemPrompt = strings.TrimSpace(body)
	}

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

// AgentFrontmatter represents the YAML frontmatter structure in AGENT.md files
// This is used for parsing custom agents
type AgentFrontmatter struct {
	Name           string   `yaml:"name"`
	Description    string   `yaml:"description"`
	Model          string   `yaml:"model"`
	PermissionMode string   `yaml:"permission-mode"`
	MaxTurns       int      `yaml:"max-turns"`
	Background     bool     `yaml:"background"`
	Skills         []string `yaml:"skills"`
	Tools          struct {
		Mode  string   `yaml:"mode"`
		Allow []string `yaml:"allow"`
		Deny  []string `yaml:"deny"`
	} `yaml:"tools"`
}

// toAgentConfig converts frontmatter to AgentConfig
func (f *AgentFrontmatter) toAgentConfig() *AgentConfig {
	config := &AgentConfig{
		Name:        f.Name,
		Description: f.Description,
		Model:       f.Model,
		MaxTurns:    f.MaxTurns,
		Background:  f.Background,
		Skills:      f.Skills,
	}

	// Parse permission mode
	switch f.PermissionMode {
	case "plan":
		config.PermissionMode = PermissionPlan
	case "acceptEdits":
		config.PermissionMode = PermissionAcceptEdits
	case "dontAsk":
		config.PermissionMode = PermissionDontAsk
	default:
		config.PermissionMode = PermissionDefault
	}

	// Parse tools
	switch f.Tools.Mode {
	case "allowlist":
		config.Tools = ToolAccess{
			Mode:  ToolAccessAllowlist,
			Allow: f.Tools.Allow,
		}
	case "denylist":
		config.Tools = ToolAccess{
			Mode: ToolAccessDenylist,
			Deny: f.Tools.Deny,
		}
	}

	return config
}

// Init is called to initialize the agent system
// This should be called during application startup
// It loads custom agents and initializes the enabled/disabled state stores
func Init(cwd string) {
	LoadCustomAgents(cwd)

	// Initialize stores for enabled/disabled state persistence
	if err := DefaultRegistry.InitStores(cwd); err != nil {
		log.Logger().Warn("Failed to initialize agent stores", zap.Error(err))
	}
}

// validateAgentName checks if an agent name is valid
func validateAgentName(name string) bool {
	// Agent names should be alphanumeric with hyphens/underscores
	pattern := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)
	return pattern.MatchString(name)
}
