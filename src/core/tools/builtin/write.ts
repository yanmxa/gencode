/**
 * Write Tool - Write content to a file
 *
 * In plan mode, this tool is only allowed to write to the plan file.
 * Shows diff preview when overwriting existing files.
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import { createTwoFilesPatch } from 'diff';
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

      // Check if file exists to determine if we should show diff
      let existingContent: string | null = null;
      let isNewFile = false;
      try {
        existingContent = await fs.readFile(filePath, 'utf-8');
      } catch {
        isNewFile = true;
      }

      // Generate diff if file exists and content is different
      let diff: string | null = null;
      if (!isNewFile && existingContent !== null && existingContent !== input.content) {
        diff = createTwoFilesPatch(
          filePath,
          filePath,
          existingContent,
          input.content,
          undefined,
          undefined
        );
        // Trim diff to remove file headers (--- +++)
        diff = diff
          .split('\n')
          .slice(2)
          .join('\n');
      }

      // Request permission with diff metadata for existing files
      if (context.askPermission && !isNewFile && diff) {
        const decision = await context.askPermission({
          tool: 'Write',
          input,
          metadata: {
            diff,
            filePath,
            isOverwrite: true,
          },
        });

        if (!decision || decision === 'deny') {
          return {
            success: false,
            output: '',
            error: 'Write operation denied by user',
          };
        }
      }

      await fs.mkdir(path.dirname(filePath), { recursive: true });
      await fs.writeFile(filePath, input.content, 'utf-8');

      const lineCount = input.content.split('\n').length;
      const action = isNewFile ? 'Created' : 'Updated';
      return { success: true, output: `${action} ${filePath} (${lineCount} lines)` };
    } catch (error) {
      return { success: false, output: '', error: `Failed to write file: ${getErrorMessage(error)}` };
    }
  },
};
