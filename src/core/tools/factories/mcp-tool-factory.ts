/**
 * MCP Tool Bridge
 * Converts MCP tools to gencode Tool format
 */

import { z } from 'zod';
import type { Client } from '@modelcontextprotocol/sdk/client/index.js';
import type { Tool, ToolContext, ToolResult } from '../types.js';
import type { MCPToolDef } from '../../../ext/mcp/types.js';

/**
 * Convert JSON Schema to Zod schema (simplified version)
 * This is a basic implementation - full JSON Schema support would be complex
 */
function jsonSchemaToZod(schema: Record<string, unknown>): z.ZodSchema {
  // For now, use a permissive object schema
  // A full implementation would parse the JSON Schema and build the correct Zod schema
  // This allows any object to pass validation
  return z.record(z.string(), z.unknown());
}

/**
 * Sanitize a string for use in tool names
 */
function sanitize(str: string): string {
  return str.replace(/[^a-zA-Z0-9_]/g, '_');
}

/**
 * Bridge an MCP tool to gencode Tool format
 */
export function bridgeMCPTool(
  mcpTool: MCPToolDef,
  client: Client,
  serverName: string
): Tool {
  const toolName = `mcp_${sanitize(serverName)}_${sanitize(mcpTool.name)}`;

  return {
    name: toolName,
    description: mcpTool.description ?? `MCP tool from ${serverName}: ${mcpTool.name}`,
    parameters: jsonSchemaToZod(mcpTool.inputSchema),
    execute: async (input: unknown, context: ToolContext): Promise<ToolResult> => {
      try {
        // Call the MCP tool via the client
        const result = await client.callTool({
          name: mcpTool.name,
          arguments: input as Record<string, unknown>,
        });

        // Check if the result is an error
        if (result.isError) {
          return {
            success: false,
            output: '',
            error: `MCP tool error: ${JSON.stringify(result.content)}`,
          };
        }

        // Format the successful result
        const content = result.content as any[];
        const output = content
          .map((item: any) => {
            if (item.type === 'text') {
              return item.text;
            } else if (item.type === 'image') {
              return `[Image: ${item.mimeType}]`;
            } else if (item.type === 'resource') {
              return `[Resource: ${item.resource.uri}]`;
            }
            return JSON.stringify(item);
          })
          .join('\n');

        return {
          success: true,
          output,
        };
      } catch (error) {
        return {
          success: false,
          output: '',
          error: `Failed to execute MCP tool: ${error instanceof Error ? error.message : String(error)}`,
        };
      }
    },
  };
}

/**
 * Bridge multiple MCP tools from a server
 */
export async function bridgeMCPTools(
  client: Client,
  serverName: string
): Promise<Tool[]> {
  try {
    // List available tools from the MCP server
    const toolsResult = await client.listTools();

    // Convert each MCP tool to gencode Tool format
    return toolsResult.tools.map((mcpTool) =>
      bridgeMCPTool(mcpTool, client, serverName)
    );
  } catch (error) {
    console.warn(`Failed to list tools from MCP server "${serverName}":`, error);
    return [];
  }
}
