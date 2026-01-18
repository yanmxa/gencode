/**
 * MCP Integration - Model Context Protocol
 * External tools and data sources via MCP servers
 */

// Types
export type {
  MCPConfig,
  MCPServerConfig,
  MCPStdioConfig,
  MCPHTTPConfig,
  MCPSSEConfig,
  MCPStatus,
  MCPServer,
  MCPServerInfo,
  MCPToolDef,
  OAuthClientConfig,
  OAuthTokens,
  OAuthStorage,
} from './types.js';

// Configuration
export {
  loadMCPConfig,
  saveMCPConfig,
  addMCPServer,
  removeMCPServer,
} from './config.js';

// Manager
export { MCPManager, getMCPManager } from './manager.js';

// Client
export { createMCPClient, getServerTimeout } from './client.js';

// Connection
export { connectToServer, disconnectFromServer } from './connection.js';

// Bridge
export { bridgeMCPTool, bridgeMCPTools } from './bridge.js';

// Auth
export {
  getOAuthTokens,
  saveOAuthTokens,
  getOAuthClientConfig,
  saveOAuthClientConfig,
  removeOAuthData,
  areTokensExpired,
  hasValidTokens,
} from './auth.js';

// OAuth
export { MCPOAuthProvider, performOAuthFlow } from './oauth-provider.js';
export {
  startOAuthCallbackServer,
  getOAuthCallbackURL,
  generateState,
} from './oauth-callback.js';

// Environment expansion
export { expandEnvVars, expandEnvVarsInObject, expandServerConfig } from './env-expand.js';
