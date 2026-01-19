/**
 * Structured logging module for GenCode
 * Provides consistent logging across all components
 */

import { getDebugConfig } from './debug.js';

export enum LogLevel {
  ERROR = 'error',
  WARN = 'warn',
  INFO = 'info',
  DEBUG = 'debug',
}

export interface LogContext {
  [key: string]: unknown;
}

/**
 * Format context object for readable output
 */
function formatContext(context: LogContext): string {
  const entries = Object.entries(context);
  if (entries.length === 0) {
    return '';
  }

  const formatted = entries
    .map(([key, value]) => {
      if (typeof value === 'string') {
        return `${key}="${value}"`;
      }
      if (value === undefined || value === null) {
        return `${key}=${value}`;
      }
      if (typeof value === 'object') {
        return `${key}=${JSON.stringify(value)}`;
      }
      return `${key}=${value}`;
    })
    .join(' ');

  return ` [${formatted}]`;
}

/**
 * Log a message with structured context
 *
 * @param level - Log level (error, warn, info, debug)
 * @param component - Component name (e.g., 'Skills', 'MCP', 'Discovery')
 * @param message - Log message
 * @param context - Optional structured context data
 */
export function log(
  level: LogLevel,
  component: string,
  message: string,
  context?: LogContext
): void {
  const timestamp = new Date().toISOString();
  const contextStr = context ? formatContext(context) : '';
  const formatted = `[${timestamp}] ${component}:${level} - ${message}${contextStr}`;

  switch (level) {
    case LogLevel.ERROR:
      console.error(formatted);
      break;
    case LogLevel.WARN:
      console.warn(formatted);
      break;
    case LogLevel.DEBUG:
      // Only output debug logs if debug mode is enabled
      if (getDebugConfig().enabled) {
        console.log(formatted);
      }
      break;
    case LogLevel.INFO:
    default:
      console.log(formatted);
      break;
  }
}

/**
 * Convenience functions for each log level
 */
export const logger = {
  error: (component: string, message: string, context?: LogContext) =>
    log(LogLevel.ERROR, component, message, context),

  warn: (component: string, message: string, context?: LogContext) =>
    log(LogLevel.WARN, component, message, context),

  info: (component: string, message: string, context?: LogContext) =>
    log(LogLevel.INFO, component, message, context),

  debug: (component: string, message: string, context?: LogContext) =>
    log(LogLevel.DEBUG, component, message, context),
};
