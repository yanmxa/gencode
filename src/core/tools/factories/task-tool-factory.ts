/**
 * Task Tool - Launch specialized subagents for isolated task execution
 *
 * Based on Claude Code's Task tool architecture.
 * Enables context-isolated exploration and specialized task delegation.
 */

import { z } from 'zod';
import type { Tool, ToolContext, ToolResult } from '../types.js';
import { Subagent } from '../../../extensions/subagents/subagent.js';
import type { TaskInput } from '../../../extensions/subagents/types.js';
import { TaskManager } from '../../session/tasks/task-manager.js';
import { SubagentSessionManager } from '../../../extensions/subagents/subagent-session-manager.js';

/**
 * Task Tool - Spawn isolated subagents for specialized tasks
 */
export const taskTool: Tool<TaskInput> = {
  name: 'Task',

  description: `Launch a specialized subagent for complex, multi-step tasks.

**When to use Task:**
- Exploring large codebases without polluting main context
- Researching patterns across multiple files
- Delegating independent subtasks
- Running multi-step command sequences

**Agent Types:**

• **Explore** - Fast read-only codebase exploration
  - Tools: Read, Glob, Grep, WebFetch
  - Model: claude-haiku-4 (fast, cost-effective)
  - Use for: Finding files, searching patterns, analyzing code structure
  - Max turns: 10

• **Plan** - Architecture design and planning
  - Tools: Read, Glob, Grep, WebFetch, TodoWrite
  - Model: claude-sonnet-4 (complex reasoning)
  - Use for: Designing implementation approaches, creating plans
  - Max turns: 15

• **Bash** - Command execution specialist
  - Tools: Bash only
  - Model: claude-haiku-4 (fast execution)
  - Use for: Running shell commands, build scripts, test commands
  - Max turns: 20

• **general-purpose** - Full capabilities
  - Tools: All tools (Read, Write, Edit, Bash, Glob, Grep, etc.)
  - Model: claude-sonnet-4 (balanced)
  - Use for: Complex multi-step tasks requiring various tools
  - Max turns: 20

**Guidelines:**
- Use Explore for finding code patterns and searching files
- Use Plan for designing implementation approaches
- Use Bash for running commands
- Use general-purpose for complex multi-step workflows
- Include a short description (3-5 words) for UI display
- Provide detailed prompt with context
- Launch multiple agents in parallel when possible (future)

**Example:**
Task({
  description: "Find auth error handling",
  prompt: "Search the codebase for authentication error handling. Look for AuthError classes, error middleware, and validation logic. Summarize where errors are caught and how they're handled.",
  subagent_type: "Explore"
})

**Context Isolation:**
Subagents run in isolated contexts - only their summary is returned to you, not their full conversation history. This prevents context pollution while still providing useful findings.`,

  parameters: z.object({
    description: z
      .string()
      .min(1)
      .describe('Short description (3-5 words) for UI display'),

    prompt: z
      .string()
      .min(1)
      .describe('Detailed task instructions for the subagent'),

    subagent_type: z
      .enum(['Explore', 'Plan', 'Bash', 'general-purpose'])
      .describe('Type of subagent to spawn'),

    model: z
      .string()
      .optional()
      .describe('Optional: specific model to use (overrides default for type)'),

    run_in_background: z
      .boolean()
      .optional()
      .describe('Optional: run in background (Phase 2 - not yet implemented)'),

    resume: z
      .string()
      .optional()
      .describe('Optional: resume a previous subagent by ID (Phase 3 - not yet implemented)'),

    max_turns: z
      .number()
      .positive()
      .optional()
      .describe('Optional: max conversation turns (overrides default)'),

    tasks: z
      .array(
        z.object({
          description: z.string().min(1).describe('Short description (3-5 words)'),
          prompt: z.string().min(1).describe('Task prompt'),
          subagent_type: z
            .enum(['Explore', 'Plan', 'Bash', 'general-purpose'])
            .describe('Subagent type'),
          model: z.string().optional().describe('Optional model override'),
          max_turns: z.number().positive().optional().describe('Optional max turns'),
        })
      )
      .optional()
      .describe('Optional: array of tasks to execute in parallel (Phase 4)'),
  }),

  async execute(input: TaskInput, context: ToolContext): Promise<ToolResult> {
    try {
      // Phase 4: Parallel execution
      if (input.tasks && input.tasks.length > 0) {
        const startTime = Date.now();

        // Launch all tasks in parallel
        const taskPromises = input.tasks.map(async (taskDef) => {
          const subagent = new Subagent({
            type: taskDef.subagent_type,
            model: taskDef.model, // Can be undefined, will fall back
            provider: context.currentProvider as any,
            authMethod: context.currentAuthMethod as any,
            parentModel: context.currentModel,
            cwd: context.cwd,
            config: taskDef.max_turns ? { maxTurns: taskDef.max_turns } : undefined,
            persistSession: true,
            description: taskDef.description,
          });

          return {
            definition: taskDef,
            result: await subagent.run(taskDef.prompt, context.abortSignal),
          };
        });

        // Wait for all tasks to complete
        const results = await Promise.all(taskPromises);

        // Format output
        const output = formatParallelResults(results, Date.now() - startTime);

        return {
          success: true,
          output,
          metadata: {
            title: 'Task(Parallel)',
            subtitle: `${input.tasks.length} tasks completed`,
            duration: Date.now() - startTime,
          },
        };
      }

      // Phase 3: Resume capability
      if (input.resume) {
        const sessionManager = new SubagentSessionManager();
        const validation = await sessionManager.validateSubagentSession(input.resume);

        if (!validation.valid) {
          return {
            success: false,
            output: '',
            error: `Resume failed: ${validation.reason}`,
          };
        }

        // Create subagent instance
        const subagent = new Subagent({
          type: input.subagent_type,
          model: input.model, // Can be undefined, will fall back
          provider: context.currentProvider as any,
          authMethod: context.currentAuthMethod as any,
          parentModel: context.currentModel,
          cwd: context.cwd,
          config: input.max_turns ? { maxTurns: input.max_turns } : undefined,
          persistSession: true, // Keep persistence enabled for resumed sessions
        });

        // Resume from session
        const result = await subagent.resumeSession(input.resume, input.prompt);

        if (!result.success) {
          return {
            success: false,
            output: '',
            error: `Resume failed: ${result.error}`,
          };
        }

        // Format output
        const output = formatResumeResult(input, result, input.resume);

        return {
          success: true,
          output,
          metadata: {
            title: `Task(${input.subagent_type})`,
            subtitle: `${input.description} (resumed)`,
            duration: result.metadata?.durationMs,
          },
        };
      }

      // Create subagent instance
      // Use parent agent's model/provider/authMethod if not explicitly specified
      const subagent = new Subagent({
        type: input.subagent_type,
        model: input.model, // Can be undefined, will fall back in Subagent constructor
        provider: context.currentProvider as any,
        authMethod: context.currentAuthMethod as any,
        parentModel: context.currentModel, // Pass parent model for fallback
        cwd: context.cwd,
        config: input.max_turns ? { maxTurns: input.max_turns } : undefined,
        persistSession: true, // Always enable persistence for resume capability
        description: input.description,
      });

      // Phase 2: Background execution
      if (input.run_in_background) {
        const taskManager = new TaskManager();
        const { taskId, outputFile } = await taskManager.createTask(
          subagent,
          input.description,
          input.prompt
        );

        const output = formatBackgroundTaskStart(taskId, outputFile, input);

        return {
          success: true,
          output,
          metadata: {
            title: `Task(${input.subagent_type})`,
            subtitle: `${input.description} (background)`,
          },
        };
      }

      // Foreground execution
      const result = await subagent.run(input.prompt, context.abortSignal);

      if (!result.success) {
        return {
          success: false,
          output: '',
          error: `Subagent execution failed: ${result.error}`,
        };
      }

      // Format output for main agent
      const output = formatSubagentResult(input, result);

      return {
        success: true,
        output,
        metadata: {
          title: `Task(${input.subagent_type})`,
          subtitle: input.description,
          duration: result.metadata?.durationMs,
        },
      };
    } catch (error) {
      return {
        success: false,
        output: '',
        error: `Task execution failed: ${error instanceof Error ? error.message : String(error)}`,
      };
    }
  },
};

