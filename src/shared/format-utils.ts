/**
 * Shared Formatting Utilities
 *
 * Common utilities for formatting output, messages, and status indicators.
 */

/**
 * Format a boxed message with title and metadata fields
 *
 * Creates a visually distinct box with header and key-value pairs:
 * ```
 * ┌─ Title ─────────────────────────────────────────
 * │ Key1: Value1
 * │ Key2: Value2
 * └─────────────────────────────────────────────────
 * ```
 *
 * @param title - Box title
 * @param fields - Key-value pairs to display
 * @param width - Total box width (default: 52)
 * @returns Formatted boxed message
 */
export function formatBoxedMessage(
  title: string,
  fields: Record<string, string>,
  width: number = 52
): string {
  const lines: string[] = [];

  // Calculate padding for title
  const titleText = ` ${title} `;
  const dashCount = Math.max(0, width - titleText.length - 2);
  const titleLine = `┌─${titleText}${'─'.repeat(dashCount)}`;

  lines.push(titleLine);

  // Add fields
  for (const [key, value] of Object.entries(fields)) {
    lines.push(`│ ${key}: ${value}`);
  }

  // Bottom border
  lines.push(`└${'─'.repeat(width - 1)}`);

  return lines.join('\n');
}

/**
 * Status indicator symbols (text-based, no emojis)
 */
export const STATUS_SYMBOLS = {
  BLOCKED: '[BLOCKED]',
  SUCCESS: '[SUCCESS]',
  FAILED: '[FAILED]',
  INFO: '[INFO]',
  WARNING: '[WARNING]',
} as const;

/**
 * Format a status message with symbol prefix
 *
 * @param status - Status type
 * @param message - Status message
 * @returns Formatted status string
 */
export function formatStatus(
  status: keyof typeof STATUS_SYMBOLS,
  message: string
): string {
  return `${STATUS_SYMBOLS[status]} ${message}`;
}
