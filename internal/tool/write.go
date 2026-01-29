package tool

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/yanmxa/gencode/internal/tool/permission"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

const (
	IconWrite = "📝"
)

// WriteTool writes content to files
type WriteTool struct{}

func (t *WriteTool) Name() string        { return "Write" }
func (t *WriteTool) Description() string { return "Write content to a file" }
func (t *WriteTool) Icon() string        { return IconWrite }

// RequiresPermission returns true - Write always requires permission
func (t *WriteTool) RequiresPermission() bool {
	return true
}

// PreparePermission prepares a permission request with diff information
func (t *WriteTool) PreparePermission(ctx context.Context, params map[string]any, cwd string) (*permission.PermissionRequest, error) {
	// Get parameters
	filePath, ok := params["file_path"].(string)
	if !ok || filePath == "" {
		return nil, &ToolError{Message: "file_path is required"}
	}

	content, ok := params["content"].(string)
	if !ok {
		return nil, &ToolError{Message: "content is required"}
	}

	// Resolve relative path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(cwd, filePath)
	}

	// Check if file exists
	_, err := os.Stat(filePath)
	isNewFile := os.IsNotExist(err)
	if err != nil && !isNewFile {
		return nil, &ToolError{Message: "failed to check file: " + err.Error()}
	}

	// Generate appropriate preview based on whether file exists
	var diffMeta *permission.DiffMetadata
	if isNewFile {
		// New file: use preview mode to show content directly
		diffMeta = permission.GeneratePreview(filePath, content, true)
	} else {
		// Existing file: generate actual diff to show what will change
		oldContent, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return nil, &ToolError{Message: "failed to read existing file: " + readErr.Error()}
		}
		diffMeta = permission.GenerateDiff(filePath, string(oldContent), content)
	}

	description := "Create new file"
	if !isNewFile {
		description = "Overwrite existing file"
	}

	return &permission.PermissionRequest{
		ID:          generateRequestID(),
		ToolName:    t.Name(),
		FilePath:    filePath,
		Description: description,
		DiffMeta:    diffMeta,
	}, nil
}

// ExecuteApproved performs the file write after user approval
func (t *WriteTool) ExecuteApproved(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	// Get parameters
	filePath, _ := params["file_path"].(string)
	content, _ := params["content"].(string)

	// Resolve relative path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(cwd, filePath)
	}

	// Create parent directories if needed
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ui.NewErrorResult(t.Name(), "failed to create directory: "+err.Error())
	}

	// Check if file exists (for status message)
	_, err := os.Stat(filePath)
	isNewFile := os.IsNotExist(err)

	// Get optional mode parameter (default 0644)
	mode := os.FileMode(0644)
	if modeVal, ok := params["mode"].(float64); ok && modeVal > 0 {
		mode = os.FileMode(int(modeVal))
	} else if modeVal, ok := params["mode"].(int); ok && modeVal > 0 {
		mode = os.FileMode(modeVal)
	}

	// Write file
	if err := os.WriteFile(filePath, []byte(content), mode); err != nil {
		return ui.NewErrorResult(t.Name(), "failed to write file: "+err.Error())
	}

	duration := time.Since(start)

	action := "Created"
	if !isNewFile {
		action = "Updated"
	}

	// Count lines
	lineCount := 1
	for _, c := range content {
		if c == '\n' {
			lineCount++
		}
	}

	return ui.ToolResult{
		Success: true,
		Output:  action + " " + filePath + " (" + strconv.Itoa(lineCount) + " lines)",
		Metadata: ui.ResultMetadata{
			Title:     t.Name(),
			Icon:      t.Icon(),
			Subtitle:  filePath,
			LineCount: lineCount,
			Duration:  duration,
		},
	}
}

// Execute implements the Tool interface (for permission-unaware execution)
func (t *WriteTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	// This will be called if permission flow is bypassed
	return t.ExecuteApproved(ctx, params, cwd)
}

func init() {
	Register(&WriteTool{})
}
