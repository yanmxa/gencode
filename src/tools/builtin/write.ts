/**
 * Write Tool - Write content to a file
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import type { Tool, ToolContext, ToolResult } from '../types.js';
import { WriteInputSchema, type WriteInput } from '../types.js';

export const writeTool: Tool<WriteInput> = {
  name: 'Write',
  description: 'Write content to a file. Creates the file if it does not exist, overwrites if it does.',
  parameters: WriteInputSchema,

  async execute(input: WriteInput, context: ToolContext): Promise<ToolResult> {
    try {
      const filePath = path.isAbsolute(input.file_path)
        ? input.file_path
        : path.resolve(context.cwd, input.file_path);

      // Ensure directory exists
      const dir = path.dirname(filePath);
      await fs.mkdir(dir, { recursive: true });

      await fs.writeFile(filePath, input.content, 'utf-8');

      const lineCount = input.content.split('\n').length;

      return {
        success: true,
        output: `Successfully wrote ${lineCount} lines to ${filePath}`,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      return {
        success: false,
        output: '',
        error: `Failed to write file: ${message}`,
      };
    }
  },
};
