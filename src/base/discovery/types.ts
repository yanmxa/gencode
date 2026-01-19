/**
 * Unified Resource Discovery System - Core Types
 *
 * This module provides the foundational types for a unified resource discovery
 * system that eliminates code duplication across Commands, Skills, Subagents, and MCP.
 *
 * All resources follow the same merge strategy:
 * - Load from all levels and namespaces
 * - Priority: user < project < local < managed
 * - Within each level: claude < gen
 * - Higher priority resources override lower priority ones (by name)
 */

/**
 * Resource level - hierarchical configuration levels
 */
export type ResourceLevel = 'user' | 'project' | 'local' | 'managed';

/**
 * Resource namespace - identifies the config system (.gen or .claude)
 */
export type ResourceNamespace = 'gen' | 'claude';

/**
 * Source metadata for discovered resources
 *
 * Tracks where a resource was loaded from, following the priority:
 * .gen > .claude and project > user
 */
export interface ResourceSource {
  /** User or project level */
  level: ResourceLevel;

  /** Gen or Claude namespace */
  namespace: ResourceNamespace;

  /** Absolute path to the resource file */
  path: string;
}

/**
 * Base interface for all discoverable resources
 *
 * Any resource type (Command, Skill, Subagent) must extend this interface
 * to be compatible with the unified discovery system.
 */
export interface DiscoverableResource {
  /** Unique resource name */
  name: string;

  /** Source metadata */
  source: ResourceSource;
}

/**
 * File pattern matching strategy
 */
export type FilePattern =
  | {
      /** Flat structure - files directly in directory (e.g., commands/*.md) */
      type: 'flat';
      /** File extension including dot (e.g., '.md') */
      extension: string;
    }
  | {
      /** Nested structure - files in subdirectories (e.g., skills/star/SKILL.md) */
      type: 'nested';
      /** Exact filename to match (e.g., 'SKILL.md') */
      filename: string;
    }
  | {
      /** Multiple extensions - supports various file types (e.g., agents/star.{json,md}) */
      type: 'multiple';
      /** Array of extensions including dots (e.g., ['.json', '.md']) */
      extensions: string[];
    }
  | {
      /** Single file - a specific file at the root (e.g., .mcp.json) */
      type: 'single';
      /** Exact filename (e.g., '.mcp.json') */
      filename: string;
    };

/**
 * Resource parser interface
 *
 * Each resource type must implement this interface to handle
 * file parsing and validation.
 */
export interface ResourceParser<T extends DiscoverableResource> {
  /**
   * Parse a resource file
   *
   * @param filePath Absolute path to the resource file
   * @param level Resource level (user or project)
   * @param namespace Resource namespace (gen or claude)
   * @returns Parsed resource or null if parsing fails
   */
  parse(
    filePath: string,
    level: ResourceLevel,
    namespace: ResourceNamespace
  ): Promise<T | null>;

  /**
   * Validate a resource name
   *
   * @param name Resource name to validate
   * @returns true if the name is valid
   */
  isValidName(name: string): boolean;
}

/**
 * Discovery configuration
 *
 * Configures the unified discovery system for a specific resource type.
 */
export interface DiscoveryConfig<T extends DiscoverableResource> {
  /** Resource type name for logging (e.g., "Command", "Skill", "MCP Server") */
  resourceType: string;

  /** Subdirectory name within .gen/.claude (e.g., "commands", "skills"), or empty for root */
  subdirectory: string;

  /** File pattern matching strategy */
  filePattern: FilePattern;

  /** Parser implementation for this resource type */
  parser: ResourceParser<T>;

  /** Which levels to include (default: ['user', 'project']) */
  levels?: ResourceLevel[];
}

/**
 * Resource directory metadata
 *
 * Used internally by the discovery system to track search paths.
 */
export interface ResourceDirectory {
  /** Absolute path to the directory */
  path: string;

  /** Resource level */
  level: ResourceLevel;

  /** Resource namespace */
  namespace: ResourceNamespace;

  /** Whether the directory exists */
  exists?: boolean;
}
