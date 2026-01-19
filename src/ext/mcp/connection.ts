/**
 * MCP Connection Management
 * Creates transports and manages connections with fallback
 */

import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';
import { SSEClientTransport } from '@modelcontextprotocol/sdk/client/sse.js';
import type { MCPServerConfig, MCPStdioConfig, MCPHTTPConfig, MCPSSEConfig } from './types.js';

/**
 * Create stdio transport for local processes
 */
function createStdioTransport(config: MCPStdioConfig): StdioClientTransport {
  return new StdioClientTransport({
    command: config.command,
    args: config.args,
    env: config.env,
  });
}

/**
 * Create HTTP transport for remote servers
 */
function createHTTPTransport(config: MCPHTTPConfig): StreamableHTTPClientTransport {
  return new StreamableHTTPClientTransport(new URL(config.url), {
    requestInit: config.headers ? { headers: config.headers } : undefined,
  });
}

/**
 * Create SSE transport for remote servers (legacy)
 */
function createSSETransport(config: MCPSSEConfig): SSEClientTransport {
  const options: any = {};
  if (config.headers) {
    // For SSE: set headers in both eventSourceInit (for SSE connection) and requestInit (for POST)
    options.eventSourceInit = { headers: config.headers };
    options.requestInit = { headers: config.headers };
  }
  return new SSEClientTransport(new URL(config.url), options);
}

/**
 * Connect to an MCP server with the appropriate transport
 * For remote servers, tries multiple transports with fallback
 */
export async function connectToServer(
  client: Client,
  config: MCPServerConfig,
  serverName: string
): Promise<void> {
  if (config.type === 'stdio') {
    // Stdio transport - direct connection
    const transport = createStdioTransport(config);
    await client.connect(transport);
    return;
  }

  // Remote servers - try multiple transports with fallback
  const transports = [];

  if (config.type === 'http') {
    transports.push({
      name: 'StreamableHTTP',
      create: () => createHTTPTransport(config),
    });
    // Also try SSE as fallback for HTTP
    transports.push({
      name: 'SSE',
      create: () => createSSETransport({ type: 'sse', url: config.url, headers: config.headers }),
    });
  } else if (config.type === 'sse') {
    transports.push({
      name: 'SSE',
      create: () => createSSETransport(config),
    });
  }

  // Try each transport in order
  let lastError: Error | null = null;

  for (const { name, create } of transports) {
    try {
      const transport = create();
      await client.connect(transport);
      console.debug(`[MCP] Connected to "${serverName}" via ${name}`);
      return;
    } catch (error) {
      console.debug(`[MCP] ${name} transport failed for "${serverName}":`, error);
      lastError = error instanceof Error ? error : new Error(String(error));
    }
  }

  // All transports failed
  throw new Error(
    `Failed to connect to "${serverName}": ${lastError?.message ?? 'All transports failed'}`
  );
}

/**
 * Disconnect from an MCP server
 */
export async function disconnectFromServer(client: Client): Promise<void> {
  try {
    await client.close();
  } catch (error) {
    console.warn('[MCP] Error during disconnect:', error);
  }
}
