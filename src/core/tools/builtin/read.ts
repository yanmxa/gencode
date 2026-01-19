/**
 * Read Tool - Read file contents
 */

import * as fs from 'fs/promises';
import type { Tool, ToolResult } from '../types.js';
import { ReadInputSchema, type ReadInput, resolvePath, getErrorMessage } from '../types.js';
import { loadToolDescription } from '../../../cli/prompts/index.js';

export const readTool: Tool<ReadInput> = {
  name: 'Read',
  description: loadToolDescription('read'),
  parameters: ReadInputSchema,

  async execute(input, context): Promise<ToolResult> {
    try {
      const filePath = resolvePath(input.file_path, context.cwd);
      const content = await fs.readFile(filePath, 'utf-8');
      const lines = content.split('\n');

      const offset = input.offset ?? 1;
      const limit = input.limit ?? lines.length;

      const selectedLines = lines.slice(offset - 1, offset - 1 + limit);
      const numberedLines = selectedLines.map((line, i) => {
        const lineNum = (offset + i).toString().padStart(5, ' ');
        return `${lineNum}│${line}`;
      });

      return { success: true, output: numberedLines.join('\n') };
    } catch (error) {
      return { success: false, output: '', error: `Failed to read file: ${getErrorMessage(error)}` };
    }
  },
};
