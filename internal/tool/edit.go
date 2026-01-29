package tool

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/tool/permission"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

const (
	IconEdit = "✏️"
)

// EditTool performs string replacement edits on files
type EditTool struct{}

func (t *EditTool) Name() string        { return "Edit" }
func (t *EditTool) Description() string { return "Edit file contents using string replacement" }
func (t *EditTool) Icon() string        { return IconEdit }

// RequiresPermission returns true - Edit always requires permission
func (t *EditTool) RequiresPermission() bool {
	return true
}

// PreparePermission prepares a permission request with diff information
func (t *EditTool) PreparePermission(ctx context.Context, params map[string]any, cwd string) (*permission.PermissionRequest, error) {
	// Get parameters
	filePath, ok := params["file_path"].(string)
	if !ok || filePath == "" {
		return nil, &ToolError{Message: "file_path is required"}
	}

	oldString, ok := params["old_string"].(string)
	if !ok {
		return nil, &ToolError{Message: "old_string is required"}
	}

	newString, ok := params["new_string"].(string)
	if !ok {
		return nil, &ToolError{Message: "new_string is required"}
	}

	// Resolve relative path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(cwd, filePath)
	}

	// Read current file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &ToolError{Message: "file not found: " + filePath}
		}
		return nil, &ToolError{Message: "failed to read file: " + err.Error()}
	}

	oldContent := string(content)

	// Check if old_string exists in the file
	count := strings.Count(oldContent, oldString)
	if count == 0 {
		return nil, &ToolError{Message: "old_string not found in file"}
	}

	// Check if replace_all is set
	replaceAll := false
	if v, ok := params["replace_all"].(bool); ok {
		replaceAll = v
	}

	// If not replace_all, check uniqueness
	if !replaceAll && count > 1 {
		return nil, &ToolError{Message: "old_string is not unique in file (found " + itoa(count) + " occurrences). Use replace_all=true to replace all."}
	}

	// Calculate new content
	var newContent string
	if replaceAll {
		newContent = strings.ReplaceAll(oldContent, oldString, newString)
	} else {
		newContent = strings.Replace(oldContent, oldString, newString, 1)
	}

	// Generate diff
	diffMeta := permission.GenerateDiff(filePath, oldContent, newContent)

	return &permission.PermissionRequest{
		ID:          generateRequestID(),
		ToolName:    t.Name(),
		FilePath:    filePath,
		Description: "Replace text in file",
		DiffMeta:    diffMeta,
	}, nil
}

// ExecuteApproved performs the file edit after user approval
func (t *EditTool) ExecuteApproved(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	// Get parameters
	filePath, _ := params["file_path"].(string)
	oldString, _ := params["old_string"].(string)
	newString, _ := params["new_string"].(string)

	// Resolve relative path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(cwd, filePath)
	}

	// Read current content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return ui.NewErrorResult(t.Name(), "failed to read file: "+err.Error())
	}

	oldContent := string(content)

	// Check replace_all
	replaceAll := false
	if v, ok := params["replace_all"].(bool); ok {
		replaceAll = v
	}

	// Perform replacement
	var newContent string
	var replaceCount int
	if replaceAll {
		replaceCount = strings.Count(oldContent, oldString)
		newContent = strings.ReplaceAll(oldContent, oldString, newString)
	} else {
		replaceCount = 1
		newContent = strings.Replace(oldContent, oldString, newString, 1)
	}

	// Write back to file
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return ui.NewErrorResult(t.Name(), "failed to write file: "+err.Error())
	}

	duration := time.Since(start)

	return ui.ToolResult{
		Success: true,
		Output:  "Successfully edited " + filePath + " (" + itoa(replaceCount) + " replacement(s))",
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: filePath,
			Duration: duration,
		},
	}
}

// Execute implements the Tool interface (for permission-unaware execution)
func (t *EditTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	// This will be called if permission flow is bypassed
	// For safety, we still perform the edit but without permission check
	return t.ExecuteApproved(ctx, params, cwd)
}

// ToolError represents a tool-specific error
type ToolError struct {
	Message string
}

func (e *ToolError) Error() string {
	return e.Message
}

// itoa converts int to string
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// generateRequestID generates a unique request ID using cryptographic randomness.
// This avoids collisions that could occur with time-based IDs in high-speed scenarios.
func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based if crypto/rand fails (unlikely)
		return "req_" + itoa(int(time.Now().UnixNano()%1000000))
	}
	return "req_" + hex.EncodeToString(b)
}

func init() {
	Register(&EditTool{})
}
