package permission

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// IsInWorkingDirectory checks if a file path is within any of the allowed
// working directories. It checks both original and symlink-resolved forms
// to prevent symlink-based escapes.
//
// Returns true if the path is within at least one working directory.
// Returns true if workingDirs is empty (no constraint).
func IsInWorkingDirectory(filePath string, workingDirs []string) bool {
	if len(workingDirs) == 0 {
		return true
	}

	// Normalize the input path
	origPath := cleanPath(filePath)
	resolvedPath := resolvePath(origPath)

	for _, dir := range workingDirs {
		origDir := cleanPath(dir)
		resolvedDir := resolvePath(origDir)

		// Both original and resolved forms must be within some form of the
		// working directory to prevent symlink escapes.
		origInOrig := IsSubpath(origPath, origDir)
		origInResolved := IsSubpath(origPath, resolvedDir)
		resolvedInOrig := IsSubpath(resolvedPath, origDir)
		resolvedInResolved := IsSubpath(resolvedPath, resolvedDir)

		// At least one combination of original path forms must match,
		// AND at least one combination of resolved path forms must match.
		origOK := origInOrig || origInResolved
		resolvedOK := resolvedInOrig || resolvedInResolved

		if origOK && resolvedOK {
			return true
		}
	}

	return false
}

// IsSubpath checks if child is inside parent directory.
func IsSubpath(child, parent string) bool {
	child = NormalizeMacOSPath(child)
	parent = NormalizeMacOSPath(parent)

	// Ensure parent ends with separator for prefix matching
	if !strings.HasSuffix(parent, string(os.PathSeparator)) {
		parent += string(os.PathSeparator)
	}

	// Exact match (child IS the parent dir)
	childWithSep := child + string(os.PathSeparator)
	if childWithSep == parent {
		return true
	}

	return strings.HasPrefix(child, parent)
}

// cleanPath normalizes a path: resolves to absolute, cleans . and .. components.
func cleanPath(path string) string {
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	return filepath.Clean(path)
}

// resolvePath resolves symlinks in a path. Returns the cleaned path on error.
func resolvePath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		// If the file doesn't exist yet, try resolving the parent directory
		dir := filepath.Dir(path)
		if resolvedDir, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(resolvedDir, filepath.Base(path))
		}
		return path
	}
	return resolved
}

// NormalizeMacOSPath handles macOS-specific symlink quirks where
// /var -> /private/var and /tmp -> /private/tmp.
func NormalizeMacOSPath(path string) string {
	if runtime.GOOS != "darwin" {
		return path
	}

	// Strip /private prefix for consistent comparison
	prefixes := []string{"/private/var/", "/private/tmp/"}
	targets := []string{"/var/", "/tmp/"}

	for i, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return targets[i] + path[len(prefix):]
		}
	}

	// Also handle exact matches
	if path == "/private/var" {
		return "/var"
	}
	if path == "/private/tmp" {
		return "/tmp"
	}

	return path
}
