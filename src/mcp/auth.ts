/**
 * MCP OAuth Authentication Storage
 * Manages OAuth tokens and client configuration
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import type { OAuthStorage, OAuthTokens, OAuthClientConfig } from './types.js';

const AUTH_FILE = path.join(os.homedir(), '.gen', 'mcp-auth.json');

/**
 * Load OAuth storage from disk
 */
async function loadAuthStorage(): Promise<OAuthStorage> {
  try {
    const content = await fs.readFile(AUTH_FILE, 'utf-8');
    return JSON.parse(content) as OAuthStorage;
  } catch {
    return {};
  }
}

/**
 * Save OAuth storage to disk
 */
async function saveAuthStorage(storage: OAuthStorage): Promise<void> {
  const dir = path.dirname(AUTH_FILE);
  await fs.mkdir(dir, { recursive: true });

  // Write with secure permissions (owner read/write only)
  await fs.writeFile(AUTH_FILE, JSON.stringify(storage, null, 2), {
    mode: 0o600,
    encoding: 'utf-8',
  });
}

/**
 * Get OAuth tokens for a server
 */
export async function getOAuthTokens(serverName: string): Promise<OAuthTokens | null> {
  const storage = await loadAuthStorage();
  return storage[serverName]?.tokens ?? null;
}

/**
 * Save OAuth tokens for a server
 */
export async function saveOAuthTokens(
  serverName: string,
  tokens: OAuthTokens
): Promise<void> {
  const storage = await loadAuthStorage();

  if (!storage[serverName]) {
    storage[serverName] = {};
  }

  storage[serverName].tokens = tokens;
  await saveAuthStorage(storage);
}

/**
 * Get OAuth client config for a server
 */
export async function getOAuthClientConfig(
  serverName: string
): Promise<OAuthClientConfig | null> {
  const storage = await loadAuthStorage();
  return storage[serverName]?.clientConfig ?? null;
}

/**
 * Save OAuth client config for a server
 */
export async function saveOAuthClientConfig(
  serverName: string,
  clientConfig: OAuthClientConfig
): Promise<void> {
  const storage = await loadAuthStorage();

  if (!storage[serverName]) {
    storage[serverName] = {};
  }

  storage[serverName].clientConfig = clientConfig;
  await saveAuthStorage(storage);
}

/**
 * Remove OAuth data for a server (logout)
 */
export async function removeOAuthData(serverName: string): Promise<void> {
  const storage = await loadAuthStorage();

  delete storage[serverName];
  await saveAuthStorage(storage);
}

/**
 * Check if tokens are expired
 */
export function areTokensExpired(tokens: OAuthTokens): boolean {
  if (!tokens.expiresAt) {
    return false; // No expiration time - assume valid
  }

  // Add 5 minute buffer
  const now = Date.now();
  return now >= tokens.expiresAt - 5 * 60 * 1000;
}

/**
 * Check if tokens are valid (exist and not expired)
 */
export async function hasValidTokens(serverName: string): Promise<boolean> {
  const tokens = await getOAuthTokens(serverName);
  if (!tokens) {
    return false;
  }

  return !areTokensExpired(tokens);
}
