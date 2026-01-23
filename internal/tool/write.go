package tool

import (
	"context"
	"os"
	"path/filepath"
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

	// Use preview mode for Write (show content directly, not diff)
	diffMeta := permission.GeneratePreview(filePath, content, isNewFile)

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

	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
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
		Output:  action + " " + filePath + " (" + itoa(lineCount) + " lines)",
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
