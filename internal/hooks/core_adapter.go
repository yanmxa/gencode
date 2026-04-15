package hooks

import (
	"context"

	"github.com/yanmxa/gencode/internal/core"
)

// coreHooksAdapter wraps an Engine as a core.Hooks implementation.
type coreHooksAdapter struct {
	engine *Engine
}

// AsCoreHooks wraps a hooks.Engine so it satisfies the core.Hooks interface.
func AsCoreHooks(engine *Engine) core.Hooks {
	if engine == nil {
		return nil
	}
	return &coreHooksAdapter{engine: engine}
}

func (a *coreHooksAdapter) Register(hook core.Hook) string {
	return ""
}

func (a *coreHooksAdapter) Unregister(id string) bool {
	return false
}

func (a *coreHooksAdapter) Fire(ctx context.Context, event core.Event) (core.Action, error) {
	return core.Action{}, nil
}

func (a *coreHooksAdapter) Has(event core.EventType) bool {
	return false
}

func (a *coreHooksAdapter) Drain() {
	// Engine manages its own async lifecycle; nothing to drain here.
}
