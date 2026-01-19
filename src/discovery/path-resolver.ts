/**
 * Path Resolver - Unified path resolution for resource discovery
 *
 * Provides consistent path resolution across all resource types (Commands, Skills,
 * Subagents, MCP) with configurable levels and namespaces.
 *
 * Priority rules:
 * - Level: user < project < local < managed (ascending priority)
 * - Namespace: claude < gen (gen has higher priority within same level)
 */

import * as path from 'path';
import * as os from 'os';
import type { ResourceLevel, ResourceNamespace, ResourceDirectory } from './types.js';
import {
  GEN_DIR,
  CLAUDE_DIR,
  getManagedPaths,
  pathExists,
  findProjectRoot,
} from '../common/path-utils.js';

// Re-export for backward compatibility
export { GEN_DIR, CLAUDE_DIR, findProjectRoot };

/**
 * Get resource directories for discovery
 *
 * Returns directories in priority order (lowest first) for proper fallback:
 * - Lower priority directories are processed first
 * - Higher priority resources override lower priority ones
 *
 * @param projectRoot Project root directory
 * @param subdirectory Subdirectory name (e.g., "commands", "skills") or empty string for root
 * @param levels Which levels to include (default: ['user', 'project'])
 * @returns Array of resource directories in priority order
 */
export async function getResourceDirectories(
  projectRoot: string,
  subdirectory: string,
  levels: ResourceLevel[] = ['user', 'project']
): Promise<ResourceDirectory[]> {
  const home = os.homedir();
  const managedPaths = getManagedPaths();
  const directories: ResourceDirectory[] = [];

  // Level priority map
  const levelPriority: Record<ResourceLevel, number> = {
    user: 10,
    project: 30,
    local: 40,
    managed: 100,
  };

  // Process each requested level
  for (const level of levels) {
    const baseDirs: Array<{ dir: string; namespace: ResourceNamespace }> = [];

    // Determine base directories for this level
    switch (level) {
      case 'user':
        baseDirs.push(
          { dir: path.join(home, CLAUDE_DIR), namespace: 'claude' },
          { dir: path.join(home, GEN_DIR), namespace: 'gen' }
        );
        break;

      case 'project':
        baseDirs.push(
          { dir: path.join(projectRoot, CLAUDE_DIR), namespace: 'claude' },
          { dir: path.join(projectRoot, GEN_DIR), namespace: 'gen' }
        );
        break;

      case 'local':
        // Local level uses .local suffix directories for override purposes
        // These are git-ignored local-only configurations
        baseDirs.push(
          { dir: path.join(projectRoot, `${CLAUDE_DIR}.local`), namespace: 'claude' },
          { dir: path.join(projectRoot, `${GEN_DIR}.local`), namespace: 'gen' }
        );
        break;

      case 'managed':
        baseDirs.push(
          { dir: managedPaths.claude, namespace: 'claude' },
          { dir: managedPaths.gen, namespace: 'gen' }
        );
        break;
    }

    // Add subdirectory and create ResourceDirectory entries
    for (const { dir, namespace } of baseDirs) {
      const fullPath = subdirectory ? path.join(dir, subdirectory) : dir;
      const exists = await pathExists(fullPath);

      directories.push({
        path: fullPath,
        level,
        namespace,
        exists,
      });
    }
  }

  // Sort by level priority (ascending), then by namespace (claude < gen)
  directories.sort((a, b) => {
    const priorityDiff = levelPriority[a.level] - levelPriority[b.level];
    if (priorityDiff !== 0) return priorityDiff;

    // Within same level, claude < gen
    if (a.namespace === 'claude' && b.namespace === 'gen') return -1;
    if (a.namespace === 'gen' && b.namespace === 'claude') return 1;
    return 0;
  });

  return directories;
}

/**
 * Get only existing resource directories
 */
export async function getExistingResourceDirectories(
  projectRoot: string,
  subdirectory: string,
  levels: ResourceLevel[] = ['user', 'project']
): Promise<ResourceDirectory[]> {
  const all = await getResourceDirectories(projectRoot, subdirectory, levels);
  return all.filter((dir) => dir.exists === true);
}
