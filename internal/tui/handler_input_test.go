package tui

import (
	"testing"
)

func TestImageRefPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected [][]string
	}{
		{
			input:    "describe @image.png",
			expected: [][]string{{"@image.png", "image.png", "png"}},
		},
		{
			input:    "@photo.jpg analyze this",
			expected: [][]string{{"@photo.jpg", "photo.jpg", "jpg"}},
		},
		{
			input:    "compare @a.png with @b.jpeg",
			expected: [][]string{{"@a.png", "a.png", "png"}, {"@b.jpeg", "b.jpeg", "jpeg"}},
		},
		{
			input:    "no images here",
			expected: nil,
		},
		{
			input:    "@path/to/image.webp",
			expected: [][]string{{"@path/to/image.webp", "path/to/image.webp", "webp"}},
		},
		{
			input:    "@animated.gif",
			expected: [][]string{{"@animated.gif", "animated.gif", "gif"}},
		},
		{
			input:    "@document.md is not an image",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			matches := imageRefPattern.FindAllStringSubmatch(tt.input, -1)
			if len(matches) != len(tt.expected) {
				t.Errorf("FindAllStringSubmatch(%q) got %d matches, want %d", tt.input, len(matches), len(tt.expected))
				return
			}
			for i, match := range matches {
				for j, part := range match {
					if j < len(tt.expected[i]) && part != tt.expected[i][j] {
						t.Errorf("match[%d][%d] = %q, want %q", i, j, part, tt.expected[i][j])
					}
				}
			}
		})
	}
}
