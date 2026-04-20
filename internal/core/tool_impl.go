package core

import (
	"sort"
	"sync"
)

// toolSet is the default Tools implementation.
//
// Thread-safe map of tools with cached schema list.
type toolSet struct {
	mu    sync.RWMutex
	tools map[string]Tool
	dirty bool         // true when schemas cache needs rebuild
	cache []ToolSchema // cached schemas
}

// NewTools creates an empty tool set.
func NewTools(tools ...Tool) Tools {
	ts := &toolSet{
		tools: make(map[string]Tool, len(tools)),
		dirty: true,
	}
	for _, t := range tools {
		ts.tools[t.Name()] = t
	}
	return ts
}

func (s *toolSet) Get(name string) Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tools[name]
}

func (s *toolSet) All() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

func (s *toolSet) Add(tool Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name()] = tool
	s.dirty = true
}

func (s *toolSet) Remove(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tools[name]; ok {
		delete(s.tools, name)
		s.dirty = true
	}
}

func (s *toolSet) Schemas() []ToolSchema {
	s.mu.RLock()
	if !s.dirty && s.cache != nil {
		out := make([]ToolSchema, len(s.cache))
		copy(out, s.cache)
		s.mu.RUnlock()
		return out
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty && s.cache != nil {
		out := make([]ToolSchema, len(s.cache))
		copy(out, s.cache)
		return out
	}
	schemas := make([]ToolSchema, 0, len(s.tools))
	for _, t := range s.tools {
		schemas = append(schemas, t.Schema())
	}
	sort.Slice(schemas, func(i, j int) bool { return schemas[i].Name < schemas[j].Name })
	s.cache = schemas
	s.dirty = false
	out := make([]ToolSchema, len(schemas))
	copy(out, schemas)
	return out
}

// TODO: Deferred tools — lazy-load rarely used tools (cron, worktree)
//   via ToolSearch; track fetched state, only include in Schemas() when activated.

// TODO: Filtering — support agent allow/disallow lists, disabled tools.
//   Could be a Subset(filter FilterFunc) Tools method or a wrapper.

// TODO: MCP tools — integrate external MCP server tools into the same Tools interface.
//   MCP tools implement Tool with Execute routing to MCP transport.

// TODO: Static override — support a fixed schema list that bypasses dynamic resolution
//   (used when parent agent restricts child tool set).
