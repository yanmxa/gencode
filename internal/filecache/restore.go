package filecache

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type RestoredFile struct {
	FilePath string
	Content  string
	Lines    int
}

func (c *Cache) RestoreRecent() ([]RestoredFile, int) {
	entries := c.Recent(RestoreMaxFiles)
	if len(entries) == 0 {
		return nil, 0
	}

	var files []RestoredFile
	totalTokens := 0

	for _, entry := range entries {
		content, lines, tokens := readFileForRestore(entry.FilePath)
		if content == "" {
			continue
		}
		if totalTokens+tokens > RestoreMaxTotal {
			if totalTokens == 0 {
				content, lines, tokens = truncateToTokenBudget(content, RestoreMaxTotal)
			} else {
				break
			}
		}
		files = append(files, RestoredFile{
			FilePath: entry.FilePath,
			Content:  content,
			Lines:    lines,
		})
		totalTokens += tokens
	}

	return files, totalTokens
}

func FormatRestoredFiles(files []RestoredFile) string {
	if len(files) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<post-compact-context>\n")
	sb.WriteString("The following files were recently accessed before compaction and are restored for context:\n\n")

	for _, f := range files {
		fmt.Fprintf(&sb, "<file path=%q lines=%d>\n", f.FilePath, f.Lines)
		sb.WriteString(f.Content)
		if !strings.HasSuffix(f.Content, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("</file>\n\n")
	}

	sb.WriteString("</post-compact-context>")
	return sb.String()
}

func readFileForRestore(filePath string) (content string, lines int, tokens int) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", 0, 0
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return "", 0, 0
	}

	var sb strings.Builder
	scanner := bufio.NewScanner(file)
	lineCount := 0
	charCount := 0

	for scanner.Scan() {
		lineCount++
		line := scanner.Text()
		charCount += len(line) + 1
		estimatedTokens := charCount / 4
		if estimatedTokens > RestoreMaxPerFile {
			break
		}
		if lineCount > 1 {
			sb.WriteString("\n")
		}
		sb.WriteString(line)
	}

	if lineCount == 0 {
		return "", 0, 0
	}

	text := sb.String()
	return text, lineCount, len(text) / 4
}

func truncateToTokenBudget(content string, maxTokens int) (string, int, int) {
	maxChars := maxTokens * 4
	if len(content) <= maxChars {
		lines := strings.Count(content, "\n") + 1
		return content, lines, len(content) / 4
	}
	content = content[:maxChars]
	if idx := strings.LastIndex(content, "\n"); idx > 0 {
		content = content[:idx]
	}
	lines := strings.Count(content, "\n") + 1
	return content, lines, len(content) / 4
}
