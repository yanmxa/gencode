/**
 * Session Compression Module
 *
 * Implements OpenCode-inspired three-layer compression strategy:
 * - Layer 1: Tool output pruning (fast, no cost)
 * - Layer 2: Compaction summarization (LLM-based, medium cost)
 * - Layer 3: Message filtering (recovery optimization)
 */

export { CompressionEngine } from './engine.js';
export type {
  CompressionConfig,
  ConversationSummary,
  ToolUsageSummary,
  TokenUsage,
  ModelInfo,
} from './types.js';
export { DEFAULT_COMPRESSION_CONFIG } from './types.js';
