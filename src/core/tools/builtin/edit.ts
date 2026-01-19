/**
 * Edit Tool - Edit file contents with string replacement
 */

import * as fs from 'fs/promises';
import type { Tool, ToolResult } from '../types.js';
import { EditInputSchema, type EditInput, resolvePath, getErrorMessage } from '../types.js';
import { loadToolDescription } from '../../../cli/prompts/index.js';

export const editTool: Tool<EditInput> = {
  name: 'Edit',
  description: loadToolDescription('edit'),
  parameters: EditInputSchema,

  async execute(input, context): Promise<ToolResult> {
    try {
      const filePath = resolvePath(input.file_path, context.cwd);
      const content = await fs.readFile(filePath, 'utf-8');
      const occurrences = content.split(input.old_string).length - 1;

      if (occurrences === 0) {
        return { success: false, output: '', error: 'The string to replace was not found in the file.' };
      }
      if (occurrences > 1) {
        return { success: false, output: '', error: `The string to replace occurs ${occurrences} times. Please provide a more unique string.` };
      }

      const newContent = content.replace(input.old_string, input.new_string);
      await fs.writeFile(filePath, newContent, 'utf-8');

      const oldLines = input.old_string.split('\n').length;
      const newLines = input.new_string.split('\n').length;
      return { success: true, output: `Successfully edited ${filePath}: replaced ${oldLines} line(s) with ${newLines} line(s)` };
    } catch (error) {
      return { success: false, output: '', error: `Failed to edit file: ${getErrorMessage(error)}` };
    }
  },
};
