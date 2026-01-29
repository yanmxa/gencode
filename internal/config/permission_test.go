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

func TestIsDestructiveCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		// Destructive commands
		{"rm -rf", "rm -rf /tmp/test", true},
		{"rm -fr", "rm -fr /tmp/test", true},
		{"rm -r", "rm -r /tmp/test", true},
		{"git reset --hard", "git reset --hard HEAD", true},
		{"git clean -fd", "git clean -fd", true},
		{"git push --force", "git push --force origin main", true},
		{"git push -f", "git push -f", true},
		{"chmod 777", "chmod 777 /tmp/file", true},

		// Path-qualified commands (should normalize to base command)
		{"rm with full path", "/bin/rm -rf /tmp/test", true},
		{"git with full path", "/usr/bin/git reset --hard HEAD", true},
		{"rm with relative path", "./rm -rf /tmp", true},

		// Safe commands
		{"rm single file", "rm /tmp/file.txt", false},
		{"git status", "git status", false},
		{"git push", "git push origin main", false},
		{"git commit", "git commit -m 'msg'", false},
		{"chmod 644", "chmod 644 /tmp/file", false},
		{"ls", "ls -la", false},
		{"npm install", "npm install", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDestructiveCommand(tt.command)
			if got != tt.want {
				t.Errorf("IsDestructiveCommand(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

func TestDenyRulesPriorityOverSession(t *testing.T) {
	settings := &Settings{
		Permissions: PermissionSettings{
			Deny: []string{
				"Read(**/.env)",
				"Bash(rm:-rf *)",
			},
		},
	}

	// Test that deny rules take priority over session permissions
	session := &SessionPermissions{
		AllowAllBash: true,
		AllowedTools: map[string]bool{"Read": true},
	}

	tests := []struct {
		name     string
		toolName string
		args     map[string]any
		want     PermissionResult
	}{
		{
			"deny rule blocks even with session allow",
			"Read",
			map[string]any{"file_path": "/path/to/.env"},
			PermissionDeny,
		},
		{
			"normal bash allowed with session",
			"Bash",
			map[string]any{"command": "ls -la"},
			PermissionAllow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := settings.CheckPermission(tt.toolName, tt.args, session)
			if got != tt.want {
				t.Errorf("CheckPermission(%q, %v) = %v, want %v", tt.toolName, tt.args, got, tt.want)
			}
		})
	}
}

func TestDestructiveCommandsRequireConfirmation(t *testing.T) {
	settings := &Settings{
		Permissions: PermissionSettings{},
	}

	// Even with AllowAllBash, destructive commands should require confirmation
	session := &SessionPermissions{
		AllowAllBash: true,
	}

	tests := []struct {
		name    string
		command string
		want    PermissionResult
	}{
		{"rm -rf requires ask", "rm -rf /tmp/test", PermissionAsk},
		{"git reset --hard requires ask", "git reset --hard HEAD", PermissionAsk},
		{"git push --force requires ask", "git push --force", PermissionAsk},
		{"normal git allowed", "git status", PermissionAllow},
		{"normal ls allowed", "ls -la", PermissionAllow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{"command": tt.command}
			got := settings.CheckPermission("Bash", args, session)
			if got != tt.want {
				t.Errorf("CheckPermission(Bash, %q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}
