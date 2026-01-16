/**
 * Import Resolver - Handle @import syntax in memory files
 *
 * Supports:
 * - @path/to/file.md - Relative imports
 * - @./file.md - Current directory imports
 * - @../parent/file.md - Parent directory imports
 *
 * Security:
 * - Circular import detection
 * - Max depth limit (5 levels)
 * - Path traversal protection
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import type { MemoryConfig } from './types.js';

export interface ImportResult {
  content: string;
  importedPaths: string[];
  errors: string[];
}

export class ImportResolver {
  private config: MemoryConfig;
  private resolvedPaths: Set<string> = new Set();
  private projectRoot: string = '';

  constructor(config: MemoryConfig) {
    this.config = config;
  }

  /**
   * Set the project root for path traversal protection
   */
  setProjectRoot(root: string): void {
    this.projectRoot = root;
  }

  /**
   * Resolve all @imports in content
   */
  async resolve(
    content: string,
    basePath: string,
    depth: number = 0
  ): Promise<ImportResult> {
    const importedPaths: string[] = [];
    const errors: string[] = [];

    if (depth >= this.config.maxImportDepth) {
      errors.push(`Max import depth (${this.config.maxImportDepth}) exceeded`);
      return { content, importedPaths, errors };
    }

    // Match @import lines: lines starting with @ followed by a path
    // Pattern: @path/to/file or @./file or @../file
    const importRegex = /^@([^\s]+\.md)$/gm;
    let resolvedContent = content;
    const matches = [...content.matchAll(importRegex)];

    for (const match of matches) {
      const importPath = match[1].trim();
      const absolutePath = this.resolvePath(importPath, basePath);

      // Circular import detection
      if (this.resolvedPaths.has(absolutePath)) {
        errors.push(`Circular import detected: ${importPath}`);
        resolvedContent = resolvedContent.replace(match[0], `<!-- Circular import: ${importPath} -->`);
        continue;
      }

      // Path traversal security check
      if (!this.isPathSafe(absolutePath)) {
        errors.push(`Import path outside allowed scope: ${importPath}`);
        resolvedContent = resolvedContent.replace(match[0], `<!-- Blocked import: ${importPath} -->`);
        continue;
      }

      try {
        const stat = await fs.stat(absolutePath);

        // Size limit check
        if (stat.size > this.config.maxFileSize) {
          errors.push(`Import too large (${Math.round(stat.size / 1024)}KB): ${importPath}`);
          resolvedContent = resolvedContent.replace(match[0], `<!-- Import too large: ${importPath} -->`);
          continue;
        }

        this.resolvedPaths.add(absolutePath);
        let importedContent = await fs.readFile(absolutePath, 'utf-8');
        importedPaths.push(absolutePath);

        // Recursively resolve imports in the imported file
        const nested = await this.resolve(importedContent, path.dirname(absolutePath), depth + 1);

        importedContent = nested.content;
        importedPaths.push(...nested.importedPaths);
        errors.push(...nested.errors);

        // Replace @import with content
        resolvedContent = resolvedContent.replace(match[0], importedContent);
      } catch {
        errors.push(`Failed to import: ${importPath}`);
        resolvedContent = resolvedContent.replace(match[0], `<!-- Failed to import: ${importPath} -->`);
      }
    }

    return { content: resolvedContent, importedPaths, errors };
  }

  /**
   * Resolve import path to absolute path
   */
  private resolvePath(importPath: string, basePath: string): string {
    if (path.isAbsolute(importPath)) {
      return importPath;
    }
    return path.resolve(basePath, importPath);
  }

  /**
   * Check if path is safe (within project or home directory)
   */
  private isPathSafe(targetPath: string): boolean {
    const home = process.env.HOME || '';
    const normalized = path.normalize(targetPath);

    // Allow imports within project root
    if (this.projectRoot && normalized.startsWith(this.projectRoot)) {
      return true;
    }

    // Allow imports from home directory (for user-level imports)
    if (home && normalized.startsWith(home)) {
      return true;
    }

    return false;
  }

  /**
   * Reset resolver state for new file
   */
  reset(): void {
    this.resolvedPaths.clear();
  }
}
