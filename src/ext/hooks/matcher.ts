/**
 * Hook Matcher - Pattern matching for filtering hooks by tool name
 *
 * Supports:
 * - Wildcard matching (empty string, "*")
 * - Exact string matching
 * - Regex patterns (e.g., "Edit|Write")
 */

// =============================================================================
// Matcher Functions
// =============================================================================

/**
 * Check if a matcher pattern matches a tool name
 *
 * Pattern types:
 * - undefined, "", "*": Match all tools
 * - "ToolName": Exact match
 * - "Tool1|Tool2": Regex alternation (or any valid regex)
 *
 * @param pattern - Matcher pattern (can be undefined, string, or regex)
 * @param toolName - Tool name to match against (can be undefined)
 * @returns true if pattern matches, false otherwise
 */
export function matchesTool(pattern: string | undefined, toolName: string | undefined): boolean {
  // Wildcard patterns - match everything (including events without toolName like SessionStart)
  if (!pattern || pattern === '' || pattern === '*') {
    return true;
  }

  // No tool name to match against - can only match wildcard (already handled above)
  if (!toolName) {
    return false;
  }

  // Try regex matching first
  try {
    const regex = new RegExp(pattern);
    return regex.test(toolName);
  } catch {
    // If regex fails, fall back to exact string match
    return pattern === toolName;
  }
}

// =============================================================================
// Matcher Validation
// =============================================================================

/**
 * Validate if a matcher pattern is a valid regex
 *
 * @param pattern - Pattern to validate
 * @returns true if valid regex, false otherwise
 */
export function isValidRegex(pattern: string): boolean {
  try {
    new RegExp(pattern);
    return true;
  } catch {
    return false;
  }
}

/**
 * Test if a pattern will match a specific tool name
 * Useful for configuration validation and testing
 *
 * @param pattern - Matcher pattern
 * @param toolName - Tool name to test
 * @returns true if pattern would match this tool
 */
export function testMatch(pattern: string | undefined, toolName: string): boolean {
  return matchesTool(pattern, toolName);
}
