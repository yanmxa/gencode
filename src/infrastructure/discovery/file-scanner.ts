/**
 * File Scanner - Scan directories for resource files
 *
 * Supports multiple file patterns:
 * - flat: Files directly in directory
 * - nested: Files in subdirectories
 * - multiple: Files with various extensions
 * - single: A single specific file
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import type { FilePattern } from './types.js';
import { pathExists } from '../utils/path-utils.js';

/**
 * Scan directory for files matching the pattern
 *
 * @param dirPath Absolute path to the directory to scan
 * @param pattern File pattern to match
 * @returns Array of absolute file paths
 */
export async function scanDirectory(
  dirPath: string,
  pattern: FilePattern
): Promise<string[]> {
  // Check if directory exists
  if (!(await pathExists(dirPath))) {
    return [];
  }

  switch (pattern.type) {
    case 'single':
      return scanSingleFile(dirPath, pattern.filename);

    case 'flat':
      return scanFlatDirectory(dirPath, pattern.extension);

    case 'nested':
      return scanNestedDirectory(dirPath, pattern.filename);

    case 'multiple':
      return scanMultipleExtensions(dirPath, pattern.extensions);

    default:
      return [];
  }
}

/**
 * Scan for a single specific file
 */
async function scanSingleFile(dirPath: string, filename: string): Promise<string[]> {
  const filePath = path.join(dirPath, filename);
  const exists = await pathExists(filePath);
  return exists ? [filePath] : [];
}

/**
 * Scan flat directory for files with specific extension
 */
async function scanFlatDirectory(
  dirPath: string,
  extension: string
): Promise<string[]> {
  try {
    const entries = await fs.readdir(dirPath, { withFileTypes: true });
    const files: string[] = [];

    for (const entry of entries) {
      if (entry.isFile() && entry.name.endsWith(extension)) {
        files.push(path.join(dirPath, entry.name));
      }
    }

    return files;
  } catch {
    return [];
  }
}

/**
 * Scan nested directory for files with specific filename
 */
async function scanNestedDirectory(
  dirPath: string,
  filename: string
): Promise<string[]> {
  try {
    const entries = await fs.readdir(dirPath, { withFileTypes: true });
    const files: string[] = [];

    for (const entry of entries) {
      if (entry.isDirectory()) {
        const filePath = path.join(dirPath, entry.name, filename);
        if (await pathExists(filePath)) {
          files.push(filePath);
        }
      }
    }

    return files;
  } catch {
    return [];
  }
}

/**
 * Scan directory for files with multiple extensions
 */
async function scanMultipleExtensions(
  dirPath: string,
  extensions: string[]
): Promise<string[]> {
  try {
    const entries = await fs.readdir(dirPath, { withFileTypes: true });
    const files: string[] = [];

    for (const entry of entries) {
      if (entry.isFile()) {
        const hasMatchingExtension = extensions.some((ext) =>
          entry.name.endsWith(ext)
        );
        if (hasMatchingExtension) {
          files.push(path.join(dirPath, entry.name));
        }
      }
    }

    return files;
  } catch {
    return [];
  }
}

/**
 * Extract resource name from file path based on pattern
 *
 * @param filePath Absolute file path
 * @param dirPath Directory path (for relative calculation)
 * @param pattern File pattern
 * @returns Resource name or null if extraction fails
 */
export function extractResourceName(
  filePath: string,
  dirPath: string,
  pattern: FilePattern
): string | null {
  const basename = path.basename(filePath);
  const relativePath = path.relative(dirPath, filePath);

  switch (pattern.type) {
    case 'single':
      // For single files, remove leading dot and .json extension
      return pattern.filename.replace(/^\./g, '').replace(/\.json$/g, '') || 'default';

    case 'flat':
      // For flat files, use filename without extension
      return basename.slice(0, -pattern.extension.length);

    case 'nested':
      // For nested files, use the parent directory name
      const parentDir = path.dirname(relativePath);
      return parentDir !== '.' ? parentDir : null;

    case 'multiple':
      // For multiple extensions, find and remove the matching extension
      for (const ext of pattern.extensions) {
        if (basename.endsWith(ext)) {
          return basename.slice(0, -ext.length);
        }
      }
      return null;

    default:
      return null;
  }
}
