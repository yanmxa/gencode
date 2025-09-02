/**
 * Edit Tool - Edit file contents with string replacement
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import type { Tool, ToolContext, ToolResult } from '../types.js';
import { EditInputSchema, type EditInput } from '../types.js';

export const editTool: Tool<EditInput> = {
  name: 'Edit',
  description: 'Edit a file by replacing a specific string with another. The old_string must be unique in the file.',
  parameters: EditInputSchema,

  async execute(input: EditInput, context: ToolContext): Promise<ToolResult> {
    try {
      const filePath = path.isAbsolute(input.file_path)
        ? input.file_path
        : path.resolve(context.cwd, input.file_path);

      const content = await fs.readFile(filePath, 'utf-8');

      // Check if old_string exists and is unique
      const occurrences = content.split(input.old_string).length - 1;

      if (occurrences === 0) {
        return {
          success: false,
          output: '',
          error: `The string to replace was not found in the file.`,
        };
      }

      if (occurrences > 1) {
        return {
          success: false,
          output: '',
          error: `The string to replace occurs ${occurrences} times. Please provide a more unique string.`,
        };
      }

      // Perform replacement
      const newContent = content.replace(input.old_string, input.new_string);
      await fs.writeFile(filePath, newContent, 'utf-8');

      // Calculate diff info
      const oldLines = input.old_string.split('\n').length;
      const newLines = input.new_string.split('\n').length;

      return {
        success: true,
        output: `Successfully edited ${filePath}: replaced ${oldLines} line(s) with ${newLines} line(s)`,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      return {
        success: false,
        output: '',
        error: `Failed to edit file: ${message}`,
      };
    }
  },
};
