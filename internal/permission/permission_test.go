package permission

import (
	"testing"

	"github.com/yanmxa/gencode/internal/setting"
)

func TestPermission_GlobPattern_MatchesCorrectly(t *testing.T) {
	settings := &setting.Settings{
		Permissions: setting.PermissionSettings{
			Deny: []string{
				"Read(**/.env)",
				"Read(*.env)",
			},
		},
	}
	session := setting.NewSessionPermissions()

	tests := []struct {
		name      string
		toolName  string
		args      map[string]any
		wantDeny  bool
	}{
		{
			name:     "** matches nested .env",
			toolName: "Read",
			args:     map[string]any{"file_path": "/home/user/project/.env"},
			wantDeny: true,
		},
		{
			name:     "** matches deeply nested .env",
			toolName: "Read",
			args:     map[string]any{"file_path": "/a/b/c/d/.env"},
			wantDeny: true,
		},
		{
			name:     "*.env does not match .env (no leading chars)",
			toolName: "Read",
			args:     map[string]any{"file_path": "/project/config.env"},
			wantDeny: true, // matches *.env pattern
		},
		{
			name:     "safe file is allowed",
			toolName: "Read",
			args:     map[string]any{"file_path": "/home/user/project/main.go"},
			wantDeny: false,
		},
		{
			name:     "Bash is not denied by Read pattern",
			toolName: "Bash",
			args:     map[string]any{"command": "cat .env"},
			wantDeny: false, // deny rules are tool-specific
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := settings.HasPermissionToUseTool(tt.toolName, tt.args, session)
			isDenied := decision.Behavior == setting.Deny
			if isDenied != tt.wantDeny {
				t.Errorf("HasPermissionToUseTool(%q, %v): got Deny=%v, want Deny=%v (reason: %s)",
					tt.toolName, tt.args, isDenied, tt.wantDeny, decision.Reason)
			}
		})
	}
}

func TestIsReadOnlyTool(t *testing.T) {
	readOnly := []string{"Read", "Glob", "Grep", "WebFetch", "WebSearch", "LSP"}
	for _, name := range readOnly {
		if !IsReadOnlyTool(name) {
			t.Errorf("IsReadOnlyTool(%q) = false, want true", name)
		}
	}

	notReadOnly := []string{"Bash", "Write", "Edit", "Agent"}
	for _, name := range notReadOnly {
		if IsReadOnlyTool(name) {
			t.Errorf("IsReadOnlyTool(%q) = true, want false", name)
		}
	}
}

func TestIsSafeTool(t *testing.T) {
	safe := []string{"TaskCreate", "TaskGet", "TaskList", "TaskUpdate",
		"AskUserQuestion", "EnterPlanMode", "ExitPlanMode", "ToolSearch", "LSP"}
	for _, name := range safe {
		if !IsSafeTool(name) {
			t.Errorf("IsSafeTool(%q) = false, want true", name)
		}
	}

	notSafe := []string{"Edit", "Bash", "Write", "Agent"}
	for _, name := range notSafe {
		if IsSafeTool(name) {
			t.Errorf("IsSafeTool(%q) = true, want false", name)
		}
	}
}
