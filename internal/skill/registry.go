package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// DefaultRegistry is the global skill registry instance.
var DefaultRegistry *Registry

// Registry manages loaded skills and their states.
type Registry struct {
	mu           sync.RWMutex
	skills       map[string]*Skill
	userStore    *Store // User-level store (~/.gen/skills.json)
	projectStore *Store // Project-level store (.gen/skills.json)
	cwd          string // Current working directory for project store
}

// Store handles persistence of skill states to a skills.json file.
type Store struct {
	path   string
	states map[string]SkillState
}

// StoreData is the JSON structure for skills.json.
type StoreData struct {
	Skills map[string]SkillState `json:"skills"`
}

// NewStore creates a new store for skill state persistence at the given path.
func NewStore(path string) (*Store, error) {
	store := &Store{
		path:   path,
		states: make(map[string]SkillState),
	}

	// Load existing states
	store.load()

	return store, nil
}

// NewUserStore creates a store for user-level settings (~/.gen/skills.json).
func NewUserStore() (*Store, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return NewStore(filepath.Join(homeDir, ".gen", "skills.json"))
}

// NewProjectStore creates a store for project-level settings (.gen/skills.json).
func NewProjectStore(cwd string) (*Store, error) {
	return NewStore(filepath.Join(cwd, ".gen", "skills.json"))
}

// load reads persisted states from disk.
func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return // File doesn't exist or can't be read
	}

	var storeData StoreData
	if err := json.Unmarshal(data, &storeData); err != nil {
		return
	}

	if storeData.Skills != nil {
		s.states = storeData.Skills
	}
}

// save writes states to disk.
func (s *Store) save() error {
	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	storeData := StoreData{
		Skills: s.states,
	}

	data, err := json.MarshalIndent(storeData, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0644)
}

// GetState returns the persisted state for a skill.
func (s *Store) GetState(name string) (SkillState, bool) {
	state, ok := s.states[name]
	return state, ok
}

// SetState sets and persists the state for a skill.
func (s *Store) SetState(name string, state SkillState) error {
	s.states[name] = state
	return s.save()
}

// Initialize loads all skills and applies persisted states.
// This should be called at application startup.
func Initialize(cwd string) error {
	loader := NewLoader(cwd)

	skills, err := loader.LoadAll()
	if err != nil {
		return err
	}

	userStore, err := NewUserStore()
	if err != nil {
		return err
	}

	projectStore, err := NewProjectStore(cwd)
	if err != nil {
		return err
	}

	registry := &Registry{
		skills:       skills,
		userStore:    userStore,
		projectStore: projectStore,
		cwd:          cwd,
	}

	// Apply persisted states (project overrides user)
	for _, skill := range skills {
		fullName := skill.FullName()
		// First apply user-level state
		if state, ok := userStore.GetState(fullName); ok {
			skill.State = state
		}
		// Then apply project-level state (higher priority)
		if state, ok := projectStore.GetState(fullName); ok {
			skill.State = state
		}
	}

	DefaultRegistry = registry
	return nil
}

// Get returns a skill by name.
func (r *Registry) Get(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	skill, ok := r.skills[name]
	return skill, ok
}

// FindByPartialName finds a skill by partial name match.
// It tries exact match first, then checks if name is a suffix (e.g., "commit" matches "git:commit").
func (r *Registry) FindByPartialName(name string) *Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Exact match first
	if skill, ok := r.skills[name]; ok {
		return skill
	}

	// Try suffix match (e.g., "commit" -> "git:commit")
	name = strings.ToLower(name)
	for fullName, skill := range r.skills {
		// Check if name matches the part after ":"
		if idx := strings.LastIndex(fullName, ":"); idx >= 0 {
			shortName := strings.ToLower(fullName[idx+1:])
			if shortName == name {
				return skill
			}
		}
		// Also try lowercase full match
		if strings.ToLower(fullName) == name {
			return skill
		}
	}

	return nil
}

// List returns all skills sorted by full name (namespace:name).
func (r *Registry) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]*Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		skills = append(skills, skill)
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].FullName() < skills[j].FullName()
	})

	return skills
}

// GetEnabled returns all enabled or active skills.
func (r *Registry) GetEnabled() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]*Skill, 0)
	for _, skill := range r.skills {
		if skill.IsEnabled() {
			skills = append(skills, skill)
		}
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].FullName() < skills[j].FullName()
	})

	return skills
}

// GetActive returns all active skills (model-aware).
func (r *Registry) GetActive() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]*Skill, 0)
	for _, skill := range r.skills {
		if skill.IsActive() {
			skills = append(skills, skill)
		}
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].FullName() < skills[j].FullName()
	})

	return skills
}

// SetState sets the state for a skill and persists it to the specified level.
// The name should be the full name (namespace:name or just name).
// If userLevel is true, saves to ~/.gen/skills.json, otherwise to .gen/skills.json.
func (r *Registry) SetState(name string, state SkillState, userLevel bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	skill, ok := r.skills[name]
	if !ok {
		return fmt.Errorf("skill not found: %s", name)
	}

	skill.State = state

	// Persist to the appropriate store
	if userLevel {
		return r.userStore.SetState(skill.FullName(), state)
	}
	return r.projectStore.SetState(skill.FullName(), state)
}

// GetStatesAt returns skill states from the specified level.
func (r *Registry) GetStatesAt(userLevel bool) map[string]SkillState {
	if userLevel {
		return r.userStore.states
	}
	return r.projectStore.states
}

// GetAvailableSkillsPrompt generates the available skills section for the system prompt.
// Only includes active skills (state = active).
// Uses progressive loading: only name + description are included here.
// Full instructions are loaded when the Skill tool is invoked.
// Returns content wrapped in <available-skills> XML tags for consistency.
func (r *Registry) GetAvailableSkillsPrompt() string {
	active := r.GetActive()
	if len(active) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<available-skills>\n")
	sb.WriteString("Use the Skill tool to invoke these capabilities:\n\n")

	for _, skill := range active {
		// Only include name and description (progressive loading)
		sb.WriteString(fmt.Sprintf("- %s: %s", skill.FullName(), skill.Description))
		if skill.ArgumentHint != "" {
			sb.WriteString(fmt.Sprintf(" %s", skill.ArgumentHint))
		}
		// Indicate if skill has resources
		if skill.HasResources() {
			resources := []string{}
			if len(skill.Scripts) > 0 {
				resources = append(resources, fmt.Sprintf("%d scripts", len(skill.Scripts)))
			}
			if len(skill.References) > 0 {
				resources = append(resources, fmt.Sprintf("%d refs", len(skill.References)))
			}
			sb.WriteString(fmt.Sprintf(" [%s]", strings.Join(resources, ", ")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\nInvoke with: Skill(skill=\"name\", args=\"optional args\")\n")
	sb.WriteString("</available-skills>")
	return sb.String()
}

// GetSkillInvocationPrompt returns the full skill content wrapped in XML for injection.
// The name should be the full name (namespace:name or just name).
func (r *Registry) GetSkillInvocationPrompt(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, ok := r.skills[name]
	if !ok {
		return ""
	}

	instructions := skill.GetInstructions()
	if instructions == "" {
		return ""
	}

	var sb strings.Builder
	// Use FullName in the XML tag
	fmt.Fprintf(&sb, "<skill-invocation name=\"%s\">\n", skill.FullName())
	sb.WriteString(instructions)
	sb.WriteString("\n</skill-invocation>")

	return sb.String()
}

// Count returns the total number of loaded skills.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.skills)
}
