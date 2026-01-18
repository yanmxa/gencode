/**
 * MCP Integration Types
 * Type definitions for Model Context Protocol integration
 */

import type { Client } from '@modelcontextprotocol/sdk/client/index.js';

// =============================================================================
// Configuration Types
// =============================================================================

/**
 * MCP server configuration
 */
export type MCPServerConfig = MCPStdioConfig | MCPHTTPConfig | MCPSSEConfig;

/**
 * Stdio transport configuration (local processes)
 */
export interface MCPStdioConfig {
  type: 'stdio';
  command: string;
  args?: string[];
  env?: Record<string, string>;
  enabled?: boolean;
}

/**
 * HTTP transport configuration (remote servers)
 */
export interface MCPHTTPConfig {
  type: 'http';
  url: string;
  headers?: Record<string, string>;
  enabled?: boolean;
}

/**
 * SSE transport configuration (legacy remote servers)
 */
export interface MCPSSEConfig {
  type: 'sse';
  url: string;
  headers?: Record<string, string>;
  enabled?: boolean;
}

/**
 * MCP configuration file format (.mcp.json)
 * Compatible with Claude Code
 */
export interface MCPConfig {
  mcpServers: {
    [serverName: string]: MCPServerConfig;
  };
}

// =============================================================================
// Status Types
// =============================================================================

/**
 * MCP server status (discriminated union for type safety)
 */
export type MCPStatus =
  | { status: 'connected' }
  | { status: 'connecting' }
  | { status: 'disabled' }
  | { status: 'failed'; error: string }
  | { status: 'needs_auth' }
  | { status: 'needs_client_registration'; error: string };

/**
 * MCP server instance
 */
export interface MCPServer {
  name: string;
  config: MCPServerConfig;
  client: Client | null;
  status: MCPStatus;
  toolCount?: number;
  connectedAt?: Date;
  lastAttempt?: Date; // Last connection attempt timestamp (for retry logic)
}

// =============================================================================
// Tool Types
// =============================================================================

/**
 * MCP tool definition (from server)
 */
export interface MCPToolDef {
  name: string;
  description?: string;
  inputSchema: Record<string, unknown>; // JSON Schema
}

// =============================================================================
// OAuth Types
// =============================================================================

/**
 * OAuth client configuration
 */
export interface OAuthClientConfig {
  clientId: string;
  clientSecret?: string;
  redirectUri: string;
}

/**
 * OAuth token storage
 */
export interface OAuthTokens {
  accessToken: string;
  refreshToken?: string;
  expiresAt?: number; // Unix timestamp
  scope?: string;
}

/**
 * OAuth storage per server
 */
export interface OAuthStorage {
  [serverName: string]: {
    tokens?: OAuthTokens;
    clientConfig?: OAuthClientConfig;
  };
}

// =============================================================================
// Helper Types
// =============================================================================

/**
 * MCP server info (for CLI display)
 */
export interface MCPServerInfo {
  name: string;
  type: 'stdio' | 'http' | 'sse';
  status: string;
  enabled: boolean;
  toolCount?: number;
  error?: string;
}
