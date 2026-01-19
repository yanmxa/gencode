/**
 * Skills System Types
 *
 * Defines types for the skills system that allows loading domain-specific
 * knowledge and workflows from SKILL.md files.
 */

import type { DiscoverableResource, ResourceSource } from '../../base/discovery/types.js';

/**
 * Skill metadata from YAML frontmatter
 */
export interface SkillMetadata {
  name: string;
  description: string;
  allowedTools?: string[];
  version?: string;
  author?: string;
  tags?: string[];
}

/**
 * Complete skill definition with content and source info
 */
export interface SkillDefinition extends SkillMetadata, DiscoverableResource {
  content: string; // Full markdown body (after frontmatter)
  directory: string; // Parent directory (for resources/ if needed)
  source: ResourceSource; // Source metadata (path, level, namespace)
}

/**
 * Input for the Skill tool
 */
export interface SkillInput {
  skill: string; // Skill name to activate
  args?: string; // Optional arguments to pass to the skill
}
