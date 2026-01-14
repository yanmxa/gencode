/**
 * Tool Registry - Manages available tools
 */

import type { Tool, ToolContext, ToolResult } from './types.js';
import { zodToJsonSchema, getErrorMessage } from './types.js';
import type { ToolDefinition } from '../providers/types.js';

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

      return await tool.execute(parsed.data, context);
    } catch (error) {
      return { success: false, output: '', error: `Tool execution failed: ${getErrorMessage(error)}` };
    }
  }
}
