package session

import "strings"

func encodePath(path string) string {
	path = strings.TrimRight(path, "/")
	return strings.ReplaceAll(path, "/", "-")
}
