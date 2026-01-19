/**
 * Debug configuration module
 * Controls debug output for various GenCode components
 *
 * Debug Levels:
 * - GEN_DEBUG=0 or unset: No debug output (default)
 * - GEN_DEBUG=1: Standard debug output (component loading, key operations)
 * - GEN_DEBUG=2: Verbose debug output (detailed execution, parameters, results)
 */

export type DebugLevel = 0 | 1 | 2;

export interface DebugConfig {
  level: DebugLevel;
  enabled: boolean; // level >= 1
  verbose: boolean; // level >= 2
  components: {
    skills: DebugLevel;
    commands: DebugLevel;
    subagents: DebugLevel;
    hooks: DebugLevel;
    mcp: DebugLevel;
    tools: DebugLevel;
    discovery: DebugLevel;
  };
}

let cachedConfig: DebugConfig | null = null;

/**
 * Parse debug level from environment variable
 */
function parseDebugLevel(value: string | undefined): DebugLevel {
  if (!value) return 0;
  const level = parseInt(value, 10);
  if (level === 2) return 2;
  if (level === 1) return 1;
  return 0;
}

/**
 * Get debug configuration based on environment variables
 *
 * - GEN_DEBUG=0 or unset: No debug output (default)
 * - GEN_DEBUG=1: Standard debug output (loading, key operations)
 * - GEN_DEBUG=2: Verbose debug output (detailed execution, parameters)
 * - GEN_DEBUG_<COMPONENT>=1|2: Component-specific debug level
 */
export function getDebugConfig(): DebugConfig {
  if (cachedConfig) {
    return cachedConfig;
  }

  const globalLevel = parseDebugLevel(process.env.GEN_DEBUG);

  cachedConfig = {
    level: globalLevel,
    enabled: globalLevel >= 1,
    verbose: globalLevel >= 2,
    components: {
      skills: parseDebugLevel(process.env.GEN_DEBUG_SKILLS) || globalLevel,
      commands: parseDebugLevel(process.env.GEN_DEBUG_COMMANDS) || globalLevel,
      subagents: parseDebugLevel(process.env.GEN_DEBUG_SUBAGENTS) || globalLevel,
      hooks: parseDebugLevel(process.env.GEN_DEBUG_HOOKS) || globalLevel,
      mcp: parseDebugLevel(process.env.GEN_DEBUG_MCP) || globalLevel,
      tools: parseDebugLevel(process.env.GEN_DEBUG_TOOLS) || globalLevel,
      discovery: parseDebugLevel(process.env.GEN_DEBUG_DISCOVERY) || globalLevel,
    },
  };

  return cachedConfig;
}

/**
 * Check if debug is enabled for a specific component (level >= 1)
 */
export function isDebugEnabled(component: keyof DebugConfig['components']): boolean {
  const config = getDebugConfig();
  return config.components[component] >= 1;
}

/**
 * Check if verbose debug is enabled for a specific component (level >= 2)
 */
export function isVerboseDebugEnabled(component: keyof DebugConfig['components']): boolean {
  const config = getDebugConfig();
  return config.components[component] >= 2;
}

/**
 * Get debug level for a specific component
 */
export function getDebugLevel(component: keyof DebugConfig['components']): DebugLevel {
  const config = getDebugConfig();
  return config.components[component];
}

/**
 * Reset cached config (useful for testing)
 */
export function resetDebugConfig(): void {
  cachedConfig = null;
}
