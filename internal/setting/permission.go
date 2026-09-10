package setting

import (
	permissionpolicy "github.com/genai-io/san/internal/permission"
	"github.com/genai-io/san/internal/tool/perm"
)

// PermissionDecision is kept as an alias for callers that consume decisions
// through Settings. The policy implementation belongs to internal/permission.
type PermissionDecision = permissionpolicy.PermissionDecision

func (s *Data) permissionPolicy() permissionpolicy.Policy {
	if s == nil {
		return permissionpolicy.Policy{}
	}
	return permissionpolicy.Policy{Permissions: s.Permissions}
}

func (s *Data) HasPermissionToUseTool(toolName string, args map[string]any, session *SessionPermissions) PermissionDecision {
	policy := s.permissionPolicy()
	return policy.HasPermissionToUseTool(toolName, args, session)
}

func (s *Data) ResolveHookAllow(toolName string, args map[string]any, session *SessionPermissions) bool {
	policy := s.permissionPolicy()
	return policy.ResolveHookAllow(toolName, args, session)
}

func (s *Data) CheckPermission(toolName string, args map[string]any, session *SessionPermissions) perm.Decision {
	policy := s.permissionPolicy()
	return policy.CheckPermission(toolName, args, session)
}
