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
      const oldContent = await fs.readFile(filePath, 'utf-8');
      const occurrences = oldContent.split(input.old_string).length - 1;

      if (occurrences === 0) {
        return { success: false, output: '', error: 'The string to replace was not found in the file.' };
      }

      // Handle replace_all vs unique replacement
      const replaceAll = input.replace_all ?? false;

      if (!replaceAll && occurrences > 1) {
        return {
          success: false,
          output: '',
          error: `The string to replace occurs ${occurrences} times. Please provide a more unique string, or use replace_all: true to replace all occurrences.`,
        };
      }

      // Perform replacement (generate new content)
      let newContent: string;
      if (replaceAll) {
        // Replace all occurrences
        newContent = oldContent.split(input.old_string).join(input.new_string);
      } else {
        // Replace single occurrence
        newContent = oldContent.replace(input.old_string, input.new_string);
      }

      // Note: Permission with diff preview is now handled by the agent BEFORE tool execution
      // The agent computes the diff metadata and yields permission_request event
      // By the time we reach here, permission has already been granted

      // Write the file
      await fs.writeFile(filePath, newContent, 'utf-8');

      const oldLines = input.old_string.split('\n').length;
      const newLines = input.new_string.split('\n').length;
      const countInfo = replaceAll && occurrences > 1 ? ` (${occurrences} occurrences)` : '';
      return {
        success: true,
        output: `Successfully edited ${filePath}: replaced ${oldLines} line(s) with ${newLines} line(s)${countInfo}`,
      };
    } catch (error) {
      return { success: false, output: '', error: `Failed to edit file: ${getErrorMessage(error)}` };
    }
  },
};
