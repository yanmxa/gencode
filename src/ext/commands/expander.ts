/**
 * Template Expander
 *
 * Handles variable expansion ($ARGUMENTS, $1, $2, etc.) and
 * file inclusion (@file) with security checks.
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import type { ExpansionContext } from './types.js';

/**
 * Parse argument string into positional array
 * Respects quoted strings: "arg with spaces" -> single argument
 */
export function parseArguments(argString: string): string[] {
  if (!argString.trim()) {
    return [];
  }

  const args: string[] = [];
  let current = '';
  let inQuotes = false;
  let quoteChar = '';

  for (let i = 0; i < argString.length; i++) {
    const char = argString[i];
    const prevChar = i > 0 ? argString[i - 1] : '';

    if ((char === '"' || char === "'") && prevChar !== '\\') {
      if (!inQuotes) {
        inQuotes = true;
        quoteChar = char;
      } else if (char === quoteChar) {
        inQuotes = false;
        quoteChar = '';
      } else {
        current += char;
      }
    } else if (char === ' ' && !inQuotes) {
      if (current) {
        args.push(current);
        current = '';
      }
    } else {
      current += char;
    }
  }

  if (current) {
    args.push(current);
  }

  return args;
}

/**
 * Validate file path for security
 * Prevents path traversal and ensures file is within project root
 */
function validateFilePath(filePath: string, projectRoot: string): {
  valid: boolean;
  resolvedPath?: string;
  error?: string;
} {
  // Reject paths with ../
  if (filePath.includes('..')) {
    return {
      valid: false,
      error: `Path traversal not allowed: ${filePath}`,
    };
  }

  // Resolve relative to project root
  const resolvedPath = path.resolve(projectRoot, filePath);

  // Ensure resolved path is within project root
  if (!resolvedPath.startsWith(projectRoot)) {
    return {
      valid: false,
      error: `File must be within project root: ${filePath}`,
    };
  }

  return { valid: true, resolvedPath };
}

/**
 * Expand @file includes in template
 * Security: Only allows files within project root, no path traversal
 */
async function expandFileIncludes(
  template: string,
  projectRoot: string
): Promise<string> {
  // Match @filepath patterns (not @word in middle of text)
  const filePattern = /@([^\s]+)/g;
  const matches = Array.from(template.matchAll(filePattern));

  if (matches.length === 0) {
    return template;
  }

  let result = template;

  for (const match of matches) {
    const fullMatch = match[0]; // @filepath
    const filePath = match[1]; // filepath

    // Validate path
    const validation = validateFilePath(filePath, projectRoot);

    if (!validation.valid) {
      // Replace with error message
      const errorMsg = `[Error: ${validation.error}]`;
      result = result.replace(fullMatch, errorMsg);
      continue;
    }

    // Read file content
    try {
      const content = await fs.readFile(validation.resolvedPath!, 'utf-8');
      result = result.replace(fullMatch, content);
    } catch (error: any) {
      // Replace with error message
      const errorMsg = `[Error reading ${filePath}: ${error.message}]`;
      result = result.replace(fullMatch, errorMsg);
    }
  }

  return result;
}

/**
 * Expand template with variables and file includes
 *
 * Variables:
 * - $ARGUMENTS: Full argument string
 * - $1, $2, $3...: Positional arguments
 * - $GEN_CONFIG_DIR: Base config directory (e.g., ~/.claude, ~/.gen)
 *
 * File includes:
 * - @filepath: Replace with file content (with security checks)
 */
export async function expandTemplate(
  template: string,
  context: ExpansionContext
): Promise<string> {
  let result = template;

  // 1. Replace $ARGUMENTS
  result = result.replace(/\$ARGUMENTS/g, context.arguments);

  // 2. Replace $1, $2, etc. (positional arguments)
  context.positionalArgs.forEach((arg, index) => {
    const pattern = new RegExp(`\\$${index + 1}`, 'g');
    result = result.replace(pattern, arg);
  });

  // 3. Replace environment variables ($CLAUDE_DIR, $SCRIPT_DIR, etc.)
  for (const [key, value] of Object.entries(context.env)) {
    const pattern = new RegExp(`\\$${key}`, 'g');
    result = result.replace(pattern, value);
  }

  // 4. Expand @file includes (async)
  result = await expandFileIncludes(result, context.projectRoot);

  return result;
}
