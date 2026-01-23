package tool

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/myan/gencode/internal/tool/ui"
)

const (
	maxGrepMatches = 50
	maxGrepFiles   = 100
)

// GrepTool searches for patterns in files
type GrepTool struct{}

func (t *GrepTool) Name() string        { return "Grep" }
func (t *GrepTool) Description() string { return "Search for patterns in files" }
func (t *GrepTool) Icon() string        { return ui.IconGrep }

func (t *GrepTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	// Get pattern parameter
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return ui.NewErrorResult(t.Name(), "pattern is required")
	}

	// Compile regex (case insensitive by default)
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return ui.NewErrorResult(t.Name(), "invalid pattern: "+err.Error())
	}

	// Get optional path parameter
	basePath := cwd
	if path, ok := params["path"].(string); ok && path != "" {
		if filepath.IsAbs(path) {
			basePath = path
		} else {
			basePath = filepath.Join(cwd, path)
		}
	}

	// Get optional include pattern
	includePattern := ""
	if include, ok := params["include"].(string); ok {
		includePattern = include
	}

	// Check if base path exists
	info, err := os.Stat(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ui.NewErrorResult(t.Name(), "path not found: "+basePath)
		}
		return ui.NewErrorResult(t.Name(), "failed to access path: "+err.Error())
	}

	var matches []ui.ContentLine
	filesSearched := 0
	filesWithMatches := 0

	// Search in a single file
	searchFile := func(filePath, relPath string) error {
		file, err := os.Open(filePath)
		if err != nil {
			return nil // Skip files we can't read
		}
		defer file.Close()

		// Check if binary file (skip)
		buf := make([]byte, 512)
		n, _ := file.Read(buf)
		if n > 0 && isBinary(buf[:n]) {
			return nil
		}

		// Reset to beginning
		file.Seek(0, 0)

		scanner := bufio.NewScanner(file)
		lineNo := 0
		fileHasMatch := false

		for scanner.Scan() {
			lineNo++
			line := scanner.Text()

			if re.MatchString(line) {
				if !fileHasMatch {
					filesWithMatches++
					fileHasMatch = true
				}

				// Truncate long lines
				displayLine := line
				if len(displayLine) > maxLineLength {
					displayLine = displayLine[:maxLineLength] + "..."
				}

				matches = append(matches, ui.ContentLine{
					LineNo: lineNo,
					Text:   strings.TrimSpace(displayLine),
					Type:   ui.LineMatch,
					File:   relPath,
				})

				// Check limit
				if len(matches) >= maxGrepMatches {
					return filepath.SkipAll
				}
			}
		}

		return nil
	}

	// If path is a file, search only that file
	if !info.IsDir() {
		relPath := filepath.Base(basePath)
		searchFile(basePath, relPath)
	} else {
		// Walk directory tree
		filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			// Check context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Skip directories
			if d.IsDir() {
				if ignoredDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}

			// Check include pattern
			if includePattern != "" {
				matched, _ := filepath.Match(includePattern, d.Name())
				if !matched {
					return nil
				}
			}

			// Get relative path
			relPath, err := filepath.Rel(basePath, path)
			if err != nil {
				relPath = path
			}

			filesSearched++
			if filesSearched > maxGrepFiles {
				return filepath.SkipAll
			}

			return searchFile(path, relPath)
		})
	}

	duration := time.Since(start)

	// Build subtitle
	subtitle := "pattern: \"" + pattern + "\""
	if includePattern != "" {
		subtitle += " (" + includePattern + ")"
	}

	truncated := len(matches) >= maxGrepMatches

	result := ui.ToolResult{
		Success: true,
		Lines:   matches,
		Metadata: ui.ResultMetadata{
			Title:     t.Name(),
			Icon:      t.Icon(),
			Subtitle:  subtitle,
			ItemCount: len(matches),
			Duration:  duration,
			Truncated: truncated,
		},
	}

	return result
}

// isBinary checks if data appears to be binary
func isBinary(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func init() {
	Register(&GrepTool{})
}
