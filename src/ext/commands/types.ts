/**
 * Custom Commands System Types
 *
 * Defines types for markdown-based slash commands with YAML frontmatter,
 * compatible with Claude Code and OpenCode formats.
 */

import type { DiscoverableResource, ResourceSource } from '../../base/discovery/types.js';

export interface CommandDefinition extends DiscoverableResource {
  /** Command name (from filename without .md extension) */
  name: string;

  /** Optional description from frontmatter */
  description?: string;

  /** Optional argument hint (e.g., "<file> <message>") */
  argumentHint?: string;

  /** Pre-authorized tools (simple names or patterns like "Bash(gh:*)") */
  allowedTools?: string[];

  /** Optional model override */
  model?: string;

  /** Template body (after frontmatter) */
  content: string;

  /** Source metadata (path, level, namespace) */
  source: ResourceSource;
}

export interface ParsedCommand {
  /** Original command definition */
  definition: CommandDefinition;

  /** Template with variables and file includes expanded */
  expandedPrompt: string;

  /** Tools pre-authorized by this command */
  preAuthorizedTools: string[];

  /** Model override if specified */
  modelOverride?: string;
}

export interface ExpansionContext {
  /** Full argument string as provided by user */
  arguments: string;

  /** Arguments split into positional array */
  positionalArgs: string[];

  /** Project root for @file security checks */
  projectRoot: string;
}

/**
 * Frontmatter schema for command markdown files
 */
export interface CommandFrontmatter {
  description?: string;
  'argument-hint'?: string;
  'allowed-tools'?: string | string[];
  model?: string;
}
