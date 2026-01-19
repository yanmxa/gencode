/**
 * Compression and summarization types for session management
 * Inspired by OpenCode's multi-layer compression strategy
 */

/**
 * Token usage tracking with fine-grained breakdown
 */
export interface TokenUsage {
  /** Regular input tokens */
  input: number;
  /** Model output tokens */
  output: number;
  /** Reasoning tokens (for thinking models like o1/o3) */
  reasoning?: number;
  /** Cache statistics */
  cache?: {
    /** Cache hit tokens (~10% cost) */
    read: number;
    /** Cache creation tokens */
    write: number;
  };
}

/**
 * Compression configuration
 */
export interface CompressionConfig {
  /** Enable/disable compression */
  enabled: boolean;
  /** Enable Layer 1: Tool output pruning */
  enablePruning: boolean;
  /** Enable Layer 2: Compaction summarization */
  enableCompaction: boolean;
  /** Minimum tokens to trigger pruning (default: 20k) */
  pruneMinimum: number;
  /** Protect recent tokens from pruning (default: 40k) */
  pruneProtect: number;
  /** Reserved output tokens (default: 32k) */
  reservedOutputTokens: number;
  /** Model to use for summarization (optional, defaults to current model) */
  model?: string;
}

/**
 * Tool usage summary
 */
export interface ToolUsageSummary {
  /** Tool name */
  tool: string;
  /** Number of times used */
  count: number;
  /** Notable uses (up to 3) */
  notableUses: string[];
}

/**
 * Conversation summary metadata
 */
export interface ConversationSummary {
  /** Unique summary ID */
  id: string;
  /** Summary type (compaction) */
  type: 'compaction';
  /** Range of messages covered [start, end] */
  coveringMessages: [number, number];
  /** Narrative summary content (continuation prompt) */
  content: string;
  /** Key decisions made */
  keyDecisions: string[];
  /** Files modified */
  filesModified: string[];
  /** Tools used with statistics */
  toolsUsed: ToolUsageSummary[];
  /** Generation timestamp */
  generatedAt: string;
  /** Estimated tokens in summary */
  estimatedTokens: number;
}

/**
 * Model information for compression decisions
 */
export interface ModelInfo {
  /** Model context window size */
  contextWindow: number;
  /** Model output limit (optional) */
  outputLimit?: number;
}

/**
 * Default compression configuration
 * Based on OpenCode constants
 */
export const DEFAULT_COMPRESSION_CONFIG: CompressionConfig = {
  enabled: true,
  enablePruning: true,
  enableCompaction: true,
  pruneMinimum: 20_000,      // Minimum tokens to trigger pruning
  pruneProtect: 40_000,      // Protect recent 40k tokens
  reservedOutputTokens: 32_000, // Reserve 32k for output
};
