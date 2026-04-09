package session

import "strings"

func EncodePath(path string) string {
	return encodePath(path)
}

func encodePath(path string) string {
	path = strings.TrimRight(path, "/")
	return strings.ReplaceAll(path, "/", "-")
}
