/**
 * Write Tool - Write content to a file
 *
 * In plan mode, this tool is only allowed to write to the plan file.
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import type { Tool, ToolResult } from '../types.js';
import { WriteInputSchema, type WriteInput, resolvePath, getErrorMessage } from '../types.js';
import { loadToolDescription } from '../../../cli/prompts/index.js';
import { getPlanModeManager } from '../../../cli/planning/index.js';

export const writeTool: Tool<WriteInput> = {
  name: 'Write',
  description: loadToolDescription('write'),
  parameters: WriteInputSchema,

  async execute(input, context): Promise<ToolResult> {
    try {
      const filePath = resolvePath(input.file_path, context.cwd);

      // Plan mode check: only allow writing to the plan file
      const planManager = getPlanModeManager();
      if (planManager.isActive()) {
        const planFilePath = planManager.getPlanFilePath();
        if (!planFilePath || path.resolve(filePath) !== path.resolve(planFilePath)) {
          return {
            success: false,
            output: '',
            error: `Write is blocked in plan mode. You can only write to the plan file: ${planFilePath ?? '(no plan file set)'}`,
          };
        }
      }

      await fs.mkdir(path.dirname(filePath), { recursive: true });
      await fs.writeFile(filePath, input.content, 'utf-8');

      const lineCount = input.content.split('\n').length;
      return { success: true, output: `Successfully wrote ${lineCount} lines to ${filePath}` };
    } catch (error) {
      return { success: false, output: '', error: `Failed to write file: ${getErrorMessage(error)}` };
    }
  },
};
