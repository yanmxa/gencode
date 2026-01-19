/**
 * WebSearch Tool - Search the web for current information
 */

import { z } from 'zod';
import type { Tool, ToolContext, ToolResult } from '../types.js';
import { getErrorMessage } from '../types.js';
import {
  createSearchProvider,
  getCurrentSearchProviderName,
  type SearchResult,
} from '../../providers/search/index.js';
import { loadToolDescription } from '../../../cli/prompts/index.js';

// Constants
const DEFAULT_NUM_RESULTS = 10;

// Input schema
export const WebSearchInputSchema = z.object({
  query: z
    .string()
    .min(2)
    .describe('The search query (minimum 2 characters)'),
  allowed_domains: z
    .array(z.string())
    .optional()
    .describe('Only include results from these domains'),
  blocked_domains: z
    .array(z.string())
    .optional()
    .describe('Exclude results from these domains'),
  num_results: z
    .number()
    .optional()
    .describe(`Number of results to return (default: ${DEFAULT_NUM_RESULTS})`),
});
export type WebSearchInput = z.infer<typeof WebSearchInputSchema>;

/**
 * Format search results as markdown
 */
function formatResults(results: SearchResult[], query: string): string {
  if (results.length === 0) {
    return `No results found for "${query}".`;
  }

  const lines: string[] = [`Found ${results.length} results for "${query}":\n`];

  results.forEach((result, index) => {
    lines.push(`${index + 1}. [${result.title}](${result.url})`);
    if (result.snippet) {
      lines.push(`   ${result.snippet}\n`);
    } else {
      lines.push('');
    }
  });

  return lines.join('\n');
}

/**
 * WebSearch Tool
 */
export const websearchTool: Tool<WebSearchInput> = {
  name: 'WebSearch',
  description: loadToolDescription('websearch'),
  parameters: WebSearchInputSchema,

  async execute(input: WebSearchInput, context: ToolContext): Promise<ToolResult> {
    const startTime = Date.now();

    try {
      const provider = createSearchProvider();

      const results = await provider.search(input.query, {
        numResults: input.num_results ?? DEFAULT_NUM_RESULTS,
        allowedDomains: input.allowed_domains,
        blockedDomains: input.blocked_domains,
        abortSignal: context.abortSignal,
      });

      const output = formatResults(results, input.query);
      const duration = Date.now() - startTime;
      const providerName = getCurrentSearchProviderName();

      return {
        success: true,
        output,
        metadata: {
          title: `Search("${input.query}")`,
          subtitle: `Found ${results.length} results via ${providerName}`,
          duration,
        },
      };
    } catch (error) {
      return {
        success: false,
        output: '',
        error: `Search failed: ${getErrorMessage(error)}`,
      };
    }
  },
};
