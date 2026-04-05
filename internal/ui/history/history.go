package history

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxHistoryEntries = 500

func historyFilePath(cwd string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(cwd, ".gen", "history")
	}
	encoded := strings.ReplaceAll(strings.TrimSuffix(cwd, "/"), "/", "-")
	return filepath.Join(homeDir, ".gen", "projects", encoded, "history")
}

func escapeEntry(entry string) string {
	entry = strings.ReplaceAll(entry, "\\", "\\\\")
	return strings.ReplaceAll(entry, "\n", "\\n")
}

func unescapeEntry(line string) string {
	line = strings.ReplaceAll(line, "\\n", "\n")
	return strings.ReplaceAll(line, "\\\\", "\\")
}

func truncate(entries []string) []string {
	if len(entries) > maxHistoryEntries {
		return entries[len(entries)-maxHistoryEntries:]
	}
	return entries
}

func Load(cwd string) []string {
	f, err := os.Open(historyFilePath(cwd))
	if err != nil {
		return nil
	}
	defer f.Close()

	var history []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if entry := unescapeEntry(scanner.Text()); entry != "" {
			history = append(history, entry)
		}
	}
	return truncate(history)
}

func Save(cwd string, history []string) {
	path := historyFilePath(cwd)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, entry := range truncate(history) {
		_, _ = fmt.Fprintln(w, escapeEntry(entry))
	}
	_ = w.Flush()
}
