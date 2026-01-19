/**
 * Session Management Types
 */

import type { Message } from '../providers/types.js';
import type { CostEstimate } from '../pricing/types.js';
import type { ConversationSummary, CompressionConfig } from './compression/types.js';
import type { FileCheckpoint } from './checkpointing/types.js';
import type { SubagentType } from '../../ext/subagents/types.js';

export interface SessionMetadata {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  provider: string;
  model: string;
  cwd: string;
  messageCount: number;
  tokenUsage?: {
    input: number;
    output: number;
  };
  parentId?: string; // For forked sessions

  // Track completions for UI restoration
  completions?: Array<{
    afterMessageIndex: number; // Which message this completion follows
    durationMs: number;
    usage?: { inputTokens: number; outputTokens: number };
    cost?: CostEstimate;
  }>;

  // Subagent session fields (Phase 3)
  isSubagentSession?: boolean; // True if this is a subagent session
  subagentType?: SubagentType; // Type of subagent (Explore, Plan, Bash, general-purpose)
  parentSessionId?: string; // Link to parent agent session (for subagents)
  resumeCount?: number; // Number of times this session has been resumed
  expiresAt?: string; // ISO 8601 timestamp when session expires
  originalDescription?: string; // Original task description
  lastResumedAt?: string; // ISO 8601 timestamp of last resume
}

export interface Session {
  metadata: SessionMetadata;
  messages: Message[];
  systemPrompt?: string;

  // Compression support
  summaries?: ConversationSummary[];
  fullMessageCount?: number; // Total messages before compression

  // Checkpointing support
  checkpoints?: FileCheckpoint[];
}

export interface SessionListItem {
  id: string;
  title: string;
  updatedAt: string;
  messageCount: number;
  preview: string; // First user message preview
}

export interface SessionConfig {
  storageDir: string;
  maxSessions: number;
  maxAge: number; // Days
  autoSave: boolean;

  // Compression configuration
  compression?: Partial<CompressionConfig>;
}

export const DEFAULT_SESSION_CONFIG: SessionConfig = {
  storageDir: '~/.gen/sessions',
  maxSessions: 50,
  maxAge: 30,
  autoSave: true,
};
