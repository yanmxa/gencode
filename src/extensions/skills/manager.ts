/**
 * Skills Discovery - Scan directories and discover SKILL.md files
 *
 * Uses the unified resource discovery system with nested file pattern.
 * Implements merge strategy: loads skills from all directories (user/project, gen/claude)
 * and merges them with priority: project gen > project claude > user gen > user claude
 */

import { ResourceManager } from '../../base/discovery/index.js';
import { SkillParser } from './parser.js';
import type { SkillDefinition } from './types.js';

/**
 * Discovers and manages skills from hierarchical directories
 *
 * Uses the unified discovery system with nested file pattern (SKILL.md in subdirectories).
 */
export class SkillDiscovery extends ResourceManager<SkillDefinition> {
  constructor(options?: { projectOnly?: boolean }) {
    super({
      resourceType: 'Skill',
      subdirectory: 'skills',
      filePattern: { type: 'nested', filename: 'SKILL.md' },
      parser: new SkillParser(),
      // Allow tests to skip user-level skills
      levels: options?.projectOnly ? ['project'] : ['user', 'project'],
    });
  }
}
