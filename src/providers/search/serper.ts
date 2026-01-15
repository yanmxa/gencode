/**
 * Serper.dev Search Provider
 *
 * Uses Google Search via Serper.dev API.
 * Requires SERPER_API_KEY environment variable.
 */

import type { SearchProvider, SearchResult, SearchOptions } from './types.js';

const API_CONFIG = {
  BASE_URL: 'https://google.serper.dev',
  ENDPOINT: '/search',
  DEFAULT_NUM_RESULTS: 10,
  DEFAULT_TIMEOUT: 10000,
} as const;

interface SerperRequest {
  q: string;
  num?: number;
}

interface SerperOrganicResult {
  title: string;
  link: string;
  snippet: string;
  position: number;
}

interface SerperResponse {
  organic: SerperOrganicResult[];
  searchParameters: {
    q: string;
    type: string;
    num: number;
  };
}

/**
 * Filter results by allowed/blocked domains
 */
function filterByDomain(results: SearchResult[], options?: SearchOptions): SearchResult[] {
  if (!options?.allowedDomains?.length && !options?.blockedDomains?.length) {
    return results;
  }

  return results.filter((result) => {
    try {
      const domain = new URL(result.url).hostname;

      if (options.allowedDomains?.length) {
        return options.allowedDomains.some(
          (allowed) => domain === allowed || domain.endsWith('.' + allowed)
        );
      }

      if (options.blockedDomains?.length) {
        return !options.blockedDomains.some(
          (blocked) => domain === blocked || domain.endsWith('.' + blocked)
        );
      }

      return true;
    } catch {
      return true;
    }
  });
}

export class SerperProvider implements SearchProvider {
  readonly name = 'serper' as const;
  private apiKey: string;

  constructor(apiKey?: string) {
    this.apiKey = apiKey ?? process.env.SERPER_API_KEY ?? '';
    if (!this.apiKey) {
      throw new Error('SERPER_API_KEY environment variable is required for Serper provider');
    }
  }

  async search(query: string, options?: SearchOptions): Promise<SearchResult[]> {
    const searchRequest: SerperRequest = {
      q: query,
      num: options?.numResults ?? API_CONFIG.DEFAULT_NUM_RESULTS,
    };

    const controller = new AbortController();
    const timeoutId = setTimeout(
      () => controller.abort(),
      options?.timeout ?? API_CONFIG.DEFAULT_TIMEOUT
    );

    try {
      const signals = options?.abortSignal
        ? [controller.signal, options.abortSignal]
        : [controller.signal];

      const response = await fetch(`${API_CONFIG.BASE_URL}${API_CONFIG.ENDPOINT}`, {
        method: 'POST',
        headers: {
          'X-API-KEY': this.apiKey,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(searchRequest),
        signal: AbortSignal.any(signals),
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Serper search error (${response.status}): ${errorText}`);
      }

      const data = await response.json() as SerperResponse;

      const results: SearchResult[] = (data.organic || []).map((item) => ({
        title: item.title,
        url: item.link,
        snippet: item.snippet,
      }));

      return filterByDomain(results, options);
    } catch (error) {
      clearTimeout(timeoutId);

      if (error instanceof Error && error.name === 'AbortError') {
        throw new Error('Search request timed out');
      }

      throw error;
    }
  }
}
