package plugin

import "testing"

func TestScopeConfigSource(t *testing.T) {
	tests := []struct {
		scope Scope
		want  string
	}{
		{ScopeUser, "user_settings"},
		{ScopeProject, "project_settings"},
		{ScopeLocal, "local_settings"},
	}
	for _, tt := range tests {
		if got := scopeConfigSource(tt.scope); got != tt.want {
			t.Fatalf("scopeConfigSource(%q) = %q, want %q", tt.scope, got, tt.want)
		}
	}
}
