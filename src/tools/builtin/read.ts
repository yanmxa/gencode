/**
 * Read Tool - Read file contents
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import type { Tool, ToolContext, ToolResult } from '../types.js';
import { ReadInputSchema, type ReadInput } from '../types.js';

export const readTool: Tool<ReadInput> = {
  name: 'Read',
  description: 'Read the contents of a file. Returns the file content with line numbers.',
  parameters: ReadInputSchema,

  async execute(input: ReadInput, context: ToolContext): Promise<ToolResult> {
    try {
      const filePath = path.isAbsolute(input.file_path)
        ? input.file_path
        : path.resolve(context.cwd, input.file_path);

      const content = await fs.readFile(filePath, 'utf-8');
      const lines = content.split('\n');

      const offset = input.offset ?? 1;
      const limit = input.limit ?? lines.length;

      const selectedLines = lines.slice(offset - 1, offset - 1 + limit);
      const numberedLines = selectedLines.map((line, i) => {
        const lineNum = (offset + i).toString().padStart(5, ' ');
        return `${lineNum}│${line}`;
      });

      return {
        success: true,
        output: numberedLines.join('\n'),
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      return {
        success: false,
        output: '',
        error: `Failed to read file: ${message}`,
      };
    }
  },
};
