// Package markdown provides shared utilities for parsing markdown files.
package markdown

import (
	"bufio"
	"os"
	"strings"
)

// ParseFrontmatterFile reads a markdown file and returns (frontmatter, body).
// Frontmatter is the YAML content between opening and closing --- delimiters.
func ParseFrontmatterFile(path string) (frontmatter, body string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	var fmBuilder strings.Builder
	var bodyBuilder strings.Builder
	inFrontmatter := false
	frontmatterDone := false

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		if lineNum == 1 {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = true
				continue
			}
			bodyBuilder.WriteString(line)
			bodyBuilder.WriteString("\n")
			continue
		}

		if inFrontmatter && !frontmatterDone {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = false
				frontmatterDone = true
				continue
			}
			fmBuilder.WriteString(line)
			fmBuilder.WriteString("\n")
		} else {
			bodyBuilder.WriteString(line)
			bodyBuilder.WriteString("\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return "", "", err
	}

	return fmBuilder.String(), strings.TrimSpace(bodyBuilder.String()), nil
}
