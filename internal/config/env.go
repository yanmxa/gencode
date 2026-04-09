// Package env provides a single place to define environment variables that are
// exported to child processes (Bash tool, hooks, MCP servers, etc.).
//
// Every GEN_* variable automatically gets a CLAUDE_* alias so that Claude Code
// plugins work without modification.
package config

import "fmt"

const prefix = "GEN_"
const aliasPrefix = "CLAUDE_"

// Pair creates env entries for a single key=value, returning both the
// GEN_ and CLAUDE_ variants.
//
//	EnvPair("PROJECT_DIR", "/tmp") → ["GEN_PROJECT_DIR=/tmp", "CLAUDE_PROJECT_DIR=/tmp"]
func EnvPair(key, value string) []string {
	return []string{
		prefix + key + "=" + value,
		aliasPrefix + key + "=" + value,
	}
}

// Pairs creates env entries for multiple key=value pairs.
func EnvPairs(kvs ...string) []string {
	if len(kvs)%2 != 0 {
		panic("config.EnvPairs: odd number of arguments")
	}
	out := make([]string, 0, len(kvs))
	for i := 0; i < len(kvs); i += 2 {
		out = append(out, EnvPair(kvs[i], kvs[i+1])...)
	}
	return out
}

// PairF is like Pair but with a formatted suffix on the key.
//
//	EnvPairF("PLUGIN_ROOT_%s", "CODEX", "/path") →
//	  ["GEN_PLUGIN_ROOT_CODEX=/path", "CLAUDE_PLUGIN_ROOT_CODEX=/path"]
func EnvPairF(keyFmt, keyArg, value string) []string {
	key := fmt.Sprintf(keyFmt, keyArg)
	return EnvPair(key, value)
}
