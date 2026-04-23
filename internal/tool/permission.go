package tool

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

// WithPermission wraps core.Tools with permission checking.
// Safe tools (perm.IsSafeTool) bypass the check automatically.
// nil check returns inner unchanged (permit-all).
func WithPermission(inner core.Tools, check perm.PermissionFunc) core.Tools {
	if check == nil {
		return inner
	}
	return &permissionTools{inner: inner, check: check}
}

// permissionTools wraps a core.Tools and injects permission checking on Get().
type permissionTools struct {
	inner core.Tools
	check perm.PermissionFunc
}

func (pt *permissionTools) Get(name string) core.Tool {
	t := pt.inner.Get(name)
	if t == nil {
		return nil
	}
	return &permissionTool{inner: t, check: pt.check}
}

func (pt *permissionTools) All() []core.Tool           { return pt.inner.All() }
func (pt *permissionTools) Add(tool core.Tool)         { pt.inner.Add(tool) }
func (pt *permissionTools) Remove(name string)         { pt.inner.Remove(name) }
func (pt *permissionTools) Schemas() []core.ToolSchema { return pt.inner.Schemas() }

// permissionTool wraps a single core.Tool with permission checking.
type permissionTool struct {
	inner core.Tool
	check perm.PermissionFunc
}

func (pt *permissionTool) Name() string            { return pt.inner.Name() }
func (pt *permissionTool) Description() string     { return pt.inner.Description() }
func (pt *permissionTool) Schema() core.ToolSchema { return pt.inner.Schema() }

func (pt *permissionTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	if !perm.IsSafeTool(pt.inner.Name()) {
		if allow, reason := pt.check(ctx, pt.inner.Name(), input); !allow {
			return "", fmt.Errorf("blocked: %s", reason)
		}
	}
	return pt.inner.Execute(ctx, input)
}
