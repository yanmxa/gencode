package permission

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
)

// GenerateDiff creates a DiffMetadata from old and new file content
func GenerateDiff(filePath, oldContent, newContent string) *DiffMetadata {
	// Generate unified diff using myers algorithm
	edits := myers.ComputeEdits(span.URIFromPath(filePath), oldContent, newContent)
	unifiedDiff := gotextdiff.ToUnified(filePath, filePath, oldContent, edits)

	// Convert to string using fmt.Sprint
	diffStr := fmt.Sprint(unifiedDiff)

	// Parse the diff into structured lines
	lines := ParseDiffLines(diffStr)

	// Count additions and removals
	addedCount := 0
	removedCount := 0
	for _, line := range lines {
		switch line.Type {
		case DiffLineAdded:
			addedCount++
		case DiffLineRemoved:
			removedCount++
		}
	}

	return &DiffMetadata{
		OldContent:   oldContent,
		NewContent:   newContent,
		UnifiedDiff:  diffStr,
		Lines:        lines,
		IsNewFile:    oldContent == "",
		AddedCount:   addedCount,
		RemovedCount: removedCount,
	}
}

// hunkHeaderRegex matches @@ -1,3 +1,4 @@ style headers
var hunkHeaderRegex = regexp.MustCompile(`^@@\s+-(\d+)(?:,\d+)?\s+\+(\d+)(?:,\d+)?\s+@@`)

// ParseDiffLines parses unified diff text into structured DiffLine slices
func ParseDiffLines(unifiedDiff string) []DiffLine {
	if unifiedDiff == "" {
		return nil
	}

	var lines []DiffLine
	diffLines := strings.Split(unifiedDiff, "\n")

	var oldLineNo, newLineNo int

	for _, line := range diffLines {
		// Skip file headers (---, +++)
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}

		// Handle metadata lines (e.g., "\ No newline at end of file")
		// These should not increment line numbers
		if strings.HasPrefix(line, "\\") {
			lines = append(lines, DiffLine{
				Type:    DiffLineMetadata,
				Content: strings.TrimPrefix(line, "\\ "),
			})
			continue
		}

		// Handle hunk headers
		if matches := hunkHeaderRegex.FindStringSubmatch(line); matches != nil {
			// Parse starting line numbers from hunk header
			oldLineNo, _ = strconv.Atoi(matches[1])
			newLineNo, _ = strconv.Atoi(matches[2])

			lines = append(lines, DiffLine{
				Type:    DiffLineHunk,
				Content: line,
			})
			continue
		}

		// Handle content lines
		if len(line) == 0 {
			// Empty line in context
			lines = append(lines, DiffLine{
				Type:      DiffLineContext,
				Content:   "",
				OldLineNo: oldLineNo,
				NewLineNo: newLineNo,
			})
			oldLineNo++
			newLineNo++
			continue
		}

		prefix := line[0]
		content := ""
		if len(line) > 1 {
			content = line[1:]
		}

		switch prefix {
		case '+':
			lines = append(lines, DiffLine{
				Type:      DiffLineAdded,
				Content:   content,
				NewLineNo: newLineNo,
			})
			newLineNo++
		case '-':
			lines = append(lines, DiffLine{
				Type:      DiffLineRemoved,
				Content:   content,
				OldLineNo: oldLineNo,
			})
			oldLineNo++
		case ' ':
			lines = append(lines, DiffLine{
				Type:      DiffLineContext,
				Content:   content,
				OldLineNo: oldLineNo,
				NewLineNo: newLineNo,
			})
			oldLineNo++
			newLineNo++
		default:
			// Unknown prefix, treat as context
			lines = append(lines, DiffLine{
				Type:      DiffLineContext,
				Content:   line,
				OldLineNo: oldLineNo,
				NewLineNo: newLineNo,
			})
			oldLineNo++
			newLineNo++
		}
	}

	return lines
}

// GenerateNewFileDiff creates a DiffMetadata for a new file (all lines are additions)
func GenerateNewFileDiff(filePath, content string) *DiffMetadata {
	lines := strings.Split(content, "\n")
	diffLines := make([]DiffLine, 0, len(lines)+1)

	// Add hunk header
	diffLines = append(diffLines, DiffLine{
		Type:    DiffLineHunk,
		Content: fmt.Sprintf("@@ -0,0 +1,%d @@", len(lines)),
	})

	// All lines are additions
	for i, line := range lines {
		diffLines = append(diffLines, DiffLine{
			Type:      DiffLineAdded,
			Content:   line,
			NewLineNo: i + 1,
		})
	}

	return &DiffMetadata{
		OldContent:   "",
		NewContent:   content,
		Lines:        diffLines,
		IsNewFile:    true,
		AddedCount:   len(lines),
		RemovedCount: 0,
	}
}

// GeneratePreview creates a DiffMetadata for content preview (used by Write tool)
// Shows the content directly without diff format
func GeneratePreview(filePath, content string, isNewFile bool) *DiffMetadata {
	lines := strings.Split(content, "\n")
	previewLines := make([]DiffLine, 0, len(lines))

	// Create preview lines (all as context for display purposes)
	for i, line := range lines {
		previewLines = append(previewLines, DiffLine{
			Type:      DiffLineContext,
			Content:   line,
			NewLineNo: i + 1,
		})
	}

	return &DiffMetadata{
		OldContent:   "",
		NewContent:   content,
		Lines:        previewLines,
		IsNewFile:    isNewFile,
		PreviewMode:  true,
		AddedCount:   len(lines),
		RemovedCount: 0,
	}
}