/**
 * Format subagent result for display
 */
function formatSubagentResult(input: TaskInput, result: { success: boolean; result?: string; agentId: string; metadata?: { subagentType: string; model: string; turns: number; durationMs: number; tokenUsage?: { input: number; output: number } } }): string {
  const lines: string[] = [];

  // Header
  lines.push('┌─ Subagent Result ─────────────────────────────────────');
  lines.push(`│ Type: ${input.subagent_type}`);
  lines.push(`│ Task: ${input.description}`);

  if (result.metadata) {
    lines.push(`│ Duration: ${result.metadata.durationMs}ms`);
    lines.push(`│ Turns: ${result.metadata.turns}`);
    lines.push(`│ Model: ${result.metadata.model}`);

    if (result.metadata.tokenUsage) {
      const total = result.metadata.tokenUsage.input + result.metadata.tokenUsage.output;
      lines.push(`│ Tokens: ${total.toLocaleString()} (${result.metadata.tokenUsage.input.toLocaleString()} in, ${result.metadata.tokenUsage.output.toLocaleString()} out)`);
    }
  }

  lines.push(`│ Agent ID: ${result.agentId}`);
  lines.push('└───────────────────────────────────────────────────────');
  lines.push('');

  // Result
  if (result.result) {
    lines.push(result.result);
  } else {
    lines.push('(No output generated)');
  }

  return lines.join('\n');
}

/**
 * Format background task start message
 */
