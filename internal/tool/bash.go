package tool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/tool/permission"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

const (
	IconBash = "$"
)

// BashTool executes shell commands
type BashTool struct{}

func (t *BashTool) Name() string        { return "Bash" }
func (t *BashTool) Description() string { return "Execute shell commands" }
func (t *BashTool) Icon() string        { return IconBash }

// RequiresPermission returns true - Bash always requires permission
func (t *BashTool) RequiresPermission() bool {
	return true
}

// PreparePermission prepares a permission request with command preview
func (t *BashTool) PreparePermission(ctx context.Context, params map[string]any, cwd string) (*permission.PermissionRequest, error) {
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return nil, &ToolError{Message: "command is required"}
	}

	description, _ := params["description"].(string)
	runBackground, _ := params["run_in_background"].(bool)

	// Count lines in command
	lineCount := strings.Count(command, "\n") + 1

	return &permission.PermissionRequest{
		ID:          generateRequestID(),
		ToolName:    t.Name(),
		Description: description,
		BashMeta: &permission.BashMetadata{
			Command:       command,
			Description:   description,
			RunBackground: runBackground,
			LineCount:     lineCount,
		},
	}, nil
}

// ExecuteApproved executes the command after user approval
func (t *BashTool) ExecuteApproved(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	command, _ := params["command"].(string)
	description, _ := params["description"].(string)

	// Get timeout (default 120 seconds, max 600 seconds)
	timeout := 120 * time.Second
	if timeoutMs, ok := params["timeout"].(float64); ok && timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
		if timeout > 600*time.Second {
			timeout = 600 * time.Second
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute command
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = cwd

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	output := stdout.String()
	errOutput := stderr.String()

	// Combine output
	fullOutput := output
	if errOutput != "" {
		if fullOutput != "" {
			fullOutput += "\n"
		}
		fullOutput += errOutput
	}

	// Count lines
	lineCount := 0
	if fullOutput != "" {
		lineCount = strings.Count(strings.TrimSuffix(fullOutput, "\n"), "\n") + 1
	}

	// Truncate if too long
	const maxLen = 30000
	truncated := false
	if len(fullOutput) > maxLen {
		fullOutput = fullOutput[:maxLen] + "\n... (output truncated)"
		truncated = true
	}

	if err != nil {
		// Check if it's a timeout
		if ctx.Err() == context.DeadlineExceeded {
			return ui.ToolResult{
				Success: false,
				Output:  fullOutput,
				Error:   "command timed out after " + timeout.String(),
				Metadata: ui.ResultMetadata{
					Title:     t.Name(),
					Icon:      t.Icon(),
					Subtitle:  "Timeout",
					LineCount: lineCount,
					Duration:  duration,
				},
			}
		}

		// Command failed
		errorMsg := err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			errorMsg = fmt.Sprintf("exit code %d", exitErr.ExitCode())
		}

		return ui.ToolResult{
			Success: false,
			Output:  fullOutput,
			Error:   errorMsg,
			Metadata: ui.ResultMetadata{
				Title:     t.Name(),
				Icon:      t.Icon(),
				Subtitle:  "Failed: " + errorMsg,
				LineCount: lineCount,
				Duration:  duration,
			},
		}
	}

	// Build subtitle
	subtitle := "Done"
	if description != "" {
		subtitle = description
	} else if truncated {
		subtitle = fmt.Sprintf("%d+ lines (truncated)", lineCount)
	} else if lineCount > 1 {
		subtitle = fmt.Sprintf("%d lines", lineCount)
	} else if output != "" {
		// Show first line preview for single-line output
		firstLine := strings.TrimSpace(strings.Split(output, "\n")[0])
		if len(firstLine) > 50 {
			firstLine = firstLine[:50] + "..."
		}
		if firstLine != "" {
			subtitle = firstLine
		}
	}

	return ui.ToolResult{
		Success: true,
		Output:  fullOutput,
		Metadata: ui.ResultMetadata{
			Title:     t.Name(),
			Icon:      t.Icon(),
			Subtitle:  subtitle,
			LineCount: lineCount,
			Duration:  duration,
		},
	}
}

// Execute implements the Tool interface (for permission-unaware execution)
func (t *BashTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	// This will be called if permission flow is bypassed
	return t.ExecuteApproved(ctx, params, cwd)
}

func init() {
	Register(&BashTool{})
}
