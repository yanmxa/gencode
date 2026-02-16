// Package image provides image loading, validation, and encoding utilities.
package image

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/yanmxa/gencode/internal/message"
)

const (
	// MaxImageSize is the maximum allowed image size (5MB)
	MaxImageSize = 5 * 1024 * 1024
)

// SupportedTypes maps file extensions to MIME types
var SupportedTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
}

// ImageInfo holds information about a loaded image
type ImageInfo struct {
	Path      string
	MediaType string
	Data      []byte
	Size      int
	FileName  string
}

// Load loads and validates an image from the given path
func Load(path string) (*ImageInfo, error) {
	// Resolve path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	// Check if file exists
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return nil, fmt.Errorf("cannot access file: %w", err)
	}

	// Check file size
	if info.Size() > MaxImageSize {
		return nil, fmt.Errorf("image too large: %d bytes (max %d)", info.Size(), MaxImageSize)
	}

	// Check extension
	ext := strings.ToLower(filepath.Ext(absPath))
	mediaType, ok := SupportedTypes[ext]
	if !ok {
		return nil, fmt.Errorf("unsupported image format: %s", ext)
	}

	// Read file
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Detect actual content type to verify
	detectedType := http.DetectContentType(data)
	if !strings.HasPrefix(detectedType, "image/") {
		return nil, fmt.Errorf("file is not a valid image")
	}

	return &ImageInfo{
		Path:      absPath,
		MediaType: mediaType,
		Data:      data,
		Size:      len(data),
		FileName:  filepath.Base(absPath),
	}, nil
}

// IsImageFile returns true if the file extension indicates a supported image format
func IsImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := SupportedTypes[ext]
	return ok
}

// ToBase64 returns the image data as a base64 encoded string
func (i *ImageInfo) ToBase64() string {
	return base64.StdEncoding.EncodeToString(i.Data)
}

// ToProviderData converts ImageInfo to message.ImageData
func (i *ImageInfo) ToProviderData() message.ImageData {
	return message.ImageData{
		MediaType: i.MediaType,
		Data:      i.ToBase64(),
		FileName:  i.FileName,
		Size:      i.Size,
	}
}

// FormatBytes formats byte size as human-readable string
func FormatBytes(bytes int) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := unit, 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
