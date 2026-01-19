/**
 * MCP Manager
 * Singleton manager for all MCP servers and clients
 */

import type { Client } from '@modelcontextprotocol/sdk/client/index.js';
import type { Tool } from '../../core/tools/types.js';
import type { MCPConfig, MCPServer, MCPStatus, MCPServerInfo } from './types.js';
import { createMCPClient } from './client.js';
import { connectToServer, disconnectFromServer } from './connection.js';
import { bridgeMCPTools } from '../../core/tools/factories/mcp-tool-factory.js';
import { logger } from '../../infrastructure/utils/logger.js';
import { isDebugEnabled } from '../../infrastructure/utils/debug.js';

/**
 * MCP Manager - Singleton
 */
export class MCPManager {
  private static instance: MCPManager | null = null;
  private servers: Map<string, MCPServer> = new Map();
  private initialized = false;

  private constructor() {}

  /**
   * Get singleton instance
   */
  static getInstance(): MCPManager {
    if (!MCPManager.instance) {
      MCPManager.instance = new MCPManager();
    }
    return MCPManager.instance;
  }

  /**
   * Initialize from MCP configuration
   */
  async initialize(config: MCPConfig): Promise<void> {
    if (this.initialized) {
      return; // Already initialized
    }

    // Create server instances from config
    for (const [name, serverConfig] of Object.entries(config.mcpServers)) {
      // Skip disabled servers
      if (serverConfig.enabled === false) {
        this.servers.set(name, {
          name,
          config: serverConfig,
          client: null,
          status: { status: 'disabled' },
        });
        continue;
      }

      // Create client but don't connect yet (lazy connection)
      this.servers.set(name, {
        name,
        config: serverConfig,
        client: null,
        status: { status: 'connecting' },
      });
    }

    this.initialized = true;

    // Connect to all enabled servers
    await this.connectAll();
  }

  /**
   * Connect to all enabled servers
   */
  async connectAll(): Promise<void> {
    const promises = Array.from(this.servers.keys()).map((name) => this.connect(name));
    await Promise.allSettled(promises);
  }

  /**
   * Connect to a specific server
   */
  async connect(serverName: string): Promise<void> {
    const server = this.servers.get(serverName);
    if (!server) {
      throw new Error(`MCP server "${serverName}" not found`);
    }

    // Skip if already connected or disabled
    if (server.status.status === 'connected' || server.status.status === 'disabled') {
      return;
    }

    try {
      // Update status to connecting and record attempt time
      server.status = { status: 'connecting' };
      server.lastAttempt = new Date();

      // Create client if not exists
      if (!server.client) {
        server.client = createMCPClient(serverName, server.config);
      }

      // Connect via appropriate transport
      await connectToServer(server.client, server.config, serverName);

      // Update status
      server.status = { status: 'connected' };
      server.connectedAt = new Date();

      // Count tools
      const tools = await this.getServerTools(serverName);
      server.toolCount = tools.length;

      if (isDebugEnabled('mcp')) {
        logger.debug('MCP', `Connected to server "${serverName}"`, {
          toolCount: tools.length,
        });
      }
    } catch (error) {
      const errorMsg = error instanceof Error ? error.message : String(error);
      server.status = { status: 'failed', error: errorMsg };
      logger.warn('MCP', `Failed to connect to server "${serverName}"`, {
        error: errorMsg,
        serverType: server.config.type,
        hint: 'Check server configuration and availability. Use retryFailedServer() to retry.',
        retryAvailable: true,
      });
    }
  }

  /**
   * Disconnect from a specific server
   */
  async disconnect(serverName: string): Promise<void> {
    const server = this.servers.get(serverName);
    if (!server || !server.client) {
      return;
    }

    await disconnectFromServer(server.client);
    server.client = null;
    server.status = { status: 'disabled' };
    server.connectedAt = undefined;
    server.toolCount = undefined;
  }

  /**
   * Disconnect from all servers
   */
  async disconnectAll(): Promise<void> {
    const promises = Array.from(this.servers.keys()).map((name) => this.disconnect(name));
    await Promise.allSettled(promises);
  }

  /**
   * Retry connecting to a failed server
   *
   * @param serverName - Server to retry
   * @param resetClient - If true, recreate the client (default: false)
   * @returns true if connection succeeded, false otherwise
   */
  async retryFailedServer(serverName: string, resetClient = false): Promise<boolean> {
    const server = this.servers.get(serverName);
    if (!server) {
      throw new Error(`MCP server "${serverName}" not found`);
    }

    // Only retry failed servers
    if (server.status.status !== 'failed') {
      console.warn(`[MCP] Server "${serverName}" is not in failed state (status: ${server.status.status})`);
      return false;
    }

    // Reset client if requested
    if (resetClient && server.client) {
      await disconnectFromServer(server.client);
      server.client = null;
    }

    // Attempt reconnection
    await this.connect(serverName);

    // Return success status (re-fetch server since status may have changed)
    const updatedServer = this.servers.get(serverName);
    return updatedServer?.status.status === 'connected' || false;
  }

  /**
   * Retry all failed servers
   *
   * @param resetClients - If true, recreate clients for failed servers
   * @returns Map of server names to success status
   */
  async retryAllFailedServers(resetClients = false): Promise<Map<string, boolean>> {
    const results = new Map<string, boolean>();

    for (const [name, server] of this.servers.entries()) {
      if (server.status.status === 'failed') {
        const success = await this.retryFailedServer(name, resetClients);
        results.set(name, success);
      }
    }

    return results;
  }

  /**
   * Get tools from a specific server
   */
  async getServerTools(serverName: string): Promise<Tool[]> {
    const server = this.servers.get(serverName);
    if (!server || !server.client || server.status.status !== 'connected') {
      return [];
    }

    try {
      return await bridgeMCPTools(server.client, serverName);
    } catch (error) {
      console.warn(`[MCP] Failed to get tools from "${serverName}":`, error);
      return [];
    }
  }

  /**
   * Get all tools from all connected servers
   */
  async getAllTools(): Promise<Tool[]> {
    const toolPromises = Array.from(this.servers.keys()).map((name) =>
      this.getServerTools(name)
    );

    const toolArrays = await Promise.all(toolPromises);
    return toolArrays.flat();
  }

  /**
   * Get server status
   */
  getServerStatus(serverName: string): MCPStatus | null {
    const server = this.servers.get(serverName);
    return server?.status ?? null;
  }

  /**
   * Get all server info for CLI display
   */
  getAllServerInfo(): MCPServerInfo[] {
    return Array.from(this.servers.values()).map((server) => ({
      name: server.name,
      type: server.config.type,
      status: server.status.status,
      enabled: server.config.enabled !== false,
      toolCount: server.toolCount,
      error: server.status.status === 'failed' ? server.status.error : undefined,
    }));
  }

  /**
   * Get a specific server instance
   */
  getServer(serverName: string): MCPServer | undefined {
    return this.servers.get(serverName);
  }

  /**
   * Check if initialized
   */
  isInitialized(): boolean {
    return this.initialized;
  }

  /**
   * Cleanup and disconnect all servers
   */
  async cleanup(): Promise<void> {
    await this.disconnectAll();
    this.servers.clear();
    this.initialized = false;
  }
}

/**
 * Get singleton instance
 */
export function getMCPManager(): MCPManager {
  return MCPManager.getInstance();
}
