/**
 * Shared Stream Utilities
 *
 * Common utilities for handling stream data collection and truncation.
 */

import type { Readable } from 'stream';

/**
 * Collect data from a readable stream with size limit
 *
 * Automatically truncates output if it exceeds maxSize.
 * This prevents memory issues with unbounded output.
 *
 * @param stream - Readable stream to collect from
 * @param maxSize - Maximum size in bytes (default: 30KB)
 * @param onTruncate - Optional callback when truncation occurs
 * @returns Promise resolving to collected string (trimmed)
 */
export async function collectStreamWithLimit(
  stream: Readable,
  maxSize: number = 30 * 1024,
  onTruncate?: () => void
): Promise<string> {
  let output = '';
  let truncated = false;

  return new Promise((resolve, reject) => {
    stream.on('data', (data: Buffer) => {
      if (truncated) {
        return; // Already truncated, ignore further data
      }

      output += data.toString();

      // Truncate if too large
      if (output.length > maxSize) {
        output = output.substring(0, maxSize);
        truncated = true;
        if (onTruncate) {
          onTruncate();
        }
      }
    });

    stream.on('end', () => {
      resolve(output.trim());
    });

    stream.on('error', (error) => {
      reject(error);
    });
  });
}

/**
 * Truncate a string to a maximum length with indicator
 *
 * @param text - Text to truncate
 * @param maxLength - Maximum length
 * @param indicator - Truncation indicator (default: "... [truncated]")
 * @returns Truncated text
 */
export function truncateWithIndicator(
  text: string,
  maxLength: number,
  indicator: string = '... [truncated]'
): string {
  if (text.length <= maxLength) {
    return text;
  }

  const truncateAt = maxLength - indicator.length;
  return text.substring(0, truncateAt) + indicator;
}
