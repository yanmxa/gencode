package config

import (
	"testing"
)

func TestParseBashAST(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantNil bool
	}{
		{"simple command", "ls -la", false},
		{"chained", "cd /tmp && git pull", false},
		{"pipe", "cat file | grep pattern", false},
		{"subshell", "echo $(whoami)", false},
		{"heredoc", "cat <<EOF\nhello\nEOF", false},
		// Malformed input should return nil (graceful fallback)
		{"unterminated quote", "echo 'hello", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseBashAST(tt.cmd)
			if (got == nil) != tt.wantNil {
				t.Errorf("ParseBashAST(%q) nil=%v, wantNil=%v", tt.cmd, got == nil, tt.wantNil)
			}
		})
	}
}

func TestExtractCommandsAST(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantCmds []string // expected command names
	}{
		{"simple", "ls -la", []string{"ls"}},
		{"chained and", "cd /tmp && git pull", []string{"cd", "git"}},
		{"chained semi", "echo hello; echo world", []string{"echo", "echo"}},
		{"chained or", "test -f file || echo missing", []string{"test", "echo"}},
		{"pipe", "cat file | grep pattern | wc -l", []string{"cat", "grep", "wc"}},
		{"env var wrapper", "NODE_ENV=prod npm start", []string{"npm"}},
		{"timeout wrapper", "timeout 30 make test", []string{"make"}},
		{"nice wrapper", "nice -n 10 gcc main.c", []string{"gcc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := ParseBashAST(tt.cmd)
			if file == nil {
				t.Fatalf("ParseBashAST(%q) returned nil", tt.cmd)
			}
			commands := ExtractCommandsAST(file)
			if len(commands) != len(tt.wantCmds) {
				names := make([]string, len(commands))
				for i, c := range commands {
					names[i] = c.Name
				}
				t.Fatalf("got %d commands %v, want %d %v", len(commands), names, len(tt.wantCmds), tt.wantCmds)
			}
			for i, want := range tt.wantCmds {
				if commands[i].Name != want {
					t.Errorf("command[%d].Name = %q, want %q", i, commands[i].Name, want)
				}
			}
		})
	}
}

func TestExtractCommandsAST_Pipe(t *testing.T) {
	file := ParseBashAST("cat file | grep pattern")
	if file == nil {
		t.Fatal("parse failed")
	}
	commands := ExtractCommandsAST(file)
	for _, cmd := range commands {
		if !cmd.HasPipe {
			t.Errorf("command %q should have HasPipe=true", cmd.Name)
		}
	}
}

func TestExtractCommandsAST_Redirect(t *testing.T) {
	file := ParseBashAST("echo data > /tmp/output.txt")
	if file == nil {
		t.Fatal("parse failed")
	}
	commands := ExtractCommandsAST(file)
	if len(commands) != 1 {
		t.Fatalf("got %d commands, want 1", len(commands))
	}
	if len(commands[0].RedirPaths) != 1 || commands[0].RedirPaths[0] != "/tmp/output.txt" {
		t.Errorf("RedirPaths = %v, want [/tmp/output.txt]", commands[0].RedirPaths)
	}
}

func TestExtractCommandsAST_PathStripping(t *testing.T) {
	file := ParseBashAST("/usr/bin/git status")
	if file == nil {
		t.Fatal("parse failed")
	}
	commands := ExtractCommandsAST(file)
	if len(commands) != 1 || commands[0].Name != "git" {
		t.Errorf("expected 'git', got %q", commands[0].Name)
	}
}

func TestCheckASTSecurity(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		// Safe commands
		{"simple ls", "ls -la", true},
		{"git status", "git status", true},
		{"npm install", "npm install lodash", true},
		{"pipe", "cat file | grep pattern", true},
		{"chained safe", "cd /tmp && ls", true},

		// Dangerous: builtins
		{"eval", "eval 'rm -rf /'", false},
		{"source", "source ~/.bashrc", false},
		{"dot source", ". /tmp/evil.sh", false},

		// Dangerous: cd + git
		{"cd git compound", "cd /tmp/repo && git pull", false},
		{"cd git separated", "cd /tmp; git clone url", false},

		// Dangerous: redirect to sensitive path
		{"redirect to etc", "echo bad > /etc/passwd", false},
		{"redirect to ssh", "echo key >> ~/.ssh/authorized_keys", false},

		// Dangerous: nested command substitution
		{"nested subst", "echo $(echo $(whoami))", false},

		// Safe: single-level command substitution
		{"single subst", "echo $(date)", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := ParseBashAST(tt.cmd)
			if file == nil {
				t.Skipf("ParseBashAST(%q) returned nil", tt.cmd)
			}
			reason := CheckASTSecurity(file)
			isSafe := reason == ""
			if isSafe != tt.wantSafe {
				t.Errorf("CheckASTSecurity(%q) = %q, wantSafe=%v", tt.cmd, reason, tt.wantSafe)
			}
		})
	}
}

func TestCheckASTSecurity_ExcessiveCommands(t *testing.T) {
	// Build a command with 51 subcommands
	parts := make([]string, 51)
	for i := range parts {
		parts[i] = "echo hi"
	}
	cmd := ""
	for i, p := range parts {
		if i > 0 {
			cmd += " && "
		}
		cmd += p
	}

	file := ParseBashAST(cmd)
	if file == nil {
		t.Fatal("parse failed")
	}
	reason := CheckASTSecurity(file)
	if reason == "" {
		t.Error("expected excessive command count to be flagged")
	}
}
