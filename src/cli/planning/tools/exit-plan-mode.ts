/**
 * ExitPlanMode Tool
 *
 * Exits plan mode and requests user approval for the implementation plan.
 * Supports pre-approving permissions for bash commands needed during execution.
 */

import { z } from 'zod';
import type { Tool, ToolResult, ToolContext } from '../../../core/tools/types.js';
import { getPlanModeManager } from '../state.js';
import { readPlanFile, parseFilesToChange, parsePreApprovedPermissions, getDisplayPath } from '../plan-file.js';
import type { AllowedPrompt } from '../types.js';

// ============================================================================
// Tool Definition
// ============================================================================

const AllowedPromptSchema = z.object({
  tool: z.literal('Bash').describe('The tool this permission applies to'),
  prompt: z.string().describe('Semantic description of the action, e.g., "run tests", "install dependencies"'),
});

const ExitPlanModeInputSchema = z.object({
  allowedPrompts: z
    .array(AllowedPromptSchema)
    .optional()
    .describe('Prompt-based permissions needed to implement the plan'),
});

export type ExitPlanModeInput = z.infer<typeof ExitPlanModeInputSchema>;

/**
 * ExitPlanMode Tool
 *
 * Call this when you have finished writing your plan and are ready
 * for user approval. The plan file should already be written.
 */
export const exitPlanModeTool: Tool<ExitPlanModeInput> = {
  name: 'ExitPlanMode',

  description: `Exit plan mode and request user approval for the implementation plan.

## How This Tool Works
- You should have already written your plan to the plan file
- This tool reads the plan from the file you wrote
- The user will see the plan contents and approve/modify/cancel

## Requesting Permissions (allowedPrompts)
Request prompt-based permissions for bash commands your plan will need:

\`\`\`json
{
  "allowedPrompts": [
    { "tool": "Bash", "prompt": "run tests" },
    { "tool": "Bash", "prompt": "install dependencies" },
    { "tool": "Bash", "prompt": "build the project" }
  ]
}
\`\`\`

Guidelines for prompts:
- Use semantic descriptions that capture the action's purpose
- "run tests" matches: npm test, pytest, go test, etc.
- "install dependencies" matches: npm install, pip install, etc.
- Keep descriptions concise but descriptive
- Only request permissions you actually need
- Scope permissions narrowly

## When to Use This Tool
Only use this tool when you have finished planning and are ready for approval.
Do NOT use this tool for research tasks - only for implementation planning.`,

  parameters: ExitPlanModeInputSchema,

  async execute(input: ExitPlanModeInput, context: ToolContext): Promise<ToolResult> {
    const manager = getPlanModeManager();

    // Check if in plan mode
    if (!manager.isActive()) {
      return {
        success: false,
        output: '',
        error: 'Not in plan mode. Use EnterPlanMode first.',
      };
    }

    const planFilePath = manager.getPlanFilePath();
    if (!planFilePath) {
      return {
        success: false,
        output: '',
        error: 'No plan file found. This should not happen.',
      };
    }

    try {
      // Read plan file
      const planFile = await readPlanFile(planFilePath);
      if (!planFile) {
        return {
          success: false,
          output: '',
          error: `Could not read plan file: ${planFilePath}`,
        };
      }

      // Parse plan content
      const filesToChange = parseFilesToChange(planFile.content);
      const planPermissions = parsePreApprovedPermissions(planFile.content);

      // Combine with input permissions
      const requestedPermissions: AllowedPrompt[] = [
        ...planPermissions,
        ...(input.allowedPrompts || []),
      ];

      // Store permissions for approval UI
      manager.setRequestedPermissions(requestedPermissions);
      manager.setPhase('approval');

      // Build response for the LLM
      const displayPath = getDisplayPath(planFilePath, context.cwd);
      let output = `
Plan ready for approval.

Plan file: ${displayPath}

`;

      // Summary of files to change
      if (filesToChange.length > 0) {
        output += 'Files to change:\n';
        for (const file of filesToChange) {
          const icon = file.action === 'create' ? '+' : file.action === 'modify' ? '~' : '-';
          output += `  ${icon} ${file.path} (${file.action})\n`;
        }
        output += '\n';
      }

      // Requested permissions
      if (requestedPermissions.length > 0) {
        output += 'Requested permissions:\n';
        for (const perm of requestedPermissions) {
          output += `  - ${perm.tool}(prompt: ${perm.prompt})\n`;
        }
        output += '\n';
      }

      output += `
The user will now be prompted to:
1. Approve - Accept plan and proceed with execution
2. Modify - Go back and modify the plan
3. Cancel - Exit plan mode without executing

Waiting for user decision...
`.trim();

      return {
        success: true,
        output,
      };
    } catch (error) {
      return {
        success: false,
        output: '',
        error: `Failed to exit plan mode: ${error instanceof Error ? error.message : String(error)}`,
      };
    }
  },
};
