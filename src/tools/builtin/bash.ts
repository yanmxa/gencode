/**
 * Bash Tool - Execute shell commands
 */

import { spawn } from 'child_process';
import type { Tool, ToolContext, ToolResult } from '../types.js';
import { BashInputSchema, type BashInput } from '../types.js';
import { loadToolDescription } from '../../prompts/index.js';
import { TaskManager } from '../../tasks/task-manager.js';

export const bashTool: Tool<BashInput> = {
  name: 'Bash',
  description: loadToolDescription('bash'),
  parameters: BashInputSchema,

  async execute(input: BashInput, context: ToolContext): Promise<ToolResult> {
    // Handle background execution
    if (input.run_in_background) {
      const taskManager = new TaskManager();
      const description = input.description || `Bash: ${input.command.slice(0, 50)}`;

      try {
        const { taskId, outputFile } = await taskManager.createBashTask(
          input.command,
          description,
          context.cwd
        );

        return {
          success: true,
          output: `Background task started: ${taskId}\n\nOutput file: ${outputFile}\n\nUse TaskOutput tool to check status:\n  TaskOutput { action: "status", taskId: "${taskId}" }`,
          metadata: {
            title: 'Bash (background)',
            subtitle: description,
          },
        };
      } catch (error) {
        const errorMsg = error instanceof Error ? error.message : String(error);
        return {
          success: false,
          output: '',
          error: `Failed to start background task: ${errorMsg}`,
        };
      }
    }

    // Foreground execution (existing code)
    const timeout = input.timeout ?? 30000;

    return new Promise((resolve) => {
      const proc = spawn('bash', ['-c', input.command], {
        cwd: context.cwd,
        env: process.env,
        stdio: ['pipe', 'pipe', 'pipe'],
      });

      let stdout = '';
      let stderr = '';

      proc.stdout.on('data', (data) => {
        stdout += data.toString();
      });

      proc.stderr.on('data', (data) => {
        stderr += data.toString();
      });

      // Handle timeout
      const timer = setTimeout(() => {
        proc.kill('SIGTERM');
        resolve({
          success: false,
          output: stdout,
          error: `Command timed out after ${timeout}ms`,
        });
      }, timeout);

      // Handle abort signal
      if (context.abortSignal) {
        context.abortSignal.addEventListener('abort', () => {
          proc.kill('SIGTERM');
          clearTimeout(timer);
          resolve({
            success: false,
            output: stdout,
            error: 'Command aborted',
          });
        });
      }

      proc.on('close', (code) => {
        clearTimeout(timer);

        // Truncate output if too long
        const maxLength = 30000;
        let output = stdout;
        if (output.length > maxLength) {
          output = output.slice(0, maxLength) + '\n... (output truncated)';
        }

        if (code === 0) {
          resolve({
            success: true,
            output: output || '(no output)',
          });
        } else {
          resolve({
            success: false,
            output,
            error: stderr || `Command exited with code ${code}`,
          });
        }
      });

      proc.on('error', (error) => {
        clearTimeout(timer);
        resolve({
          success: false,
          output: '',
          error: `Failed to execute command: ${error.message}`,
        });
      });
    });
  },
};
