package tool

import (
	"context"
	"strings"
	"sync"

	"github.com/myan/gencode/internal/tool/ui"
)

// Registry manages tool registration and execution
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates a new tool registry
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[strings.ToLower(tool.Name())] = tool
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[strings.ToLower(name)]
	return tool, ok
}

// List returns all registered tool names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Execute runs a tool by name with the given parameters
func (r *Registry) Execute(ctx context.Context, name string, params map[string]any, cwd string) ui.ToolResult {
	tool, ok := r.Get(name)
	if !ok {
		return ui.NewErrorResult(name, "unknown tool: "+name)
	}
	return tool.Execute(ctx, params, cwd)
}

// DefaultRegistry is the global default tool registry
var DefaultRegistry = NewRegistry()

// Register adds a tool to the default registry
func Register(tool Tool) {
	DefaultRegistry.Register(tool)
}

// Get retrieves a tool from the default registry
func Get(name string) (Tool, bool) {
	return DefaultRegistry.Get(name)
}

// Execute runs a tool from the default registry
func Execute(ctx context.Context, name string, params map[string]any, cwd string) ui.ToolResult {
	return DefaultRegistry.Execute(ctx, name, params, cwd)
}