function formatBackgroundTaskStart(taskId: string, outputFile: string, input: TaskInput): string {
  const lines: string[] = [];

  lines.push('┌─ Background Task Started ─────────────────────────────');
  lines.push(`│ Task ID: ${taskId}`);
  lines.push(`│ Type: ${input.subagent_type}`);
  lines.push(`│ Description: ${input.description}`);
  lines.push(`│ Output: ${outputFile}`);
  lines.push('└───────────────────────────────────────────────────────');
  lines.push('');
  lines.push('The task is running in the background. You can continue working.');
  lines.push('');
  lines.push('To check status:');
  lines.push(`  TaskOutput({ action: 'status', taskId: '${taskId}' })`);
  lines.push('');
  lines.push('To get result (blocks until complete):');
  lines.push(`  TaskOutput({ action: 'result', taskId: '${taskId}', block: true })`);
  lines.push('');
  lines.push('To cancel:');
  lines.push(`  TaskOutput({ action: 'cancel', taskId: '${taskId}' })`);

  return lines.join('\n');
}

/**
 * Format parallel tasks results (Phase 4)
 */
function formatParallelResults(
  results: Array<{
    definition: { description: string; subagent_type: string };
    result: { success: boolean; result?: string; error?: string; metadata?: { turns: number; durationMs: number; tokenUsage?: { input: number; output: number } } };
  }>,
  totalDurationMs: number
): string {
  const lines: string[] = [];

  // Header
  lines.push('┌─ Parallel Tasks Completed ────────────────────────────');
  lines.push(`│ Tasks: ${results.length}`);
  lines.push(`│ Total Duration: ${totalDurationMs}ms`);

  // Calculate totals
  let successCount = 0;
  let totalTurns = 0;
  let totalTokens = { input: 0, output: 0 };

  for (const { result } of results) {
    if (result.success) successCount++;
    if (result.metadata) {
      totalTurns += result.metadata.turns || 0;
      if (result.metadata.tokenUsage) {
        totalTokens.input += result.metadata.tokenUsage.input;
        totalTokens.output += result.metadata.tokenUsage.output;
      }
    }
  }

  lines.push(`│ Success: ${successCount}/${results.length}`);
  lines.push(`│ Total Turns: ${totalTurns}`);
  lines.push(
    `│ Total Tokens: ${(totalTokens.input + totalTokens.output).toLocaleString()} (${totalTokens.input.toLocaleString()} in, ${totalTokens.output.toLocaleString()} out)`
  );
  lines.push('└───────────────────────────────────────────────────────');
  lines.push('');

  // Individual results
  for (let i = 0; i < results.length; i++) {
    const { definition, result } = results[i];
    const statusIcon = result.success ? '✅' : '❌';

    lines.push(`${statusIcon} Task ${i + 1}: ${definition.description}`);
    lines.push(`   Type: ${definition.subagent_type}`);

    if (result.metadata) {
      lines.push(`   Duration: ${result.metadata.durationMs}ms`);
      lines.push(`   Turns: ${result.metadata.turns}`);
      if (result.metadata.tokenUsage) {
        const total = result.metadata.tokenUsage.input + result.metadata.tokenUsage.output;
        lines.push(`   Tokens: ${total.toLocaleString()}`);
      }
    }

    if (result.success && result.result) {
      // Truncate result to 200 chars for overview
      const preview = result.result.length > 200 ? result.result.slice(0, 200) + '...' : result.result;
      lines.push(`   Result: ${preview}`);
    } else if (result.error) {
      lines.push(`   Error: ${result.error}`);
    }

    lines.push('');
  }

  return lines.join('\n');
}

/**
 * Format resume result message
 */
function formatResumeResult(input: TaskInput, result: { success: boolean; result?: string; agentId: string; metadata?: { subagentType: string; model: string; turns: number; durationMs: number; tokenUsage?: { input: number; output: number }; sessionId?: string } }, sessionId: string): string {
  const lines: string[] = [];

  // Header
  lines.push('┌─ Resumed Subagent Result ─────────────────────────────');
  lines.push(`│ Session ID: ${sessionId}`);
  lines.push(`│ Type: ${input.subagent_type}`);
  lines.push(`│ Task: ${input.description}`);

  if (result.metadata) {
    lines.push(`│ Duration: ${result.metadata.durationMs}ms`);
    lines.push(`│ Turns: ${result.metadata.turns}`);
    lines.push(`│ Model: ${result.metadata.model}`);

    if (result.metadata.tokenUsage) {
      const total = result.metadata.tokenUsage.input + result.metadata.tokenUsage.output;
      lines.push(`│ Tokens: ${total.toLocaleString()} (${result.metadata.tokenUsage.input.toLocaleString()} in, ${result.metadata.tokenUsage.output.toLocaleString()} out)`);
    }
  }

  lines.push(`│ Agent ID: ${result.agentId}`);
  lines.push('└───────────────────────────────────────────────────────');
  lines.push('');

  // Result
  if (result.result) {
    lines.push(result.result);
  } else {
    lines.push('(No output generated)');
  }

  return lines.join('\n');
}
