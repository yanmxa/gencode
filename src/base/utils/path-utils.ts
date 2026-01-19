// src/common/path-utils.ts
import * as os from 'os';
import * as fs from 'fs/promises';
import * as path from 'path';

/**
 * Directory names for GenCode and Claude Code configurations
 */
export const GEN_DIR = '.gen';
export const CLAUDE_DIR = '.claude';

/**
 * Platform-specific managed paths
 */
export interface ManagedPaths {
  gen: string;
  claude: string;
}

/**
 * Get managed (system-level) configuration paths
 */
export function getManagedPaths(): ManagedPaths {
  const platform = os.platform();

  if (platform === 'darwin') {
    return {
      gen: '/Library/Application Support/GenCode',
      claude: '/Library/Application Support/ClaudeCode',
    };
  } else if (platform === 'win32') {
    return {
      gen: 'C:\\Program Files\\GenCode',
      claude: 'C:\\Program Files\\ClaudeCode',
    };
  } else {
    return {
      gen: '/usr/share/gencode',
      claude: '/usr/share/claude-code',
    };
  }
}

/**
 * Check if a path exists
 */
export async function pathExists(filePath: string): Promise<boolean> {
  try {
    await fs.access(filePath);
    return true;
  } catch {
    return false;
  }
}

/**
 * Find project root by looking for .git, .gen, or .claude directory
 */
export async function findProjectRoot(cwd: string): Promise<string> {
  let current = path.resolve(cwd);
  const root = path.parse(current).root;

  while (current !== root) {
    // Check for git directory
    if (await pathExists(path.join(current, '.git'))) {
      return current;
    }

    // Check for .gen or .claude directories
    if (await pathExists(path.join(current, GEN_DIR))) {
      return current;
    }

    if (await pathExists(path.join(current, CLAUDE_DIR))) {
      return current;
    }

    const parent = path.dirname(current);
    if (parent === current) break;
    current = parent;
  }

  return cwd;
}
