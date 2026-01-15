/**
 * WebFetch Tool - Fetch and convert web content
 */

import TurndownService from 'turndown';
import { z } from 'zod';
import type { Tool, ToolContext, ToolResult } from '../types.js';
import { getErrorMessage } from '../types.js';
import { validateUrl } from '../utils/ssrf.js';

// Constants
const MAX_RESPONSE_SIZE = 5 * 1024 * 1024; // 5MB
const DEFAULT_TIMEOUT = 30 * 1000; // 30 seconds
const MAX_TIMEOUT = 120 * 1000; // 2 minutes
const MAX_LINE_LENGTH = 2000;
const MAX_OUTPUT_LENGTH = 50000;

// Input schema
export const WebFetchInputSchema = z.object({
  url: z.string().describe('The URL to fetch content from (http:// or https://)'),
  format: z
    .enum(['text', 'markdown', 'html'])
    .optional()
    .describe('Output format: markdown (default), text, or html'),
  timeout: z.number().optional().describe('Timeout in seconds (default: 30, max: 120)'),
});
export type WebFetchInput = z.infer<typeof WebFetchInputSchema>;

/**
 * Get Accept header based on requested format
 */
function getAcceptHeader(format: string): string {
  switch (format) {
    case 'markdown':
      return 'text/markdown, text/plain, text/html;q=0.9, */*;q=0.1';
    case 'text':
      return 'text/plain, text/html;q=0.8, */*;q=0.1';
    case 'html':
      return 'text/html, application/xhtml+xml, */*;q=0.1';
    default:
      return 'text/html, */*;q=0.1';
  }
}

/**
 * Convert HTML to Markdown using Turndown
 */
function convertHtmlToMarkdown(html: string): string {
  const turndown = new TurndownService({
    headingStyle: 'atx',
    hr: '---',
    bulletListMarker: '-',
    codeBlockStyle: 'fenced',
    emDelimiter: '*',
  });

  // Remove script, style, meta, link, noscript tags
  turndown.remove(['script', 'style', 'meta', 'link', 'noscript']);

  return turndown.turndown(html);
}

/**
 * Extract plain text from HTML
 */
function extractTextFromHtml(html: string): string {
  return (
    html
      // Remove script and style content
      .replace(/<script[^>]*>[\s\S]*?<\/script>/gi, '')
      .replace(/<style[^>]*>[\s\S]*?<\/style>/gi, '')
      .replace(/<noscript[^>]*>[\s\S]*?<\/noscript>/gi, '')
      // Remove all tags
      .replace(/<[^>]+>/g, ' ')
      // Decode common HTML entities
      .replace(/&nbsp;/g, ' ')
      .replace(/&lt;/g, '<')
      .replace(/&gt;/g, '>')
      .replace(/&amp;/g, '&')
      .replace(/&quot;/g, '"')
      .replace(/&#(\d+);/g, (_, num) => String.fromCharCode(parseInt(num)))
      // Normalize whitespace
      .replace(/\s+/g, ' ')
      .trim()
  );
}

/**
 * Process content based on content type and requested format
 */
function processContent(content: string, contentType: string, format: string): string {
  const isHtml = contentType.includes('text/html') || contentType.includes('application/xhtml');

  switch (format) {
    case 'markdown':
      if (isHtml) {
        return convertHtmlToMarkdown(content);
      }
      return content;

    case 'text':
      if (isHtml) {
        return extractTextFromHtml(content);
      }
      return content;

    case 'html':
      return content;

    default:
      return content;
  }
}

/**
 * Format bytes to human-readable size
 */
function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes}B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
}

/**
 * Truncate output to prevent excessive content
 */
function truncateOutput(output: string): string {
  // Truncate long lines
  const lines = output.split('\n').map((line) => {
    if (line.length > MAX_LINE_LENGTH) {
      return line.slice(0, MAX_LINE_LENGTH) + '... (truncated)';
    }
    return line;
  });

  let result = lines.join('\n');

  // Truncate overall output
  if (result.length > MAX_OUTPUT_LENGTH) {
    result = result.slice(0, MAX_OUTPUT_LENGTH) + '\n\n... (output truncated)';
  }

  return result;
}

/**
 * WebFetch Tool
 */
export const webfetchTool: Tool<WebFetchInput> = {
  name: 'WebFetch',
  description: `Fetch content from a URL and return it in the specified format.
- Converts HTML to Markdown by default for easier reading
- Supports text, markdown, and html output formats
- Maximum response size: 5MB
- Timeout: 30 seconds (configurable up to 120 seconds)`,
  parameters: WebFetchInputSchema,

  async execute(input: WebFetchInput, context: ToolContext): Promise<ToolResult> {
    const startTime = Date.now();

    try {
      // Validate URL (SSRF protection)
      validateUrl(input.url);

      // Calculate timeout
      const timeoutMs = input.timeout
        ? Math.min(input.timeout * 1000, MAX_TIMEOUT)
        : DEFAULT_TIMEOUT;

      // Create abort controller for timeout
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

      // Combine with context abort signal if present
      const signal = context.abortSignal
        ? AbortSignal.any([controller.signal, context.abortSignal])
        : controller.signal;

      try {
        // Fetch with appropriate headers
        const response = await fetch(input.url, {
          signal,
          headers: {
            'User-Agent': 'GenCode/1.0 (+https://github.com/gencode)',
            Accept: getAcceptHeader(input.format ?? 'markdown'),
            'Accept-Language': 'en-US,en;q=0.9',
          },
          redirect: 'follow',
        });

        clearTimeout(timeoutId);

        if (!response.ok) {
          return {
            success: false,
            output: '',
            error: `HTTP ${response.status}: ${response.statusText}`,
          };
        }

        // Check content length header
        const contentLength = response.headers.get('content-length');
        if (contentLength && parseInt(contentLength) > MAX_RESPONSE_SIZE) {
          return {
            success: false,
            output: '',
            error: `Response too large: ${contentLength} bytes (max: ${MAX_RESPONSE_SIZE})`,
          };
        }

        // Read response body with size limit
        const arrayBuffer = await response.arrayBuffer();
        if (arrayBuffer.byteLength > MAX_RESPONSE_SIZE) {
          return {
            success: false,
            output: '',
            error: `Response too large: ${arrayBuffer.byteLength} bytes (max: ${MAX_RESPONSE_SIZE})`,
          };
        }

        const content = new TextDecoder().decode(arrayBuffer);
        const contentType = response.headers.get('content-type') || '';

        // Process content based on format
        let output = processContent(content, contentType, input.format ?? 'markdown');

        // Truncate long lines and overall output
        output = truncateOutput(output);

        // Build result with metadata for improved display
        const size = arrayBuffer.byteLength;
        const duration = Date.now() - startTime;

        return {
          success: true,
          output: output,
          metadata: {
            title: `Fetch(${input.url})`,
            subtitle: `Received ${formatSize(size)} (${response.status} ${response.statusText})`,
            size,
            statusCode: response.status,
            contentType: contentType,
            duration,
          },
        };
      } finally {
        clearTimeout(timeoutId);
      }
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        return {
          success: false,
          output: '',
          error: 'Request timed out',
        };
      }
      return {
        success: false,
        output: '',
        error: `Fetch failed: ${getErrorMessage(error)}`,
      };
    }
  },
};
