/**
 * MCP Client Initialization
 * Creates and configures MCP clients for different transport types
 */

import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import type { MCPServerConfig } from './types.js';

/**
 * Create an MCP client instance
 * Client must be connected via transport separately
 */
export function createMCPClient(serverName: string, config: MCPServerConfig): Client {
  // Client options
  const options = {
    name: 'gencode',
    version: '0.4.1',
  };

  // Create client instance
  const client = new Client(options, {
    capabilities: {
      // Request sampling capabilities from server
      sampling: {},
    },
  });

  return client;
}

/**
 * Get timeout for server operations
 * @param config Server configuration
 * @returns Timeout in milliseconds
 */
export function getServerTimeout(config: MCPServerConfig): number {
  // Default timeouts based on transport type
  switch (config.type) {
    case 'stdio':
      return 30000; // 30s for local processes
    case 'http':
    case 'sse':
      return 60000; // 60s for remote servers
  }
}
