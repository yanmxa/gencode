/**
 * EnterPlanMode Tool
 *
 * Transitions the agent into plan mode for designing implementation
 * approaches before writing code. In plan mode, only read-only tools
 * are available.
 */

import { z } from 'zod';
import type { Tool, ToolResult, ToolContext } from '../../../core/tools/types.js';
import { getPlanModeManager } from '../state.js';
import { createPlanFile, getDisplayPath } from '../plan-file.js';
import { PLAN_MODE_ALLOWED_TOOLS, PLAN_MODE_BLOCKED_TOOLS } from '../types.js';

// ============================================================================
// Tool Definition
// ============================================================================

const EnterPlanModeInputSchema = z.object({});

export type EnterPlanModeInput = z.infer<typeof EnterPlanModeInputSchema>;

/**
 * EnterPlanMode Tool
 *
 * Use this tool when you need to plan an implementation before writing code.
 * This is recommended for:
 * - New feature implementations
 * - Tasks with multiple valid approaches
 * - Architectural decisions
 * - Multi-file changes
 * - Unclear requirements
 */
export const enterPlanModeTool: Tool<EnterPlanModeInput> = {
  name: 'EnterPlanMode',

  description: `Transition into plan mode to design an implementation approach before writing code.

Use this tool when:
- Implementing new features that require design decisions
- Tasks with multiple valid approaches
- Changes that affect existing behavior or structure
- Architectural decisions between patterns or technologies
- Multi-file changes (more than 2-3 files)
- Unclear requirements that need exploration

In plan mode:
- Only read-only tools are available (Read, Glob, Grep, WebFetch, WebSearch, TodoWrite, AskUserQuestion)
- Write, Edit, and Bash tools are blocked
- You can explore the codebase and design an approach
- Use ExitPlanMode when ready to request user approval

This tool REQUIRES user approval before entering plan mode.`,

  parameters: EnterPlanModeInputSchema,

  async execute(_input: EnterPlanModeInput, context: ToolContext): Promise<ToolResult> {
    const manager = getPlanModeManager();

    // Check if already in plan mode
    if (manager.isActive()) {
      return {
        success: false,
        output: '',
        error: 'Already in plan mode. Use ExitPlanMode to exit first.',
      };
    }

    try {
      // Create plan file
      const planFile = await createPlanFile(context.cwd);
      const displayPath = getDisplayPath(planFile.path, context.cwd);

      // Enter plan mode
      manager.enter(planFile.path);

      // Build response
      const output = `
Entered PLAN mode.

Plan file: ${displayPath}

In plan mode, you have access to read-only tools for exploration:
${PLAN_MODE_ALLOWED_TOOLS.map((t) => `  - ${t}`).join('\n')}

Blocked tools (write/execute operations):
${PLAN_MODE_BLOCKED_TOOLS.map((t) => `  - ${t}`).join('\n')}

Instructions:
1. Explore the codebase using Read, Glob, Grep tools
2. Research if needed using WebFetch, WebSearch
3. Design your implementation approach
4. Write your plan to the plan file using Write (plan file only)
5. Call ExitPlanMode when ready for user approval

Use Shift+Tab to toggle between Plan and Build modes.
`.trim();

      return {
        success: true,
        output,
      };
    } catch (error) {
      return {
        success: false,
        output: '',
        error: `Failed to enter plan mode: ${error instanceof Error ? error.message : String(error)}`,
      };
    }
  },
};
