package tool

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/yanmxa/gencode/internal/tool/ui"
)

const (
	maxReadLines  = 2000
	maxLineLength = 500
)

// ReadTool reads file contents
type ReadTool struct{}

func (t *ReadTool) Name() string        { return "Read" }
func (t *ReadTool) Description() string { return "Read file contents" }
func (t *ReadTool) Icon() string        { return ui.IconRead }

func (t *ReadTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	// Get file path parameter
	filePath, ok := params["file_path"].(string)
	if !ok || filePath == "" {
		return ui.NewErrorResult(t.Name(), "file_path is required")
	}

	// Resolve relative path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(cwd, filePath)
	}

	// Get optional parameters
	offset := 0
	if v, ok := params["offset"].(int); ok {
		offset = v
	} else if v, ok := params["offset"].(float64); ok {
		offset = int(v)
	}

	limit := maxReadLines
	if v, ok := params["limit"].(int); ok && v > 0 {
		limit = v
	} else if v, ok := params["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	// Get file info
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ui.NewErrorResult(t.Name(), "file not found: "+filePath)
		}
		return ui.NewErrorResult(t.Name(), "failed to stat file: "+err.Error())
	}

	if info.IsDir() {
		return ui.NewErrorResult(t.Name(), "path is a directory: "+filePath)
	}

	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return ui.NewErrorResult(t.Name(), "failed to open file: "+err.Error())
	}
	defer file.Close()

	// Check for binary file by reading first 512 bytes
	header := make([]byte, 512)
	n, _ := file.Read(header)
	if n > 0 {
		for _, b := range header[:n] {
			if b == 0 {
				return ui.ToolResult{
					Success: true,
					Output:  "Binary file detected: " + filePath,
					Metadata: ui.ResultMetadata{
						Title:    t.Name(),
						Icon:     t.Icon(),
						Subtitle: filePath + " (binary)",
						Size:     info.Size(),
					},
				}
			}
		}
	}
	// Reset file position to beginning
	file.Seek(0, 0)

	// Read lines
	var lines []ui.ContentLine
	scanner := bufio.NewScanner(file)
	lineNo := 0
	readCount := 0
	truncated := false

	for scanner.Scan() {
		lineNo++

		// Skip lines before offset
		if offset > 0 && lineNo < offset {
			continue
		}

		// Check limit
		if readCount >= limit {
			truncated = true
			break
		}

		text := scanner.Text()

		// Truncate long lines
		if len(text) > maxLineLength {
			text = text[:maxLineLength] + "..."
		}

		lines = append(lines, ui.ContentLine{
			LineNo: lineNo,
			Text:   text,
			Type:   ui.LineNormal,
		})
		readCount++
	}

	if err := scanner.Err(); err != nil {
		return ui.NewErrorResult(t.Name(), "error reading file: "+err.Error())
	}

	duration := time.Since(start)

	// Build result
	result := ui.ToolResult{
		Success: true,
		Lines:   lines,
		Metadata: ui.ResultMetadata{
			Title:     t.Name(),
			Icon:      t.Icon(),
			Subtitle:  filePath,
			Size:      info.Size(),
			LineCount: len(lines),
			Duration:  duration,
			Truncated: truncated,
		},
	}

	return result
}

func init() {
	Register(&ReadTool{})
}
