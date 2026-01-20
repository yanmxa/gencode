/**
 * Provider Registry - Provider class registry with metadata
 */

import type { Provider, AuthMethod, ProviderClassMeta, LLMProvider } from './types.js';
import type { SearchProviderName } from './search/types.js';
import { AnthropicProvider } from './anthropic.js';
import { AnthropicVertexProvider } from './anthropic-vertex.js';
import { OpenAIProvider } from './openai.js';
import { GoogleProvider } from './google.js';

// ============================================================================
// LLM Provider Classes
// ============================================================================

export type ProviderClass = {
  new (config?: any): LLMProvider;
  meta: ProviderClassMeta;
};

/**
 * All registered provider classes
 * Each class has static metadata describing which provider and auth method it implements
 */
export const PROVIDER_CLASSES: ProviderClass[] = [
  AnthropicProvider,
  AnthropicVertexProvider,
  OpenAIProvider,
  GoogleProvider,
];

/**
 * Provider metadata (for UI display)
 */
export interface ProviderMeta {
  id: Provider;
  name: string;
  popularity: number; // Lower = more popular
}

export const PROVIDER_METADATA: ProviderMeta[] = [
  { id: 'anthropic', name: 'Anthropic', popularity: 1 },
  { id: 'openai', name: 'OpenAI', popularity: 2 },
  { id: 'google', name: 'Google', popularity: 3 },
];

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Get a specific provider class by provider and auth method
 */
export function getProviderClass(
  provider: Provider,
  authMethod: AuthMethod
): ProviderClass | undefined {
  return PROVIDER_CLASSES.find(
    (cls) => cls.meta.provider === provider && cls.meta.authMethod === authMethod
  );
}

/**
 * Get all provider classes for a given provider
 */
export function getProviderClasses(provider: Provider): ProviderClass[] {
  return PROVIDER_CLASSES.filter((cls) => cls.meta.provider === provider);
}

/**
 * Check if a provider class is ready (has required env vars)
 */
export function isProviderReady(providerClass: ProviderClass): boolean {
  // Special case for Vertex AI: requires explicit opt-in
  if (providerClass.meta.authMethod === 'vertex') {
    const useVertex = process.env.CLAUDE_CODE_USE_VERTEX === '1' || process.env.CLAUDE_CODE_USE_VERTEX === 'true';
    const hasProject = !!(
      process.env.ANTHROPIC_VERTEX_PROJECT_ID ||
      process.env.GCLOUD_PROJECT ||
      process.env.GOOGLE_CLOUD_PROJECT
    );
    return useVertex && hasProject;
  }

  // For other providers: check if any required env var is set
  return providerClass.meta.envVars.some((v) => !!process.env[v]);
}

/**
 * Get provider metadata by ID
 */
export function getProviderMeta(id: Provider): ProviderMeta | undefined {
  return PROVIDER_METADATA.find((p) => p.id === id);
}

/**
 * Get all providers sorted by popularity
 */
export function getProvidersSorted(): ProviderMeta[] {
  return [...PROVIDER_METADATA].sort((a, b) => a.popularity - b.popularity);
}

/**
 * Get the first available (ready) provider class for a provider
 */
export function getAvailableProviderClass(provider: Provider): ProviderClass | undefined {
  const classes = getProviderClasses(provider);
  return classes.find((cls) => isProviderReady(cls));
}

/**
 * Get all available (ready) provider classes for a provider
 */
export function getAvailableProviderClasses(provider: Provider): ProviderClass[] {
  return getProviderClasses(provider).filter((cls) => isProviderReady(cls));
}

// ============================================================================
// Legacy Support (for backward compatibility with search providers)
// ============================================================================

export interface ConnectionOption {
  name: string;
  envVars: string[];
  description?: string;
}

export interface SearchProviderDefinition {
  id: SearchProviderName;
  name: string;
  popularity: number;
  connections: ConnectionOption[];
  requiresKey: boolean;
}

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
        name: 'API Key',
        envVars: ['BRAVE_API_KEY'],
        description: 'Privacy-focused search',
      },
    ],
    requiresKey: true,
  },
];

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

// Helper: check if any env var in the list is set
const hasAnyEnvVar = (envVars: string[]) => envVars.some((v) => !!process.env[v]);

/**
 * Check if a specific connection option has its env vars set
 */
export function isConnectionReady(conn: ConnectionOption): boolean {
  return hasAnyEnvVar(conn.envVars);
}
