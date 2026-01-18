/**
 * Subagent System Types
 *
 * Types for the subagent system that enables isolated, specialized task execution.
 * Based on Claude Code's Task tool architecture.
 */

import type { Message } from '../providers/types.js';
import type { DiscoverableResource, ResourceSource } from '../discovery/types.js';

/**
 * Subagent Types
 * - Explore: Fast read-only codebase exploration (haiku)
 * - Plan: Architecture design and planning (sonnet)
 * - Bash: Command execution specialist (haiku)
 * - general-purpose: Full capabilities (sonnet)
 */
export type SubagentType = 'Explore' | 'Plan' | 'Bash' | 'general-purpose';

/**
 * Execution modes for subagents
 */
export type SubagentExecutionMode = 'foreground' | 'background';

/**
 * Single task definition for parallel execution (Phase 4)
 */
export interface ParallelTaskDefinition {
  /** Short description (3-5 words) */
  description: string;

  /** Task prompt */
  prompt: string;

  /** Subagent type */
  subagent_type: SubagentType;

  /** Optional model override */
  model?: string;

  /** Optional max turns */
  max_turns?: number;
}

/**
 * Task tool input schema
 * Defines parameters for launching a subagent
 */
export interface TaskInput {
  /** Short description (3-5 words) for UI display */
  description: string;

  /** Detailed task instructions/prompt for the subagent */
  prompt: string;

  /** Type of subagent to spawn */
  subagent_type: SubagentType;

  /** Optional: specific model to use (overrides default for type) */
  model?: string;

  /** Optional: run in background (returns immediately with agent ID) */
  run_in_background?: boolean;

  /** Optional: resume a previous subagent by ID */
  resume?: string;

  /** Optional: max conversation turns (default: varies by type) */
  max_turns?: number;

  /** Optional: parallel tasks array (Phase 4) - mutually exclusive with single task */
  tasks?: ParallelTaskDefinition[];
}

/**
 * Task tool output
 * Result from subagent execution
 */
export interface TaskOutput {
  /** Whether task completed successfully */
  success: boolean;

  /** Summary result from subagent (for foreground execution) */
  result?: string;

  /** Unique agent ID (for tracking/resume) */
  agentId: string;

  /** Output file path (for background tasks) */
  outputFile?: string;

  /** Error message if failed */
  error?: string;

  /** Execution metadata */
  metadata?: {
    subagentType: SubagentType;
    model: string;
    turns: number;
    durationMs: number;
    tokenUsage?: { input: number; output: number };
  };
}

/**
 * Summary generation configuration
 */
export interface SummaryConfig {
  /** Maximum length for summary (in characters) */
  maxLength: number;

  /** Truncation strategy: simple (hard cut) or smart (try to break at sentence) */
  truncationStrategy?: 'simple' | 'smart';
}

/**
 * Subagent configuration per type
 * Defines capabilities and behavior for each subagent type
 */
export interface SubagentConfig {
  /** Type identifier */
  type: SubagentType;

  /** Tools allowed for this agent type */
  allowedTools: string[];

  /** Default model to use */
  defaultModel: string;

  /** System prompt for this agent type */
  systemPrompt: string;

  /** Maximum conversation turns */
  maxTurns: number;

  /** Summary generation configuration (optional, uses defaults if not specified) */
  summaryConfig?: SummaryConfig;
}

/**
 * Subagent session state (for resume capability)
 */
export interface SubagentSession {
  /** Unique session ID */
  id: string;

  /** Agent type */
  type: SubagentType;

  /** Original task description */
  description: string;

  /** Current execution status */
  status: 'running' | 'completed' | 'error' | 'cancelled';

  /** Messages (conversation history) */
  messages: Message[];

  /** Result summary (if completed) */
  result?: string;

  /** Error (if failed) */
  error?: string;

  /** Creation timestamp */
  createdAt: Date;

  /** Last update timestamp */
  updatedAt: Date;

  /** Token usage */
  tokenUsage?: { input: number; output: number };
}

/**
 * Background task metadata (for Phase 2)
 */
export interface BackgroundTask {
  /** Task ID */
  id: string;

  /** Subagent session reference */
  subagentId: string;

  /** Task description */
  description: string;

  /** Current status */
  status: 'running' | 'completed' | 'error';

  /** Output file path */
  outputFile: string;

  /** Started at */
  startedAt: Date;

  /** Completed at (if finished) */
  completedAt?: Date;
}

/**
 * Custom Agent Definition - for discovery system
 *
 * Represents a user-defined custom agent loaded from JSON or Markdown files.
 * This is the discoverable resource type used by the unified loading system.
 */
export interface CustomAgentDefinition extends DiscoverableResource {
  /** Agent name (identifier) */
  name: string;

  /** Agent description */
  description: string;

  /** Tools this agent can use */
  allowedTools: string[];

  /** Default model to use */
  defaultModel: string;

  /** Maximum conversation turns */
  maxTurns: number;

  /** System prompt for the agent */
  systemPrompt: string;

  /** Source metadata (path, level, namespace) */
  source: ResourceSource;
}

/**
 * Convert CustomAgentDefinition to SubagentConfig
 */
export function customAgentToConfig(agent: CustomAgentDefinition): SubagentConfig {
  return {
    type: 'general-purpose', // Custom agents are general-purpose type
    allowedTools: agent.allowedTools,
    defaultModel: agent.defaultModel,
    maxTurns: agent.maxTurns,
    systemPrompt: agent.systemPrompt,
  };
}
