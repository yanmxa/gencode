/**
 * Search Provider Types
 */

export type SearchProviderName = 'exa' | 'serper' | 'brave';

export interface SearchResult {
  title: string;
  url: string;
  snippet: string;
}

export interface SearchOptions {
  numResults?: number;
  allowedDomains?: string[];
  blockedDomains?: string[];
  timeout?: number;
  abortSignal?: AbortSignal;
}

export interface SearchProvider {
  readonly name: SearchProviderName;
  search(query: string, options?: SearchOptions): Promise<SearchResult[]>;
}
