/**
 * Rules Parser - Parse frontmatter from rules files
 *
 * Uses gray-matter for YAML frontmatter parsing.
 * Supports 'paths:' frontmatter for glob-based rule scoping.
 *
 * Example rule file:
 * ---
 * paths:
 *   - "src/api/**\/*.ts"
 *   - "src/routes/**\/*.ts"
 * ---
 *
 * # API Development Rules
 * - Use async/await consistently
 */

import matter from 'gray-matter';
import { minimatch } from 'minimatch';
import type { MemoryRule } from './types.js';

export interface ParsedRule {
  paths: string[];
  content: string;
}

/**
 * Parse frontmatter from a rule file
 */
export function parseRuleFrontmatter(fileContent: string): ParsedRule {
  try {
    const { data, content } = matter(fileContent);

    // Extract paths from frontmatter
    let paths: string[] = [];
    if (data.paths) {
      if (Array.isArray(data.paths)) {
        paths = data.paths.filter((p: unknown) => typeof p === 'string');
      } else if (typeof data.paths === 'string') {
        paths = [data.paths];
      }
    }

    return { paths, content: content.trim() };
  } catch {
    // If frontmatter parsing fails, return full content with no paths
    return { paths: [], content: fileContent };
  }
}

/**
 * Check if a file path matches any of the glob patterns
 */
export function matchesPatterns(filePath: string, patterns: string[]): boolean {
  if (patterns.length === 0) {
    return true; // No patterns = always active
  }

  return patterns.some((pattern) =>
    minimatch(filePath, pattern, {
      matchBase: true,
      dot: true,
    })
  );
}

/**
 * Activate rules based on current file context
 */
export function activateRules(rules: MemoryRule[], currentFile?: string): MemoryRule[] {
  return rules.map((rule) => ({
    ...rule,
    isActive: currentFile ? matchesPatterns(currentFile, rule.patterns) : rule.patterns.length === 0,
  }));
}

/**
 * Get only active rules
 */
export function getActiveRules(rules: MemoryRule[]): MemoryRule[] {
  return rules.filter((rule) => rule.isActive);
}
