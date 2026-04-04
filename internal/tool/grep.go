package tool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/tool/ui"
)

const rgBinary = "/usr/lib/node_modules/@anthropic-ai/claude-code/vendor/ripgrep/x64-linux/rg"

// GrepTool searches for patterns in files using ripgrep.
type GrepTool struct{}

func (t *GrepTool) Name() string        { return "Grep" }
func (t *GrepTool) Description() string { return "Search for patterns in files" }
func (t *GrepTool) Icon() string        { return ui.IconGrep }

func (t *GrepTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return ui.NewErrorResult(t.Name(), "pattern is required")
	}

	// Output mode: "content" | "files_with_matches" (default) | "count"
	outputMode := "files_with_matches"
	if om, ok := params["output_mode"].(string); ok && om != "" {
		outputMode = om
	}

	// Limits
	headLimit := 250
	if hl, ok := params["head_limit"].(float64); ok && hl > 0 {
		headLimit = int(hl)
	} else if hl, ok := params["head_limit"].(int); ok && hl > 0 {
		headLimit = hl
	}
	offset := 0
	if off, ok := params["offset"].(float64); ok && off > 0 {
		offset = int(off)
	} else if off, ok := params["offset"].(int); ok && off > 0 {
		offset = off
	}

	// Build rg args
	args := []string{"--no-messages"}

	// Case sensitivity
	caseInsensitive := true
	if ci, ok := params["-i"].(bool); ok {
		caseInsensitive = ci
	} else if cs, ok := params["case_sensitive"].(bool); ok && cs {
		caseInsensitive = false
	}
	if caseInsensitive {
		args = append(args, "--ignore-case")
	}

	// Multiline
	if ml, ok := params["multiline"].(bool); ok && ml {
		args = append(args, "--multiline", "--multiline-dotall")
	}

	// Output mode flags
	switch outputMode {
	case "files_with_matches":
		args = append(args, "--files-with-matches")
	case "count":
		args = append(args, "--count")
	default: // "content"
		args = append(args, "--line-number", "--with-filename", "--no-heading")

		// Context lines
		contextLines := 0
		if c, ok := params["context"].(float64); ok {
			contextLines = int(c)
		} else if c, ok := params["-C"].(float64); ok {
			contextLines = int(c)
		}
		if contextLines > 0 {
			args = append(args, fmt.Sprintf("--context=%d", contextLines))
		} else {
			afterLines := 0
			if a, ok := params["-A"].(float64); ok {
				afterLines = int(a)
			}
			beforeLines := 0
			if b, ok := params["-B"].(float64); ok {
				beforeLines = int(b)
			}
			if afterLines > 0 {
				args = append(args, fmt.Sprintf("--after-context=%d", afterLines))
			}
			if beforeLines > 0 {
				args = append(args, fmt.Sprintf("--before-context=%d", beforeLines))
			}
		}
	}

	// File type filter
	if fileType, ok := params["type"].(string); ok && fileType != "" {
		args = append(args, "--type", fileType)
	}

	// Glob filter
	if glob, ok := params["glob"].(string); ok && glob != "" {
		args = append(args, "--glob", glob)
	} else if include, ok := params["include"].(string); ok && include != "" {
		args = append(args, "--glob", include)
	}

	// Pattern
	args = append(args, "--", pattern)

	// Path
	searchPath := cwd
	if path, ok := params["path"].(string); ok && path != "" {
		if filepath.IsAbs(path) {
			searchPath = path
		} else {
			searchPath = filepath.Join(cwd, path)
		}
	}
	args = append(args, searchPath)

	// Execute rg
	rgPath := findRG()
	cmd := exec.CommandContext(ctx, rgPath, args...)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	// rg exits 1 when no matches found (not an error), 2 on actual error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 2 {
				return ui.NewErrorResult(t.Name(), "search error: "+stderr.String())
			}
		}
	}

	duration := time.Since(start)

	// Parse output, applying offset and head_limit
	rawLines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(rawLines) == 1 && rawLines[0] == "" {
		rawLines = nil
	}

	// Apply offset
	if offset > 0 && offset < len(rawLines) {
		rawLines = rawLines[offset:]
	} else if offset >= len(rawLines) {
		rawLines = nil
	}

	truncated := false
	if headLimit > 0 && len(rawLines) > headLimit {
		rawLines = rawLines[:headLimit]
		truncated = true
	}

	// Build content lines for UI
	var lines []ui.ContentLine
	for _, line := range rawLines {
		lines = append(lines, ui.ContentLine{Text: line, Type: ui.LineMatch})
	}

	subtitle := fmt.Sprintf("pattern: %q mode: %s", pattern, outputMode)

	hookContent := strings.Join(rawLines, "\n")
	if truncated {
		hookContent += "\n(results truncated)"
	}

	return ui.ToolResult{
		Success: true,
		Lines:   lines,
		HookResponse: map[string]any{
			"mode":     outputMode,
			"numLines": len(rawLines),
			"content":  hookContent,
		},
		Metadata: ui.ResultMetadata{
			Title:     t.Name(),
			Icon:      t.Icon(),
			Subtitle:  subtitle,
			ItemCount: len(rawLines),
			Duration:  duration,
			Truncated: truncated,
		},
	}
}

// findRG returns the path to the rg binary, preferring the bundled vendor binary.
func findRG() string {
	if _, err := exec.LookPath(rgBinary); err == nil {
		return rgBinary
	}
	if path, err := exec.LookPath("rg"); err == nil {
		return path
	}
	return "rg"
}

func init() {
	Register(&GrepTool{})
}
