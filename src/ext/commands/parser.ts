/**
 * Command Parser
 *
 * Parses markdown files with YAML frontmatter into CommandDefinition objects.
 * Compatible with both Claude Code and OpenCode command formats.
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import matter from 'gray-matter';
import type { CommandDefinition, CommandFrontmatter } from './types.js';
import type { ResourceParser, ResourceLevel, ResourceNamespace } from '../../base/discovery/types.js';
import { isValidResourceName } from '../../base/utils/validation.js';
import { validateCommandFrontmatter } from '../../base/utils/config-validator.js';

/**
 * Parse a command markdown file
 *
 * @param filePath - Absolute path to the .md file
 * @param level - Command level (user or project)
 * @param namespace - Namespace (gen or claude)
 * @returns Parsed command definition
 */
export async function parseCommandFile(
  filePath: string,
  level: ResourceLevel,
  namespace: ResourceNamespace
): Promise<CommandDefinition> {
  const fileContent = await fs.readFile(filePath, 'utf-8');
  const { data, content } = matter(fileContent);

  // Validate frontmatter using Zod schema
  const validation = validateCommandFrontmatter(data, filePath);
  if (!validation.valid) {
    const errorList = validation.errors?.join(', ') || 'Unknown validation error';
    throw new Error(`Invalid command frontmatter: ${errorList}`);
  }

  const frontmatter = validation.data!;

  // Extract command name from filename (without .md extension)
  const name = path.basename(filePath, '.md');

  // Normalize allowed-tools to array
  let allowedTools: string[] | undefined;
  if (frontmatter['allowed-tools']) {
    if (typeof frontmatter['allowed-tools'] === 'string') {
      // Single tool as string
      allowedTools = [frontmatter['allowed-tools']];
    } else if (Array.isArray(frontmatter['allowed-tools'])) {
      allowedTools = frontmatter['allowed-tools'];
    }
  }

  return {
    name,
    description: frontmatter.description,
    argumentHint: frontmatter['argument-hint'],
    allowedTools,
    model: frontmatter.model,
    content: content.trim(),
    source: {
      path: filePath,
      level,
      namespace,
    },
  };
}

/**
 * Validate command name (no special characters except dash/underscore)
 */
export function isValidCommandName(name: string): boolean {
  return isValidResourceName(name);
}

/**
 * Command Parser - implements ResourceParser interface
 *
 * Adapts parseCommandFile to the unified discovery system.
 */
export class CommandParser implements ResourceParser<CommandDefinition> {
  async parse(
    filePath: string,
    level: ResourceLevel,
    namespace: ResourceNamespace
  ): Promise<CommandDefinition | null> {
    try {
      return await parseCommandFile(filePath, level, namespace);
    } catch (error) {
      console.warn(`Failed to parse command file ${filePath}:`, error);
      return null;
    }
  }

  isValidName(name: string): boolean {
    return isValidCommandName(name);
  }
}
