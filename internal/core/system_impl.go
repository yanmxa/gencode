package core

import (
	"sort"
	"strings"
	"sync"
)

// system is the default System implementation.
//
// Layers are stored by name and assembled by priority order on Prompt().
// The result is cached until a layer changes.
type system struct {
	mu     sync.RWMutex
	layers map[string]Layer
	cached string
	dirty  bool
}

// NewSystem creates an empty System.
func NewSystem(layers ...Layer) System {
	s := &system{
		layers: make(map[string]Layer, len(layers)),
		dirty:  true,
	}
	for _, l := range layers {
		s.layers[l.Name] = l
	}
	return s
}

func (s *system) Prompt() string {
	s.mu.RLock()
	if !s.dirty {
		defer s.mu.RUnlock()
		return s.cached
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return s.cached
	}
	s.cached = s.build()
	s.dirty = false
	return s.cached
}

func (s *system) Set(layer Layer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.layers[layer.Name] = layer
	s.dirty = true
}

func (s *system) Get(name string) (Layer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.layers[name]
	return l, ok
}

func (s *system) Remove(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.layers[name]; ok {
		delete(s.layers, name)
		s.dirty = true
	}
}

func (s *system) Layers() []Layer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sorted()
}

func (s *system) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty = true
}

// sorted returns layers ordered by priority (ascending).
func (s *system) sorted() []Layer {
	ls := make([]Layer, 0, len(s.layers))
	for _, l := range s.layers {
		ls = append(ls, l)
	}
	sort.Slice(ls, func(i, j int) bool {
		return ls[i].Priority < ls[j].Priority
	})
	return ls
}

// build assembles all layers into a single prompt string.
func (s *system) build() string {
	ls := s.sorted()
	parts := make([]string, 0, len(ls))
	for _, l := range ls {
		if l.Content != "" {
			parts = append(parts, l.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}
