/**
 * Configuration Levels - Path resolution for multi-level config
 *
 * Defines the configuration hierarchy and resolves paths for each level.
 * At each level, both .gen and .claude directories are loaded and merged,
 * with .gen taking higher priority.
 */

import * as path from 'path';
import * as os from 'os';
import {
  ConfigLevelType,
  GEN_DIR,
  CLAUDE_DIR,
  SETTINGS_FILE_NAME,
  SETTINGS_LOCAL_FILE_NAME,
  MANAGED_SETTINGS_FILE_NAME,
  GEN_CONFIG_ENV,
  getManagedPaths,
} from './types.js';
import { pathExists, findProjectRoot } from '../utils/path-utils.js';

// Re-export for backward compatibility
export { findProjectRoot };

/**
 * Configuration path info for a specific namespace
 */
export interface ConfigPathInfo {
  settingsPath: string;
  localSettingsPath?: string;
  dir: string;
  namespace: 'gen' | 'claude' | 'extra';
  exists: boolean;
}

/**
 * Configuration level with resolved paths
 */
export interface ResolvedLevel {
  type: ConfigLevelType;
  priority: number;
  paths: ConfigPathInfo[];
  description: string;
}

/**
 * Parse GEN_CONFIG environment variable
 */
export function parseExtraConfigDirs(): string[] {
  const value = process.env[GEN_CONFIG_ENV];
  if (!value) return [];

  return value
    .split(':')
    .map((dir) => dir.trim())
    .filter((dir) => dir.length > 0)
    .map((dir) => dir.replace(/^~/, os.homedir()));
}

/**
 * Get all configuration levels with resolved paths
 */
export async function getConfigLevels(cwd: string): Promise<ResolvedLevel[]> {
  const projectRoot = await findProjectRoot(cwd);
  const home = os.homedir();
  const managedPaths = getManagedPaths();
  const extraDirs = parseExtraConfigDirs();

  const levels: ResolvedLevel[] = [];

  // Level 1: User (lowest priority for settings, loaded first)
  const userPaths: ConfigPathInfo[] = [];

  // Claude first (lower priority within level)
  const userClaudeDir = path.join(home, CLAUDE_DIR);
  userPaths.push({
    settingsPath: path.join(userClaudeDir, SETTINGS_FILE_NAME),
    dir: userClaudeDir,
    namespace: 'claude',
    exists: await pathExists(path.join(userClaudeDir, SETTINGS_FILE_NAME)),
  });

  // GenCode second (higher priority within level)
  const userGencodeDir = path.join(home, GEN_DIR);
  userPaths.push({
    settingsPath: path.join(userGencodeDir, SETTINGS_FILE_NAME),
    dir: userGencodeDir,
    namespace: 'gen',
    exists: await pathExists(path.join(userGencodeDir, SETTINGS_FILE_NAME)),
  });

  levels.push({
    type: 'user',
    priority: 10,
    paths: userPaths,
    description: 'User global settings',
  });

  // Level 2: Extra config dirs from environment variable
  if (extraDirs.length > 0) {
    for (let i = 0; i < extraDirs.length; i++) {
      const dir = extraDirs[i];
      const extraPaths: ConfigPathInfo[] = [
        {
          settingsPath: path.join(dir, SETTINGS_FILE_NAME),
          dir,
          namespace: 'extra',
          exists: await pathExists(path.join(dir, SETTINGS_FILE_NAME)),
        },
      ];

      levels.push({
        type: 'extra',
        priority: 20 + i, // Each extra dir has slightly higher priority
        paths: extraPaths,
        description: `Extra config from ${dir}`,
      });
    }
  }

  // Level 3: Project (shared, committed to git)
  const projectPaths: ConfigPathInfo[] = [];

  // Claude first (lower priority within level)
  const projectClaudeDir = path.join(projectRoot, CLAUDE_DIR);
  projectPaths.push({
    settingsPath: path.join(projectClaudeDir, SETTINGS_FILE_NAME),
    dir: projectClaudeDir,
    namespace: 'claude',
    exists: await pathExists(path.join(projectClaudeDir, SETTINGS_FILE_NAME)),
  });

  // GenCode second (higher priority within level)
  const projectGencodeDir = path.join(projectRoot, GEN_DIR);
  projectPaths.push({
    settingsPath: path.join(projectGencodeDir, SETTINGS_FILE_NAME),
    dir: projectGencodeDir,
    namespace: 'gen',
    exists: await pathExists(path.join(projectGencodeDir, SETTINGS_FILE_NAME)),
  });

  levels.push({
    type: 'project',
    priority: 30,
    paths: projectPaths,
    description: 'Project shared settings',
  });

  // Level 4: Local (personal, gitignored)
  const localPaths: ConfigPathInfo[] = [];

  // Claude first (lower priority within level)
  localPaths.push({
    settingsPath: path.join(projectClaudeDir, SETTINGS_LOCAL_FILE_NAME),
    dir: projectClaudeDir,
    namespace: 'claude',
    exists: await pathExists(path.join(projectClaudeDir, SETTINGS_LOCAL_FILE_NAME)),
  });

  // GenCode second (higher priority within level)
  localPaths.push({
    settingsPath: path.join(projectGencodeDir, SETTINGS_LOCAL_FILE_NAME),
    dir: projectGencodeDir,
    namespace: 'gen',
    exists: await pathExists(path.join(projectGencodeDir, SETTINGS_LOCAL_FILE_NAME)),
  });

  levels.push({
    type: 'local',
    priority: 40,
    paths: localPaths,
    description: 'Local personal settings (gitignored)',
  });

  // Level 5: Managed (highest priority, enforced)
  const managedPathsList: ConfigPathInfo[] = [];

  // Claude first (lower priority within level)
  managedPathsList.push({
    settingsPath: path.join(managedPaths.claude, MANAGED_SETTINGS_FILE_NAME),
    dir: managedPaths.claude,
    namespace: 'claude',
    exists: await pathExists(path.join(managedPaths.claude, MANAGED_SETTINGS_FILE_NAME)),
  });

  // GenCode second (higher priority within level)
  managedPathsList.push({
    settingsPath: path.join(managedPaths.gen, MANAGED_SETTINGS_FILE_NAME),
    dir: managedPaths.gen,
    namespace: 'gen',
    exists: await pathExists(path.join(managedPaths.gen, MANAGED_SETTINGS_FILE_NAME)),
  });

  levels.push({
    type: 'managed',
    priority: 100, // Highest priority
    paths: managedPathsList,
    description: 'System-wide managed settings (enforced)',
  });

  // Sort by priority (ascending, so we merge from low to high)
  return levels.sort((a, b) => a.priority - b.priority);
}

/**
 * Get the primary settings directory for saving
 * Prefers .gen if it exists, otherwise creates it
 */
export function getPrimarySettingsDir(
  level: 'user' | 'project' | 'local',
  projectRoot: string
): string {
  const home = os.homedir();

  switch (level) {
    case 'user':
      return path.join(home, GEN_DIR);
    case 'project':
    case 'local':
      return path.join(projectRoot, GEN_DIR);
    default:
      return path.join(home, GEN_DIR);
  }
}

/**
 * Get the settings file path for a specific level
 */
export function getSettingsFilePath(
  level: 'user' | 'project' | 'local',
  projectRoot: string
): string {
  const dir = getPrimarySettingsDir(level, projectRoot);
  const fileName = level === 'local' ? SETTINGS_LOCAL_FILE_NAME : SETTINGS_FILE_NAME;
  return path.join(dir, fileName);
}
