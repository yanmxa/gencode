/**
 * Glob Tool - Find files matching a pattern
 */

import fastGlob from 'fast-glob';
import type { Tool, ToolResult } from '../types.js';
import { GlobInputSchema, type GlobInput, resolvePath, getErrorMessage } from '../types.js';
import { loadToolDescription } from '../../../cli/prompts/index.js';

const MAX_RESULTS = 100;

export const globTool: Tool<GlobInput> = {
  name: 'Glob',
  description: loadToolDescription('glob'),
  parameters: GlobInputSchema,

  async execute(input, context): Promise<ToolResult> {
    try {
      const searchPath = input.path ? resolvePath(input.path, context.cwd) : context.cwd;

      const files = await fastGlob(input.pattern, {
        cwd: searchPath,
        absolute: true,
        onlyFiles: true,
        ignore: ['**/node_modules/**', '**/.git/**'],
        followSymbolicLinks: false,
      });

      if (files.length === 0) {
        return { success: true, output: 'No files found matching the pattern.' };
      }

      const truncated = files.length > MAX_RESULTS;
      const displayFiles = files.slice(0, MAX_RESULTS);
      return {
        success: true,
        output: `Found ${files.length} file(s):\n${displayFiles.join('\n')}${truncated ? '\n... (truncated)' : ''}`,
      };
    } catch (error) {
      return { success: false, output: '', error: `Glob search failed: ${getErrorMessage(error)}` };
    }
  },
};
