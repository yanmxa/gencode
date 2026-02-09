package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// PlanStatus represents the status of a plan
type PlanStatus string

const (
	StatusDraft    PlanStatus = "draft"
	StatusApproved PlanStatus = "approved"
)

// Plan represents a saved implementation plan
type Plan struct {
	ID        string     `yaml:"id"`
	CreatedAt time.Time  `yaml:"created_at"`
	Task      string     `yaml:"task"`
	Status    PlanStatus `yaml:"status"`
	Content   string     `yaml:"-"` // markdown body (not in frontmatter)
}

// Store manages plan file storage
type Store struct {
	baseDir string
}

// NewStore creates a new plan store
// Plans are stored in ~/.gen/plans/
func NewStore() (*Store, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".gen", "plans")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create plans directory: %w", err)
	}

	return &Store{baseDir: baseDir}, nil
}

// Save saves a plan to disk and returns the file path
func (s *Store) Save(plan *Plan) (string, error) {
	if plan.ID == "" {
		plan.ID = GeneratePlanName(plan.Task)
	}
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = time.Now()
	}
	if plan.Status == "" {
		plan.Status = StatusDraft
	}

	filePath := filepath.Join(s.baseDir, plan.ID+".md")

	// Build file content with YAML frontmatter
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", plan.ID))
	sb.WriteString(fmt.Sprintf("created_at: %s\n", plan.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("task: %s\n", escapeYAML(plan.Task)))
	sb.WriteString(fmt.Sprintf("status: %s\n", plan.Status))
	sb.WriteString("---\n\n")
	sb.WriteString(plan.Content)

	if err := os.WriteFile(filePath, []byte(sb.String()), 0644); err != nil {
		return "", fmt.Errorf("failed to write plan file: %w", err)
	}

	return filePath, nil
}

// Load loads a plan from disk by ID
func (s *Store) Load(id string) (*Plan, error) {
	filePath := filepath.Join(s.baseDir, id+".md")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read plan file: %w", err)
	}

	return parsePlanFile(string(data))
}

// List returns all saved plans
func (s *Store) List() ([]*Plan, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read plans directory: %w", err)
	}

	plans := make([]*Plan, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".md")
		plan, err := s.Load(id)
		if err != nil {
			continue // Skip invalid plan files
		}
		plans = append(plans, plan)
	}

	return plans, nil
}

// Delete removes a plan file
func (s *Store) Delete(id string) error {
	filePath := filepath.Join(s.baseDir, id+".md")
	return os.Remove(filePath)
}

// GetPath returns the full path for a plan ID
func (s *Store) GetPath(id string) string {
	return filepath.Join(s.baseDir, id+".md")
}

// parsePlanFile parses a plan file with YAML frontmatter
func parsePlanFile(content string) (*Plan, error) {
	// Check for frontmatter
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("plan file missing frontmatter")
	}

	// Find the end of frontmatter
	endIdx := strings.Index(content[4:], "\n---\n")
	if endIdx == -1 {
		return nil, fmt.Errorf("plan file has unclosed frontmatter")
	}

	frontmatter := content[4 : 4+endIdx]
	body := strings.TrimPrefix(content[4+endIdx+5:], "\n")

	plan := &Plan{
		Content: body,
	}

	// Parse frontmatter fields manually (simple YAML parsing)
	lines := strings.Split(frontmatter, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "id":
			plan.ID = value
		case "created_at":
			if t, err := time.Parse(time.RFC3339, value); err == nil {
				plan.CreatedAt = t
			}
		case "task":
			plan.Task = unescapeYAML(value)
		case "status":
			plan.Status = PlanStatus(value)
		}
	}

	return plan, nil
}

// escapeYAML escapes a string for safe YAML output
func escapeYAML(s string) string {
	needsQuoting := strings.ContainsAny(s, ":\n\"'") ||
		strings.HasPrefix(s, " ") ||
		strings.HasSuffix(s, " ")

	if !needsQuoting {
		return s
	}

	escaped := s
	escaped = strings.ReplaceAll(escaped, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	return "\"" + escaped + "\""
}

// unescapeYAML unescapes a YAML string value
func unescapeYAML(s string) string {
	if len(s) < 2 {
		return s
	}

	hasQuotes := (s[0] == '"' && s[len(s)-1] == '"') ||
		(s[0] == '\'' && s[len(s)-1] == '\'')

	if !hasQuotes {
		return s
	}

	s = s[1 : len(s)-1]
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// ValidatePlanID checks if a plan ID is valid
func ValidatePlanID(id string) bool {
	// Must match pattern: YYYYMMDD-keyword-keyword... or keyword-keyword-keyword
	pattern := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)+$`)
	return pattern.MatchString(id)
}
