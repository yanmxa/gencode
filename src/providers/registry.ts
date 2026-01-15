/**
 * Provider Registry - Provider definitions with connection options
 */

import type { ProviderName } from './index.js';

export interface ConnectionOption {
  method: string;
  name: string;
  envVars: string[];
  description?: string;
  providerImpl?: ProviderName; // Override provider implementation (e.g., vertex-ai for Anthropic via GCP)
}

export interface ProviderDefinition {
  id: ProviderName;
  name: string;
  popularity: number; // Lower = more popular, used for sorting
  connections: ConnectionOption[];
}

/**
 * All supported providers with their connection options
 */
export const PROVIDERS: ProviderDefinition[] = [
  {
    id: 'anthropic',
    name: 'Anthropic',
    popularity: 1,
    connections: [
      {
        method: 'api_key',
        name: 'API Key',
        envVars: ['ANTHROPIC_API_KEY'],
        description: 'Direct API access',
      },
      {
        method: 'vertex',
        name: 'Google Vertex AI',
        envVars: ['ANTHROPIC_VERTEX_PROJECT_ID', 'GOOGLE_CLOUD_PROJECT'],
        description: 'Claude via GCP',
        providerImpl: 'vertex-ai',
      },
      {
        method: 'bedrock',
        name: 'Amazon Bedrock',
        envVars: ['AWS_ACCESS_KEY_ID', 'AWS_PROFILE'],
        description: 'Claude via AWS (coming soon)',
      },
    ],
  },
  {
    id: 'openai',
    name: 'OpenAI',
    popularity: 2,
    connections: [
      {
        method: 'api_key',
        name: 'API Key',
        envVars: ['OPENAI_API_KEY'],
        description: 'Direct API access',
      },
    ],
  },
  {
    id: 'gemini',
    name: 'Google Gemini',
    popularity: 3,
    connections: [
      {
        method: 'api_key',
        name: 'API Key',
        envVars: ['GOOGLE_API_KEY', 'GEMINI_API_KEY'],
        description: 'Direct API access',
      },
    ],
  },
];

/**
 * Get provider definition by ID
 */
export function getProvider(id: ProviderName): ProviderDefinition | undefined {
  return PROVIDERS.find((p) => p.id === id);
}

/**
 * Get all providers sorted by popularity
 */
export function getProvidersSorted(): ProviderDefinition[] {
  return [...PROVIDERS].sort((a, b) => a.popularity - b.popularity);
}

/**
 * Check if any of the provider's env vars are set
 */
export function hasEnvVars(provider: ProviderDefinition): boolean {
  return provider.connections.some((conn) =>
    conn.envVars.some((envVar) => !!process.env[envVar])
  );
}

/**
 * Get the first available connection method (where env vars are set)
 */
export function getAvailableConnection(
  provider: ProviderDefinition
): ConnectionOption | undefined {
  return provider.connections.find((conn) =>
    conn.envVars.some((envVar) => !!process.env[envVar])
  );
}

/**
 * Check if a specific connection option has its env vars set
 */
export function isConnectionReady(conn: ConnectionOption): boolean {
  return conn.envVars.some((envVar) => !!process.env[envVar]);
}

/**
 * Get all available (ready) connections for a provider
 */
export function getAvailableConnections(
  provider: ProviderDefinition
): ConnectionOption[] {
  return provider.connections.filter((conn) =>
    conn.envVars.some((envVar) => !!process.env[envVar])
  );
}
