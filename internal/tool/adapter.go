package tool

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/core"
)

// AdaptTool wraps a legacy Tool as a core.Tool with a dynamic CWD resolver.
func AdaptTool(t Tool, schema core.ToolSchema, cwd func() string) core.Tool {
	return &toolAdapter{inner: t, schema: schema, cwd: cwd}
}

// AdaptToolRegistry wraps all tools from the global registry as core.Tools.
// The schema list maps tool names to their JSON schemas.
func AdaptToolRegistry(schemas []core.ToolSchema, cwd func() string) core.Tools {
	schemaByName := make(map[string]core.ToolSchema, len(schemas))
	for _, s := range schemas {
		if s.Name != "" {
			schemaByName[s.Name] = s
		}
	}

	var adapted []core.Tool
	for name, schema := range schemaByName {
		if t, ok := Get(name); ok {
			adapted = append(adapted, AdaptTool(t, schema, cwd))
		}
	}
	return core.NewTools(adapted...)
}

// toolAdapter wraps a legacy Tool as a core.Tool.
type toolAdapter struct {
	inner  Tool
	schema core.ToolSchema
	cwd    func() string
}

func (a *toolAdapter) Name() string            { return a.inner.Name() }
func (a *toolAdapter) Description() string     { return a.inner.Description() }
func (a *toolAdapter) Schema() core.ToolSchema { return a.schema }

func (a *toolAdapter) Execute(ctx context.Context, input map[string]any) (string, error) {
	cwd := ""
	if a.cwd != nil {
		cwd = a.cwd()
	}

	result := a.inner.Execute(ctx, input, cwd)
	text := result.FormatForLLM()
	if !result.Success {
		return text, fmt.Errorf("%s", text)
	}
	return text, nil
}
