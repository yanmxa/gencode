/**
 * Provider Registry - Provider definitions with connection options
 */

import type { ProviderName } from './index.js';
import type { SearchProviderName } from './search/types.js';

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

export interface SearchProviderDefinition {
  id: SearchProviderName;
  name: string;
  popularity: number;
  connections: ConnectionOption[];
  requiresKey: boolean;
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
 * All supported search providers
 */
export const SEARCH_PROVIDERS: SearchProviderDefinition[] = [
  {
    id: 'exa',
    name: 'Exa AI',
    popularity: 1,
    connections: [
      {
        method: 'public',
        name: 'Public API',
        envVars: [],
        description: 'No API key required',
      },
    ],
    requiresKey: false,
  },
  {
    id: 'serper',
    name: 'Serper.dev',
    popularity: 2,
    connections: [
      {
        method: 'api_key',
        name: 'API Key',
        envVars: ['SERPER_API_KEY'],
        description: 'Google Search via Serper',
      },
    ],
    requiresKey: true,
  },
  {
    id: 'brave',
    name: 'Brave Search',
    popularity: 3,
    connections: [
      {
        method: 'api_key',
        name: 'API Key',
        envVars: ['BRAVE_API_KEY'],
        description: 'Privacy-focused search',
      },
    ],
    requiresKey: true,
  },
];

/**
 * Get provider definition by ID
 */
export function getProvider(id: ProviderName): ProviderDefinition | undefined {
  return PROVIDERS.find((p) => p.id === id);
}

/**
 * Get search provider definition by ID
 */
export function getSearchProvider(id: SearchProviderName): SearchProviderDefinition | undefined {
  return SEARCH_PROVIDERS.find((p) => p.id === id);
}

/**
 * Get all search providers sorted by popularity
 */
export function getSearchProvidersSorted(): SearchProviderDefinition[] {
  return [...SEARCH_PROVIDERS].sort((a, b) => a.popularity - b.popularity);
}

/**
 * Get all providers sorted by popularity
 */
export function getProvidersSorted(): ProviderDefinition[] {
  return [...PROVIDERS].sort((a, b) => a.popularity - b.popularity);
}

// Helper: check if any env var in the list is set
const hasAnyEnvVar = (envVars: string[]) => envVars.some((v) => !!process.env[v]);

/**
 * Check if any of the provider's env vars are set
 */
export function hasEnvVars(provider: ProviderDefinition): boolean {
  return provider.connections.some((conn) => hasAnyEnvVar(conn.envVars));
}

/**
 * Get the first available connection method (where env vars are set)
 */
export function getAvailableConnection(
  provider: ProviderDefinition
): ConnectionOption | undefined {
  return provider.connections.find((conn) => hasAnyEnvVar(conn.envVars));
}

/**
 * Check if a specific connection option has its env vars set
 */
export function isConnectionReady(conn: ConnectionOption): boolean {
  return hasAnyEnvVar(conn.envVars);
}

/**
 * Get all available (ready) connections for a provider
 */
export function getAvailableConnections(
  provider: ProviderDefinition
): ConnectionOption[] {
  return provider.connections.filter((conn) => hasAnyEnvVar(conn.envVars));
}
