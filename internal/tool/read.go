package tool

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
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

	filePath, err := requireString(params, "file_path")
	if err != nil {
		return ui.NewErrorResult(t.Name(), err.Error())
	}

	// Resolve relative path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(cwd, filePath)
	}

	offset := getInt(params, "offset", 0)
	limit := getInt(params, "limit", maxReadLines)

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
		if bytes.IndexByte(header[:n], 0) >= 0 {
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

	// Build content string for hook response
	var hookBuf strings.Builder
	for _, l := range lines {
		hookBuf.WriteString(l.Text)
		hookBuf.WriteByte('\n')
	}
	contentForHook := hookBuf.String()

	startLine := 1
	if offset > 0 {
		startLine = offset
	}

	// Build result
	result := ui.ToolResult{
		Success: true,
		Lines:   lines,
		HookResponse: map[string]any{
			"type": "text",
			"file": map[string]any{
				"filePath":  filePath,
				"content":   contentForHook,
				"numLines":  len(lines),
				"startLine": startLine,
				"totalLines": lineNo,
			},
		},
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
