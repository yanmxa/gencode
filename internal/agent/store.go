package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// AgentStoreData is the JSON structure for agents.json
type AgentStoreData struct {
	Disabled []string `json:"disabled"`
}

// AgentStore handles persistence of agent enabled/disabled states
type AgentStore struct {
	mu       sync.RWMutex
	path     string
	disabled map[string]bool
}

// NewAgentStore creates a new store at the given path
func NewAgentStore(path string) *AgentStore {
	store := &AgentStore{
		path:     path,
		disabled: make(map[string]bool),
	}
	store.load()
	return store
}

// NewUserAgentStore creates a store for user-level (~/.gen/agents.json)
func NewUserAgentStore() *AgentStore {
	home, err := os.UserHomeDir()
	if err != nil {
		return &AgentStore{disabled: make(map[string]bool)}
	}
	return NewAgentStore(filepath.Join(home, ".gen", "agents.json"))
}

// NewProjectAgentStore creates a store for project-level (.gen/agents.json)
func NewProjectAgentStore(cwd string) *AgentStore {
	return NewAgentStore(filepath.Join(cwd, ".gen", "agents.json"))
}

// load reads disabled agents from disk
func (s *AgentStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}

	var storeData AgentStoreData
	if err := json.Unmarshal(data, &storeData); err != nil {
		return
	}

	s.disabled = make(map[string]bool)
	for _, name := range storeData.Disabled {
		s.disabled[name] = true
	}
}

// save writes disabled agents to disk
func (s *AgentStore) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build disabled list
	disabled := make([]string, 0, len(s.disabled))
	for name := range s.disabled {
		disabled = append(disabled, name)
	}

	storeData := AgentStoreData{Disabled: disabled}
	data, err := json.MarshalIndent(storeData, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0644)
}

// IsDisabled returns whether an agent is disabled
func (s *AgentStore) IsDisabled(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.disabled[name]
}

// SetDisabled sets the disabled state for an agent
func (s *AgentStore) SetDisabled(name string, disabled bool) error {
	s.mu.Lock()
	if disabled {
		s.disabled[name] = true
	} else {
		delete(s.disabled, name)
	}
	s.mu.Unlock()

	return s.save()
}

// GetDisabled returns a copy of the disabled agents map
func (s *AgentStore) GetDisabled() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]bool, len(s.disabled))
	for k, v := range s.disabled {
		result[k] = v
	}
	return result
}
