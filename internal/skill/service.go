package skill

import "sync"

// Service is the public contract for the skill module.
type Service interface {
	// query
	List() []*Skill                                          // all loaded skills
	Get(name string) (*Skill, bool)                          // lookup by name
	IsEnabled(name string) bool                              // check if enabled
	FindByPartialName(name string) *Skill                    // partial/suffix match
	GetEnabled() []*Skill                                    // all enabled or active skills
	GetActive() []*Skill                                     // all active skills (model-aware)
	Count() int                                              // total number of loaded skills

	// mutation
	SetEnabled(name string, enabled bool, userLevel bool) error
	GetDisabledAt(userLevel bool) map[string]bool

	// system prompt
	PromptSection() string                                   // rendered section for system prompt
	GetSkillInvocationPrompt(name string) string             // full skill content for injection

	// plugin
	AddPluginSkills(paths []struct {
		Path      string
		Namespace string
		IsProject bool
	})
}

// Compile-time check: *Registry implements Service.
var _ Service = (*Registry)(nil)

// ── singleton ──────────────────────────────────────────────

var (
	mu       sync.RWMutex
	instance Service
)

// Default returns the singleton Service instance.
// Panics if Initialize has not been called.
func Default() Service {
	mu.RLock()
	s := instance
	mu.RUnlock()
	if s == nil {
		panic("skill: not initialized")
	}
	return s
}

// DefaultIfInit returns the singleton Service instance, or nil if not yet
// initialized. Useful for nil-guards that used to check DefaultRegistry == nil.
func DefaultIfInit() Service {
	mu.RLock()
	s := instance
	mu.RUnlock()
	return s
}

// SetDefault replaces the singleton instance. Intended for tests.
func SetDefault(s Service) {
	mu.Lock()
	instance = s
	mu.Unlock()
}

// Reset clears the singleton instance. Intended for tests.
func Reset() {
	mu.Lock()
	instance = nil
	mu.Unlock()
}
