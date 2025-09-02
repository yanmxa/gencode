/**
 * Grep Tool - Search for patterns in files
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import fastGlob from 'fast-glob';
import type { Tool, ToolContext, ToolResult } from '../types.js';
import { GrepInputSchema, type GrepInput } from '../types.js';

export const grepTool: Tool<GrepInput> = {
  name: 'Grep',
  description: 'Search for a regex pattern in files. Returns matching lines with file paths and line numbers.',
  parameters: GrepInputSchema,

  async execute(input: GrepInput, context: ToolContext): Promise<ToolResult> {
    try {
      const searchPath = input.path
        ? path.isAbsolute(input.path)
          ? input.path
          : path.resolve(context.cwd, input.path)
        : context.cwd;

      const regex = new RegExp(input.pattern, 'gi');

      // Get files to search
      let files: string[];
      const stat = await fs.stat(searchPath).catch(() => null);

      if (stat?.isFile()) {
        files = [searchPath];
      } else {
        const pattern = input.include || '**/*';
        files = await fastGlob(pattern, {
          cwd: searchPath,
          absolute: true,
          onlyFiles: true,
          ignore: ['**/node_modules/**', '**/.git/**', '**/*.min.*'],
          followSymbolicLinks: false,
        });
      }

      const results: string[] = [];
      let matchCount = 0;
      const maxMatches = 50;

      for (const file of files) {
        if (matchCount >= maxMatches) break;

        try {
          const content = await fs.readFile(file, 'utf-8');
          const lines = content.split('\n');

          for (let i = 0; i < lines.length; i++) {
            if (matchCount >= maxMatches) break;

            if (regex.test(lines[i])) {
              const relPath = path.relative(context.cwd, file);
              results.push(`${relPath}:${i + 1}: ${lines[i].trim()}`);
              matchCount++;
            }
            // Reset regex lastIndex for global flag
            regex.lastIndex = 0;
          }
        } catch {
          // Skip files that can't be read (binary, etc.)
        }
      }

      if (results.length === 0) {
        return {
          success: true,
          output: 'No matches found.',
        };
      }

      return {
        success: true,
        output: `Found ${matchCount} match(es):\n${results.join('\n')}${matchCount >= maxMatches ? '\n... (truncated)' : ''}`,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      return {
        success: false,
        output: '',
        error: `Grep search failed: ${message}`,
      };
    }
  },
};
