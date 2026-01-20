/**
 * Shared Validation Utilities
 *
 * Common validation functions used across the codebase.
 */

/**
 * Validate resource name (commands, skills, agents, etc.)
 *
 * Resource names can only contain:
 * - Letters (a-z, A-Z)
 * - Numbers (0-9)
 * - Dash (-)
 * - Underscore (_)
 * - Colon (:) for namespaced commands (e.g., email:digest, jira:my-issues)
 *
 * @param name - Resource name to validate
 * @returns true if valid, false otherwise
 */
export function isValidResourceName(name: string): boolean {
  return /^[a-zA-Z0-9_:-]+$/.test(name);
}
