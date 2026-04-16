package transport

import (
	"maps"
	"os"
	"regexp"
	"strings"
)

// Environment variable expansion utilities.
// Supports ${VAR} and ${VAR:-default} syntax.

var (
	simpleVarPattern  = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	defaultVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*):-([^}]*)\}`)
)

// expandEnv expands environment variables in a string.
// Supports ${VAR} and ${VAR:-default} syntax.
func expandEnv(s string) string {
	// First handle ${VAR:-default} patterns
	result := defaultVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := defaultVarPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		if val, ok := os.LookupEnv(parts[1]); ok {
			return val
		}
		return parts[2]
	})

	// Then handle simple ${VAR} patterns
	return simpleVarPattern.ReplaceAllStringFunc(result, func(match string) string {
		parts := simpleVarPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		return os.Getenv(parts[1])
	})
}

// expandEnvSlice expands environment variables in each string of a slice.
func expandEnvSlice(s []string) []string {
	if s == nil {
		return nil
	}
	result := make([]string, len(s))
	for i, v := range s {
		result[i] = expandEnv(v)
	}
	return result
}

// expandEnvMap expands environment variables in each value of a map.
func expandEnvMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = expandEnv(v)
	}
	return result
}

// buildEnv creates an environment slice by merging the current environment
// with additional variables from configEnv.
func buildEnv(configEnv map[string]string) []string {
	env := os.Environ()
	if len(configEnv) == 0 {
		return env
	}

	envMap := make(map[string]string)
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok {
			envMap[k] = v
		}
	}

	maps.Copy(envMap, configEnv)

	result := make([]string, 0, len(envMap))
	for k, v := range envMap {
		result = append(result, k+"="+v)
	}
	return result
}
