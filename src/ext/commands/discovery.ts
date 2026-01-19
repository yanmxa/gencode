/**
 * Command Discovery
 *
 * Uses the unified resource discovery system to scan for command markdown files.
 * Precedence (lowest to highest):
 * 1. ~/.claude/commands/ (user-level Claude Code)
 * 2. ~/.gen/commands/ (user-level GenCode)
 * 3. .claude/commands/ (project-level Claude Code)
 * 4. .gen/commands/ (project-level GenCode)
 */

import { discoverResources } from '../../base/discovery/index.js';
import { CommandParser } from './parser.js';
import type { CommandDefinition } from './types.js';

/**
 * Command parser instance
 */
const commandParser = new CommandParser();

/**
 * Discover all commands with proper precedence handling
 *
 * Uses the unified discovery system to load commands from all configured
 * directories. Higher priority commands (gen > claude, project > user)
 * override lower priority ones with the same name.
 *
 * @param projectRoot Project root directory
 * @returns Map of command name -> definition
 */
export async function discoverCommands(
  projectRoot: string
): Promise<Map<string, CommandDefinition>> {
  return discoverResources(projectRoot, {
    resourceType: 'Command',
    subdirectory: 'commands',
    filePattern: { type: 'flat', extension: '.md' },
    parser: commandParser,
    levels: ['user', 'project'], // Commands only use user and project levels
  });
}

