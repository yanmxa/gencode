package config

import (
	"testing"
)

func TestMatchRule(t *testing.T) {
	tests := []struct {
		name    string
		rule    string
		pattern string
		want    bool
	}{
		// Exact matches
		{"exact match", "Bash(npm)", "Bash(npm)", true},
		{"exact mismatch", "Bash(npm)", "Bash(yarn)", false},

		// Wildcard patterns
		{"wildcard suffix", "Bash(npm:install)", "Bash(npm:*)", true},
		{"wildcard prefix", "Bash(npm:install)", "Bash(*:install)", true},
		{"wildcard middle", "Bash(npm:install:lodash)", "Bash(npm:*:lodash)", true},
		{"wildcard no match", "Bash(yarn:install)", "Bash(npm:*)", false},

		// Double wildcard
		{"double wildcard", "Read(/path/to/.env)", "Read(**/.env)", true},
		{"double wildcard suffix", "Read(/a/b/c/file.go)", "Read(**/*.go)", true},
		{"double wildcard prefix", "Read(/home/user/file.txt)", "Read(/home/**)", true},

		// File path patterns
		{"file path exact", "Edit(/path/to/file.go)", "Edit(/path/to/file.go)", true},
		{"file path wildcard", "Edit(/path/to/file.go)", "Edit(/path/to/*.go)", true},

		// Tool name mismatch
		{"tool mismatch", "Read(/path/file)", "Edit(/path/file)", false},

		// WebFetch domain patterns
		{"domain match", "WebFetch(domain:github.com)", "WebFetch(domain:github.com)", true},
		{"domain mismatch", "WebFetch(domain:gitlab.com)", "WebFetch(domain:github.com)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchRule(tt.rule, tt.pattern)
			if got != tt.want {
				t.Errorf("MatchRule(%q, %q) = %v, want %v", tt.rule, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestBuildRule(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		args     map[string]any
		want     string
	}{
		{
			"bash command",
			"Bash",
			map[string]any{"command": "npm install lodash"},
			"Bash(npm:install lodash)",
		},
		{
			"bash git command",
			"Bash",
			map[string]any{"command": "git status"},
			"Bash(git:status)",
		},
		{
			"read file",
			"Read",
			map[string]any{"file_path": "/path/to/file.txt"},
			"Read(/path/to/file.txt)",
		},
		{
			"edit file",
			"Edit",
			map[string]any{"file_path": "/path/to/file.go", "old_string": "foo", "new_string": "bar"},
			"Edit(/path/to/file.go)",
		},
		{
			"glob pattern",
			"Glob",
			map[string]any{"pattern": "**/*.go"},
			"Glob(**/*.go)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildRule(tt.toolName, tt.args)
			if got != tt.want {
				t.Errorf("BuildRule(%q, %v) = %q, want %q", tt.toolName, tt.args, got, tt.want)
			}
		})
	}
}

func TestCheckPermission(t *testing.T) {
	settings := &Settings{
		Permissions: PermissionSettings{
			Allow: []string{
				"Bash(git:*)",
				"Bash(npm:*)",
				"Read(**/*.go)",
			},
			Deny: []string{
				"Read(**/.env)",
				"Read(**/.env.*)",
			},
			Ask: []string{
				"Bash(rm:*)",
			},
		},
	}

	tests := []struct {
		name     string
		toolName string
		args     map[string]any
		session  *SessionPermissions
		want     PermissionResult
	}{
		// Allow rules
		{
			"git command allowed",
			"Bash",
			map[string]any{"command": "git status"},
			nil,
			PermissionAllow,
		},
		{
			"npm command allowed",
			"Bash",
			map[string]any{"command": "npm install"},
			nil,
			PermissionAllow,
		},
		{
			"read go file allowed",
			"Read",
			map[string]any{"file_path": "/path/to/file.go"},
			nil,
			PermissionAllow,
		},

		// Deny rules
		{
			"read .env denied",
			"Read",
			map[string]any{"file_path": "/path/to/.env"},
			nil,
			PermissionDeny,
		},
		{
			"read .env.local denied",
			"Read",
			map[string]any{"file_path": "/path/to/.env.local"},
			nil,
			PermissionDeny,
		},

		// Ask rules
		{
			"rm command needs ask",
			"Bash",
			map[string]any{"command": "rm -rf /tmp/test"},
			nil,
			PermissionAsk,
		},

		// Default behavior - read-only allowed
		{
			"glob default allowed",
			"Glob",
			map[string]any{"pattern": "*.txt"},
			nil,
			PermissionAllow,
		},

		// Default behavior - write needs ask
		{
			"edit default needs ask",
			"Edit",
			map[string]any{"file_path": "/path/to/file.txt"},
			nil,
			PermissionAsk,
		},

		// Session permissions
		{
			"session allow all edits",
			"Edit",
			map[string]any{"file_path": "/path/to/file.txt"},
			&SessionPermissions{AllowAllEdits: true},
			PermissionAllow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := settings.CheckPermission(tt.toolName, tt.args, tt.session)
			if got != tt.want {
				t.Errorf("CheckPermission(%q, %v) = %v, want %v", tt.toolName, tt.args, got, tt.want)
			}
		})
	}
}

func TestLoaderLoad(t *testing.T) {
	loader := NewLoader()
	settings, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if settings == nil {
		t.Fatal("Load() returned nil settings")
	}
	// Just verify it loads without error - actual values depend on environment
}
