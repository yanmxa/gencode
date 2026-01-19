/**
 * OAuth Callback Server
 * HTTP server for handling OAuth redirect callbacks
 */

import * as http from 'http';

const CALLBACK_PORT = 19876;
const CALLBACK_PATH = '/oauth/callback';
const TIMEOUT_MS = 5 * 60 * 1000; // 5 minutes

export interface OAuthCallbackResult {
  code: string;
  state: string;
}

/**
 * Start OAuth callback server and wait for authorization code
 */
export async function startOAuthCallbackServer(
  expectedState: string
): Promise<OAuthCallbackResult> {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      server.close();
      reject(new Error('OAuth callback timeout (5 minutes)'));
    }, TIMEOUT_MS);

    const server = http.createServer((req, res) => {
      // Only handle callback path
      if (!req.url?.startsWith(CALLBACK_PATH)) {
        res.writeHead(404, { 'Content-Type': 'text/plain' });
        res.end('Not Found');
        return;
      }

      // Parse query parameters
      const url = new URL(req.url, `http://localhost:${CALLBACK_PORT}`);
      const code = url.searchParams.get('code');
      const state = url.searchParams.get('state');
      const error = url.searchParams.get('error');
      const errorDescription = url.searchParams.get('error_description');

      // Check for error
      if (error) {
        res.writeHead(400, { 'Content-Type': 'text/html' });
        res.end(`
          <html>
            <head><title>Authentication Failed</title></head>
            <body>
              <h1>Authentication Failed</h1>
              <p>${errorDescription ?? error}</p>
              <p>You can close this window.</p>
            </body>
          </html>
        `);

        clearTimeout(timeout);
        server.close();
        reject(new Error(`OAuth error: ${errorDescription ?? error}`));
        return;
      }

      // Validate state parameter (CSRF protection)
      if (state !== expectedState) {
        res.writeHead(400, { 'Content-Type': 'text/html' });
        res.end(`
          <html>
            <head><title>Invalid State</title></head>
            <body>
              <h1>Invalid State</h1>
              <p>State parameter mismatch. Possible CSRF attack.</p>
              <p>You can close this window.</p>
            </body>
          </html>
        `);

        clearTimeout(timeout);
        server.close();
        reject(new Error('State parameter mismatch'));
        return;
      }

      // Validate code
      if (!code) {
        res.writeHead(400, { 'Content-Type': 'text/html' });
        res.end(`
          <html>
            <head><title>Missing Code</title></head>
            <body>
              <h1>Missing Authorization Code</h1>
              <p>No authorization code received.</p>
              <p>You can close this window.</p>
            </body>
          </html>
        `);

        clearTimeout(timeout);
        server.close();
        reject(new Error('No authorization code received'));
        return;
      }

      // Success
      res.writeHead(200, { 'Content-Type': 'text/html' });
      res.end(`
        <html>
          <head><title>Authentication Successful</title></head>
          <body>
            <h1>Authentication Successful</h1>
            <p>You can close this window and return to the terminal.</p>
          </body>
        </html>
      `);

      clearTimeout(timeout);
      server.close();
      resolve({ code, state });
    });

    server.listen(CALLBACK_PORT, () => {
      console.log(`OAuth callback server listening on http://localhost:${CALLBACK_PORT}${CALLBACK_PATH}`);
    });

    server.on('error', (error) => {
      clearTimeout(timeout);
      reject(error);
    });
  });
}

/**
 * Get OAuth callback URL
 */
export function getOAuthCallbackURL(): string {
  return `http://localhost:${CALLBACK_PORT}${CALLBACK_PATH}`;
}

/**
 * Generate random state parameter for CSRF protection
 */
export function generateState(): string {
  // Generate 32 random bytes and convert to hex
  const randomBytes = Array.from({ length: 32 }, () =>
    Math.floor(Math.random() * 256)
  );
  return Buffer.from(randomBytes).toString('hex');
}
