package permission

import (
	"testing"

	"github.com/yanmxa/gencode/internal/config"
)

func TestPermission_DontAskMode_DeniesAllPrompts(t *testing.T) {
	checker := DontAsk()

	// Read-only tools should be permitted
	readTools := []string{"Read", "Glob", "Grep", "WebFetch", "WebSearch"}
	for _, name := range readTools {
		d := checker.Check(name, nil)
		if d != Permit {
			t.Errorf("DontAsk: expected Permit for read-only tool %q, got %v", name, d)
		}
	}

	// Safe tools should be permitted
	safeTools := []string{"AskUserQuestion", "EnterPlanMode", "ExitPlanMode", "TaskCreate", "TaskList"}
	for _, name := range safeTools {
		d := checker.Check(name, nil)
		if d != Permit {
			t.Errorf("DontAsk: expected Permit for safe tool %q, got %v", name, d)
		}
	}

	// Write/Edit/Bash should be rejected (no prompting)
	writeTools := []string{"Write", "Edit", "Bash"}
	for _, name := range writeTools {
		d := checker.Check(name, nil)
		if d != Reject {
			t.Errorf("DontAsk: expected Reject (no prompt) for tool %q, got %v", name, d)
		}
	}
}

func TestPermission_GlobPattern_MatchesCorrectly(t *testing.T) {
	settings := &config.Settings{
		Permissions: config.PermissionSettings{
			Deny: []string{
				"Read(**/.env)",
				"Read(*.env)",
			},
		},
	}
	session := config.NewSessionPermissions()

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
			isDenied := decision.Behavior == config.Deny
			if isDenied != tt.wantDeny {
				t.Errorf("HasPermissionToUseTool(%q, %v): got Deny=%v, want Deny=%v (reason: %s)",
					tt.toolName, tt.args, isDenied, tt.wantDeny, decision.Reason)
			}
		})
	}
}

func TestIsReadOnlyToolMatchesConfig(t *testing.T) {
	tools := []string{
		"Read",
		"Glob",
		"Grep",
		"WebFetch",
		"WebSearch",
		"LSP",
		"Bash",
		"Write",
	}

	for _, name := range tools {
		if got, want := IsReadOnlyTool(name), config.IsReadOnlyTool(name); got != want {
			t.Fatalf("IsReadOnlyTool(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsSafeToolMatchesConfig(t *testing.T) {
	tools := []string{
		"TaskCreate",
		"TaskGet",
		"TaskList",
		"TaskUpdate",
		"AskUserQuestion",
		"EnterPlanMode",
		"ExitPlanMode",
		"ToolSearch",
		"LSP",
		"Edit",
	}

	for _, name := range tools {
		if got, want := IsSafeTool(name), config.IsSafeTool(name); got != want {
			t.Fatalf("IsSafeTool(%q) = %v, want %v", name, got, want)
		}
	}
}
