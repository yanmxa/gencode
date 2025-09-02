/**
 * Glob Tool - Find files matching a pattern
 */

import fastGlob from 'fast-glob';
import * as path from 'path';
import type { Tool, ToolContext, ToolResult } from '../types.js';
import { GlobInputSchema, type GlobInput } from '../types.js';

export const globTool: Tool<GlobInput> = {
  name: 'Glob',
  description: 'Find files matching a glob pattern. Returns a list of matching file paths.',
  parameters: GlobInputSchema,

  async execute(input: GlobInput, context: ToolContext): Promise<ToolResult> {
    try {
      const searchPath = input.path
        ? path.isAbsolute(input.path)
          ? input.path
          : path.resolve(context.cwd, input.path)
        : context.cwd;

      const files = await fastGlob(input.pattern, {
        cwd: searchPath,
        absolute: true,
        onlyFiles: true,
        ignore: ['**/node_modules/**', '**/.git/**'],
        followSymbolicLinks: false,
      });

      if (files.length === 0) {
        return {
          success: true,
          output: 'No files found matching the pattern.',
        };
      }

      // Sort by modification time (newest first) - simplified version
      const sortedFiles = files.slice(0, 100); // Limit results

      return {
        success: true,
        output: `Found ${files.length} file(s):\n${sortedFiles.join('\n')}${files.length > 100 ? '\n... (truncated)' : ''}`,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      return {
        success: false,
        output: '',
        error: `Glob search failed: ${message}`,
      };
    }
  },
};
