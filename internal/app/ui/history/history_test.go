package history

import "testing"

func TestEscapeUnescapeRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple", "hello world"},
		{"newline", "line1\nline2"},
		{"backslash", `a\b`},
		{"backslash-n literal", `a\nb`},
		{"double backslash", `a\\b`},
		{"backslash before newline", "a\\\nb"},
		{"multiple newlines", "a\nb\nc"},
		{"trailing backslash", `end\`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			escaped := escapeEntry(tt.input)
			got := unescapeEntry(escaped)
			if got != tt.input {
				t.Errorf("roundtrip failed:\n  input:   %q\n  escaped: %q\n  got:     %q", tt.input, escaped, got)
			}
		})
	}
}
