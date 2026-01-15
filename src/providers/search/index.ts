/**
 * Search Providers - Factory and exports
 */

export * from './types.js';
export { ExaProvider } from './exa.js';
export { SerperProvider } from './serper.js';
export { BraveProvider } from './brave.js';

import type { SearchProvider, SearchProviderName } from './types.js';
import { ExaProvider } from './exa.js';
import { SerperProvider } from './serper.js';
import { BraveProvider } from './brave.js';
import { getProviderStore } from '../store.js';

/**
 * Detect search provider from environment variables
 */
function detectFromEnv(): SearchProviderName | undefined {
  if (process.env.SERPER_API_KEY) return 'serper';
  if (process.env.BRAVE_API_KEY) return 'brave';
  return undefined;
}

/**
 * Create a search provider instance
 *
 * Priority:
 * 1. Explicit name parameter
 * 2. Configured in provider store
 * 3. Detected from environment variables
 * 4. Default: Exa (no key required)
 */
export function createSearchProvider(name?: SearchProviderName): SearchProvider {
  const providerName = name ?? getProviderStore().getSearchProvider() ?? detectFromEnv() ?? 'exa';

  switch (providerName) {
    case 'serper':
      return new SerperProvider();
    case 'brave':
      return new BraveProvider();
    case 'exa':
    default:
      return new ExaProvider();
  }
}

/**
 * Get the name of the current search provider
 */
export function getCurrentSearchProviderName(): SearchProviderName {
  return getProviderStore().getSearchProvider() ?? detectFromEnv() ?? 'exa';
}

/**
 * Check if a search provider is available (has required API keys)
 */
export function isSearchProviderAvailable(name: SearchProviderName): boolean {
  switch (name) {
    case 'exa':
      return true; // Always available
    case 'serper':
      return !!process.env.SERPER_API_KEY;
    case 'brave':
      return !!process.env.BRAVE_API_KEY;
    default:
      return false;
  }
}

/**
 * Get all available search providers
 */
export function getAvailableSearchProviders(): SearchProviderName[] {
  const providers: SearchProviderName[] = ['exa']; // Always available
  if (process.env.SERPER_API_KEY) providers.push('serper');
  if (process.env.BRAVE_API_KEY) providers.push('brave');
  return providers;
}
