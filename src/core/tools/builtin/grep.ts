/**
 * Grep Tool - Search for patterns in files
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import fastGlob from 'fast-glob';
import type { Tool, ToolResult } from '../types.js';
import { GrepInputSchema, type GrepInput, resolvePath, getErrorMessage } from '../types.js';
import { loadToolDescription } from '../../../cli/prompts/index.js';

const MAX_MATCHES = 50;

export const grepTool: Tool<GrepInput> = {
  name: 'Grep',
  description: loadToolDescription('grep'),
  parameters: GrepInputSchema,

  async execute(input, context): Promise<ToolResult> {
    try {
      const searchPath = input.path ? resolvePath(input.path, context.cwd) : context.cwd;
      const regex = new RegExp(input.pattern, 'gi');

      const stat = await fs.stat(searchPath).catch(() => null);
      const files = stat?.isFile()
        ? [searchPath]
        : await fastGlob(input.include || '**/*', {
            cwd: searchPath,
            absolute: true,
            onlyFiles: true,
            ignore: ['**/node_modules/**', '**/.git/**', '**/*.min.*'],
            followSymbolicLinks: false,
          });

      const results: string[] = [];

      for (const file of files) {
        if (results.length >= MAX_MATCHES) break;

        try {
          const content = await fs.readFile(file, 'utf-8');
          const lines = content.split('\n');

          for (let i = 0; i < lines.length && results.length < MAX_MATCHES; i++) {
            if (regex.test(lines[i])) {
              results.push(`${path.relative(context.cwd, file)}:${i + 1}: ${lines[i].trim()}`);
            }
            regex.lastIndex = 0;
          }
        } catch {
          // Skip unreadable files
        }
      }

      if (results.length === 0) {
        return { success: true, output: 'No matches found.' };
      }

      const truncated = results.length >= MAX_MATCHES;
      return {
        success: true,
        output: `Found ${results.length} match(es):\n${results.join('\n')}${truncated ? '\n... (truncated)' : ''}`,
      };
    } catch (error) {
      return { success: false, output: '', error: `Grep search failed: ${getErrorMessage(error)}` };
    }
  },
};
