/**
 * Skills Parser - Parse SKILL.md files with YAML frontmatter
 *
 * Uses gray-matter to parse YAML frontmatter and extract skill metadata.
 */

import matter from 'gray-matter';
import * as fs from 'fs/promises';
import * as path from 'path';
import type { SkillDefinition } from './types.js';
import type { ResourceParser, ResourceLevel, ResourceNamespace } from '../discovery/types.js';
import { isValidResourceName } from '../shared/validation.js';
import { validateSkillFrontmatter } from '../shared/config-validator.js';

/**
 * Parse a SKILL.md file and return a SkillDefinition
 *
 * @param filePath - Absolute path to the SKILL.md file
 * @param level - Whether this is a user or project level skill
 * @param namespace - Whether this is from .gen or .claude directory
 * @returns Parsed skill definition
 * @throws Error if file is invalid or missing required fields
 */
export async function parseSkillFile(
  filePath: string,
  level: ResourceLevel,
  namespace: ResourceNamespace
): Promise<SkillDefinition> {
  try {
    const content = await fs.readFile(filePath, 'utf-8');
    const { data, content: body } = matter(content);

    // Validate frontmatter using Zod schema
    const validation = validateSkillFrontmatter(data, filePath);
    if (!validation.valid) {
      const errorList = validation.errors?.join(', ') || 'Unknown validation error';
      throw new Error(`Invalid SKILL.md frontmatter: ${errorList}`);
    }

    // Use validated data
    const validatedData = validation.data!;

    // Normalize allowed-tools to array
    let allowedTools: string[] | undefined;
    if (validatedData['allowed-tools']) {
      if (typeof validatedData['allowed-tools'] === 'string') {
        allowedTools = [validatedData['allowed-tools']];
      } else {
        allowedTools = validatedData['allowed-tools'];
      }
    }

    // Build skill definition using validated data
    const skill: SkillDefinition = {
      name: validatedData.name,
      description: validatedData.description,
      allowedTools,
      version: validatedData.version,
      author: validatedData.author,
      tags: validatedData.tags,
      content: body.trim(),
      directory: path.dirname(filePath),
      source: {
        path: filePath,
        level,
        namespace,
      },
    };

    return skill;
  } catch (error) {
    if (error instanceof Error) {
      throw error;
    }
    throw new Error(`Failed to parse SKILL.md at ${filePath}: ${String(error)}`);
  }
}

/**
 * Skill Parser - implements ResourceParser interface
 *
 * Adapts parseSkillFile to the unified discovery system.
 */
export class SkillParser implements ResourceParser<SkillDefinition> {
  async parse(
    filePath: string,
    level: ResourceLevel,
    namespace: ResourceNamespace
  ): Promise<SkillDefinition | null> {
    try {
      return await parseSkillFile(filePath, level, namespace);
    } catch (error) {
      console.warn(`Failed to parse skill file ${filePath}:`, error);
      return null;
    }
  }

  isValidName(name: string): boolean {
    // Skill names come from directory names, allow dash and underscore
    return isValidResourceName(name);
  }
}
