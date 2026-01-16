/**
 * Tool Registry - Manages available tools
 */

import * as fs from 'fs/promises';
import type { Tool, ToolContext, ToolResult } from './types.js';
import { zodToJsonSchema, getErrorMessage, resolvePath } from './types.js';
import type { ToolDefinition } from '../providers/types.js';
import { getPlanModeManager } from '../planning/index.js';
import { getCheckpointManager } from '../checkpointing/index.js';
import type { ChangeType } from '../checkpointing/index.js';

// Tools that modify files and should be tracked for checkpointing
const CHECKPOINT_TOOLS = ['Write', 'Edit'];

export class ToolRegistry {
  private tools: Map<string, Tool> = new Map();

  register(tool: Tool): void {
    this.tools.set(tool.name, tool);
  }

  registerAll(tools: Tool[]): void {
    for (const tool of tools) {
      this.register(tool);
    }
  }

  get(name: string): Tool | undefined {
    return this.tools.get(name);
  }

  has(name: string): boolean {
    return this.tools.has(name);
  }

  list(): string[] {
    return Array.from(this.tools.keys());
  }

  /**
   * Get tool definitions for LLM
   */
  getDefinitions(toolNames?: string[]): ToolDefinition[] {
    const names = toolNames ?? this.list();
    return names
      .map((name) => {
        const tool = this.tools.get(name);
        if (!tool) return null;

        return {
          name: tool.name,
          description: tool.description,
          parameters: zodToJsonSchema(tool.parameters),
        };
      })
      .filter((t): t is ToolDefinition => t !== null);
  }

  /**
   * Get tool definitions filtered by plan mode
   * In plan mode, only read-only tools are returned
   */
  getFilteredDefinitions(toolNames?: string[]): ToolDefinition[] {
    const planManager = getPlanModeManager();
    const names = toolNames ?? this.list();

    // Filter tools based on plan mode state
    const filteredNames = planManager.filterTools(names);

    return this.getDefinitions(filteredNames);
  }

  /**
   * Execute a tool by name
   */
  async execute(name: string, input: unknown, context: ToolContext): Promise<ToolResult> {
    const tool = this.tools.get(name);

    if (!tool) {
      return {
        success: false,
        output: '',
        error: `Unknown tool: ${name}`,
      };
    }

    try {
      // Validate input
      const parsed = tool.parameters.safeParse(input);
      if (!parsed.success) {
        return {
          success: false,
          output: '',
          error: `Invalid input: ${parsed.error.message}`,
        };
      }

      // Capture pre-execution state for checkpointing
      let preState: { filePath: string; content: string | null; existed: boolean } | null = null;
      if (CHECKPOINT_TOOLS.includes(name) && parsed.data && typeof parsed.data === 'object') {
        const filePath = (parsed.data as { file_path?: string }).file_path;
        if (filePath) {
          preState = await this.captureFileState(filePath, context.cwd);
        }
      }

      // Execute the tool
      const result = await tool.execute(parsed.data, context);

      // Record checkpoint on successful file modification
      if (result.success && preState) {
        await this.recordCheckpoint(name, preState, context.cwd);
      }

      return result;
    } catch (error) {
      return { success: false, output: '', error: `Tool execution failed: ${getErrorMessage(error)}` };
    }
  }

  /**
   * Capture file state before modification
   */
  private async captureFileState(
    filePath: string,
    cwd: string
  ): Promise<{ filePath: string; content: string | null; existed: boolean }> {
    const resolvedPath = resolvePath(filePath, cwd);
    try {
      const content = await fs.readFile(resolvedPath, 'utf-8');
      return { filePath: resolvedPath, content, existed: true };
    } catch {
      // File doesn't exist
      return { filePath: resolvedPath, content: null, existed: false };
    }
  }

  /**
   * Record a checkpoint after file modification
   */
  private async recordCheckpoint(
    toolName: string,
    preState: { filePath: string; content: string | null; existed: boolean },
    cwd: string
  ): Promise<void> {
    const checkpointManager = getCheckpointManager();

    // Read current file content
    let newContent: string | null = null;
    try {
      newContent = await fs.readFile(preState.filePath, 'utf-8');
    } catch {
      // File was deleted or doesn't exist
    }

    // Determine change type
    let changeType: ChangeType;
    if (!preState.existed && newContent !== null) {
      changeType = 'create';
    } else if (preState.existed && newContent === null) {
      changeType = 'delete';
    } else {
      changeType = 'modify';
    }

    // Record the change
    checkpointManager.recordChange({
      path: preState.filePath,
      changeType,
      previousContent: preState.content,
      newContent,
      toolName,
    });
  }
}
