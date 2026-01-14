/**
 * Write Tool - Write content to a file
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import type { Tool, ToolResult } from '../types.js';
import { WriteInputSchema, type WriteInput, resolvePath, getErrorMessage } from '../types.js';

export const writeTool: Tool<WriteInput> = {
  name: 'Write',
  description: 'Write content to a file. Creates the file if it does not exist, overwrites if it does.',
  parameters: WriteInputSchema,

  async execute(input, context): Promise<ToolResult> {
    try {
      const filePath = resolvePath(input.file_path, context.cwd);
      await fs.mkdir(path.dirname(filePath), { recursive: true });
      await fs.writeFile(filePath, input.content, 'utf-8');

      const lineCount = input.content.split('\n').length;
      return { success: true, output: `Successfully wrote ${lineCount} lines to ${filePath}` };
    } catch (error) {
      return { success: false, output: '', error: `Failed to write file: ${getErrorMessage(error)}` };
    }
  },
};
