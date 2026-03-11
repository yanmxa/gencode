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
	encoded := strings.ReplaceAll(strings.TrimRight(cwd, "/"), "/", "-")
	return filepath.Join(homeDir, ".gen", "projects", encoded, "history")
}

func Load(cwd string) []string {
	path := historyFilePath(cwd)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var history []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		entry := strings.ReplaceAll(line, "\\n", "\n")
		entry = strings.ReplaceAll(entry, "\\\\", "\\")
		if entry != "" {
			history = append(history, entry)
		}
	}
	if len(history) > maxHistoryEntries {
		history = history[len(history)-maxHistoryEntries:]
	}
	return history
}

func Save(cwd string, history []string) {
	path := historyFilePath(cwd)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	entries := history
	if len(entries) > maxHistoryEntries {
		entries = entries[len(entries)-maxHistoryEntries:]
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, entry := range entries {
		escaped := strings.ReplaceAll(entry, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		fmt.Fprintln(w, escaped)
	}
	w.Flush()
}
