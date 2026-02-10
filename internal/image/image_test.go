package image

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsImageFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"screenshot.png", true},
		{"photo.jpg", true},
		{"photo.jpeg", true},
		{"animation.gif", true},
		{"modern.webp", true},
		{"document.md", false},
		{"code.go", false},
		{"data.json", false},
		{"PHOTO.PNG", true}, // Case insensitive
		{"Image.JPEG", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := IsImageFile(tt.path)
			if result != tt.expected {
				t.Errorf("IsImageFile(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/to/image.png")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

func TestLoad_UnsupportedFormat(t *testing.T) {
	// Create a temp file with unsupported extension
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmpFile)
	if err == nil {
		t.Error("Expected error for unsupported format, got nil")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{5242880, "5.0 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestSupportedTypes(t *testing.T) {
	// Verify all expected types are supported
	expectedTypes := map[string]string{
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".webp": "image/webp",
		".gif":  "image/gif",
	}

	for ext, mimeType := range expectedTypes {
		if SupportedTypes[ext] != mimeType {
			t.Errorf("SupportedTypes[%q] = %q, want %q", ext, SupportedTypes[ext], mimeType)
		}
	}
}
