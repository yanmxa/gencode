/**
 * Brave Search Provider
 *
 * Uses Brave Search API (same as Claude Code).
 * Requires BRAVE_API_KEY environment variable.
 */

import type { SearchProvider, SearchResult, SearchOptions } from './types.js';

const API_CONFIG = {
  BASE_URL: 'https://api.search.brave.com',
  ENDPOINT: '/res/v1/web/search',
  DEFAULT_NUM_RESULTS: 10,
  DEFAULT_TIMEOUT: 10000,
} as const;

interface BraveWebResult {
  title: string;
  url: string;
  description: string;
  is_source_local?: boolean;
  is_source_both?: boolean;
}

interface BraveResponse {
  web?: {
    results: BraveWebResult[];
  };
  query?: {
    original: string;
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

export class BraveProvider implements SearchProvider {
  readonly name = 'brave' as const;
  private apiKey: string;

  constructor(apiKey?: string) {
    this.apiKey = apiKey ?? process.env.BRAVE_API_KEY ?? '';
    if (!this.apiKey) {
      throw new Error('BRAVE_API_KEY environment variable is required for Brave provider');
    }
  }

  async search(query: string, options?: SearchOptions): Promise<SearchResult[]> {
    const params = new URLSearchParams({
      q: query,
      count: String(options?.numResults ?? API_CONFIG.DEFAULT_NUM_RESULTS),
    });

    const controller = new AbortController();
    const timeoutId = setTimeout(
      () => controller.abort(),
      options?.timeout ?? API_CONFIG.DEFAULT_TIMEOUT
    );

    try {
      const signals = options?.abortSignal
        ? [controller.signal, options.abortSignal]
        : [controller.signal];

      const response = await fetch(
        `${API_CONFIG.BASE_URL}${API_CONFIG.ENDPOINT}?${params.toString()}`,
        {
          method: 'GET',
          headers: {
            Accept: 'application/json',
            'Accept-Encoding': 'gzip',
            'X-Subscription-Token': this.apiKey,
          },
          signal: AbortSignal.any(signals),
        }
      );

      clearTimeout(timeoutId);

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Brave search error (${response.status}): ${errorText}`);
      }

      const data = (await response.json()) as BraveResponse;

      const results: SearchResult[] = (data.web?.results || []).map((item) => ({
        title: item.title,
        url: item.url,
        snippet: item.description,
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
