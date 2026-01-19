/**
 * Exa AI Search Provider
 *
 * Uses Exa's public MCP endpoint (no API key required).
 * Based on OpenCode's implementation pattern.
 */

import type { SearchProvider, SearchResult, SearchOptions } from './types.js';

const API_CONFIG = {
  BASE_URL: 'https://mcp.exa.ai',
  ENDPOINT: '/mcp',
  DEFAULT_NUM_RESULTS: 8,
  DEFAULT_TIMEOUT: 25000,
} as const;

interface McpSearchRequest {
  jsonrpc: string;
  id: number;
  method: string;
  params: {
    name: string;
    arguments: {
      query: string;
      numResults?: number;
      livecrawl?: 'fallback' | 'preferred';
      type?: 'auto' | 'fast' | 'deep';
      contextMaxCharacters?: number;
    };
  };
}

interface McpSearchResponse {
  jsonrpc: string;
  result: {
    content: Array<{
      type: string;
      text: string;
    }>;
  };
}

/**
 * Parse Exa's response text into structured search results
 *
 * Exa returns results in this format:
 * Title: ...
 * URL: ...
 * Text: ...
 */
function parseExaResults(text: string): SearchResult[] {
  const results: SearchResult[] = [];
  const lines = text.split('\n');

  let currentResult: Partial<SearchResult> = {};
  let collectingText = false;
  let textBuffer: string[] = [];

  for (const line of lines) {
    const trimmed = line.trim();

    // Check for "Title: ..." line
    if (trimmed.startsWith('Title:')) {
      // Save previous result if exists
      if (currentResult.title && currentResult.url) {
        results.push({
          title: currentResult.title,
          url: currentResult.url,
          snippet: textBuffer.join(' ').trim().substring(0, 300),
        });
      }
      currentResult = { title: trimmed.substring(6).trim() };
      collectingText = false;
      textBuffer = [];
      continue;
    }

    // Check for "URL: ..." line
    if (trimmed.startsWith('URL:')) {
      currentResult.url = trimmed.substring(4).trim();
      continue;
    }

    // Check for "Text: ..." line - start of snippet
    if (trimmed.startsWith('Text:')) {
      collectingText = true;
      const initialText = trimmed.substring(5).trim();
      if (initialText) {
        textBuffer.push(initialText);
      }
      continue;
    }

    // Collect text lines until next Title
    if (collectingText && trimmed && !trimmed.startsWith('Title:')) {
      textBuffer.push(trimmed);
    }
  }

  // Add the last result
  if (currentResult.title && currentResult.url) {
    results.push({
      title: currentResult.title,
      url: currentResult.url,
      snippet: textBuffer.join(' ').trim().substring(0, 300),
    });
  }

  return results;
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

export class ExaProvider implements SearchProvider {
  readonly name = 'exa' as const;

  async search(query: string, options?: SearchOptions): Promise<SearchResult[]> {
    const searchRequest: McpSearchRequest = {
      jsonrpc: '2.0',
      id: 1,
      method: 'tools/call',
      params: {
        name: 'web_search_exa',
        arguments: {
          query,
          type: 'auto',
          numResults: options?.numResults ?? API_CONFIG.DEFAULT_NUM_RESULTS,
          livecrawl: 'fallback',
          contextMaxCharacters: 10000,
        },
      },
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
          Accept: 'application/json, text/event-stream',
          'Content-Type': 'application/json',
          // Explicitly omit authentication (Exa's public endpoint)
          'X-Api-Key': '',
          Authorization: '',
        },
        body: JSON.stringify(searchRequest),
        signal: AbortSignal.any(signals),
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Exa search error (${response.status}): ${errorText}`);
      }

      const responseText = await response.text();

      // Parse SSE response
      const lines = responseText.split('\n');
      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const data: McpSearchResponse = JSON.parse(line.substring(6));
          if (data.result?.content?.length > 0) {
            const text = data.result.content[0].text;
            const results = parseExaResults(text);
            return filterByDomain(results, options);
          }
        }
      }

      return [];
    } catch (error) {
      clearTimeout(timeoutId);

      if (error instanceof Error && error.name === 'AbortError') {
        throw new Error('Search request timed out');
      }

      throw error;
    }
  }
}
