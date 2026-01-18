/**
 * Environment Variable Expansion
 * Supports ${VAR} and ${VAR:-default} syntax
 */

/**
 * Expand environment variables in a string
 * Supports:
 * - ${VAR} - Replace with environment variable value, empty string if not set
 * - ${VAR:-default} - Replace with environment variable value, or default if not set
 *
 * @param str String to expand
 * @returns Expanded string
 */
export function expandEnvVars(str: string): string {
  return str.replace(/\$\{([^}:]+)(:-([^}]+))?\}/g, (match, varName, _, defaultValue) => {
    const value = process.env[varName];

    if (value !== undefined) {
      return value;
    }

    if (defaultValue !== undefined) {
      return defaultValue;
    }

    // Variable not set and no default - return empty string
    return '';
  });
}

/**
 * Expand environment variables in an object recursively
 * Handles strings, arrays, and nested objects
 */
export function expandEnvVarsInObject<T>(obj: T): T {
  if (typeof obj === 'string') {
    return expandEnvVars(obj) as T;
  }

  if (Array.isArray(obj)) {
    return obj.map(expandEnvVarsInObject) as T;
  }

  if (obj !== null && typeof obj === 'object') {
    const result: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(obj)) {
      result[key] = expandEnvVarsInObject(value);
    }
    return result as T;
  }

  return obj;
}

/**
 * Expand environment variables in MCP server configuration
 */
export function expandServerConfig<T extends Record<string, unknown>>(config: T): T {
  return expandEnvVarsInObject(config);
}
