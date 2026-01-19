/**
 * OAuth Provider for MCP
 * Implements OAuth client provider for MCP SDK
 * Note: This is a placeholder for future OAuth implementation
 */

import type { OAuthClientConfig, OAuthTokens } from './types.js';
import {
  getOAuthTokens,
  saveOAuthTokens,
  getOAuthClientConfig,
  saveOAuthClientConfig,
  areTokensExpired,
} from './auth.js';
import {
  startOAuthCallbackServer,
  getOAuthCallbackURL,
  generateState,
} from './oauth-callback.js';

/**
 * OAuth provider for MCP servers
 * Manages OAuth authentication flow and token storage
 */
export class MCPOAuthProvider {
  private serverName: string;

  constructor(serverName: string) {
    this.serverName = serverName;
  }

  /**
   * Get stored tokens
   */
  async getTokens(): Promise<OAuthTokens | null> {
    return getOAuthTokens(this.serverName);
  }

  /**
   * Check if we have valid tokens
   */
  async hasValidTokens(): Promise<boolean> {
    const tokens = await this.getTokens();
    if (!tokens) {
      return false;
    }
    return !areTokensExpired(tokens);
  }

  /**
   * Get client configuration
   */
  async getClientConfig(): Promise<OAuthClientConfig | null> {
    return getOAuthClientConfig(this.serverName);
  }

  /**
   * Start OAuth flow and return authorization URL
   */
  async startAuthFlow(authorizationUrl: string, clientConfig: OAuthClientConfig): Promise<string> {
    // Save client config for later
    await saveOAuthClientConfig(this.serverName, clientConfig);

    // Generate state for CSRF protection
    const state = generateState();

    // Build authorization URL
    const url = new URL(authorizationUrl);
    url.searchParams.set('client_id', clientConfig.clientId);
    url.searchParams.set('redirect_uri', clientConfig.redirectUri);
    url.searchParams.set('response_type', 'code');
    url.searchParams.set('state', state);

    return url.toString();
  }

  /**
   * Complete OAuth flow by exchanging code for tokens
   */
  async completeAuthFlow(
    tokenUrl: string,
    authorizationCode: string
  ): Promise<OAuthTokens> {
    const clientConfig = await this.getClientConfig();
    if (!clientConfig) {
      throw new Error('No client configuration found');
    }

    // Exchange code for tokens
    const response = await fetch(tokenUrl, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: new URLSearchParams({
        grant_type: 'authorization_code',
        code: authorizationCode,
        redirect_uri: clientConfig.redirectUri,
        client_id: clientConfig.clientId,
        ...(clientConfig.clientSecret ? { client_secret: clientConfig.clientSecret } : {}),
      }),
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(`Token exchange failed: ${error}`);
    }

    const data = await response.json() as any;

    const tokens: OAuthTokens = {
      accessToken: data.access_token,
      refreshToken: data.refresh_token,
      expiresAt: data.expires_in ? Date.now() + data.expires_in * 1000 : undefined,
      scope: data.scope,
    };

    // Save tokens
    await saveOAuthTokens(this.serverName, tokens);

    return tokens;
  }

  /**
   * Refresh expired tokens
   */
  async refreshTokens(tokenUrl: string): Promise<OAuthTokens> {
    const tokens = await this.getTokens();
    if (!tokens?.refreshToken) {
      throw new Error('No refresh token available');
    }

    const clientConfig = await this.getClientConfig();
    if (!clientConfig) {
      throw new Error('No client configuration found');
    }

    // Refresh tokens
    const response = await fetch(tokenUrl, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: new URLSearchParams({
        grant_type: 'refresh_token',
        refresh_token: tokens.refreshToken,
        client_id: clientConfig.clientId,
        ...(clientConfig.clientSecret ? { client_secret: clientConfig.clientSecret } : {}),
      }),
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(`Token refresh failed: ${error}`);
    }

    const data = await response.json() as any;

    const newTokens: OAuthTokens = {
      accessToken: data.access_token,
      refreshToken: data.refresh_token ?? tokens.refreshToken, // Keep old refresh token if new one not provided
      expiresAt: data.expires_in ? Date.now() + data.expires_in * 1000 : undefined,
      scope: data.scope,
    };

    // Save new tokens
    await saveOAuthTokens(this.serverName, newTokens);

    return newTokens;
  }
}

/**
 * Perform interactive OAuth flow
 * Opens browser and waits for callback
 */
export async function performOAuthFlow(
  serverName: string,
  authorizationUrl: string,
  tokenUrl: string,
  clientConfig?: OAuthClientConfig
): Promise<OAuthTokens> {
  const provider = new MCPOAuthProvider(serverName);

  // Use provided client config or default
  const config = clientConfig ?? {
    clientId: 'gencode',
    redirectUri: getOAuthCallbackURL(),
  };

  // Start auth flow
  const authUrl = await provider.startAuthFlow(authorizationUrl, config);

  console.log('\nOpening browser for authentication...');
  console.log(`If browser doesn't open, visit: ${authUrl}\n`);

  // Open browser (platform-specific)
  const { default: open } = await import('open');
  await open(authUrl);

  // Start callback server
  const state = generateState();
  const result = await startOAuthCallbackServer(state);

  // Exchange code for tokens
  const tokens = await provider.completeAuthFlow(tokenUrl, result.code);

  console.log('Authentication successful!\n');

  return tokens;
}
