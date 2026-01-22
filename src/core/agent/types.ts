/**
 * Agent Types
 */

import type { PermissionConfig } from '../permissions/types.js';
import type { CostEstimate } from '../pricing/types.js';
import type { Provider, AuthMethod } from '../providers/types.js';
import type { CompressionConfig } from '../session/compression/types.js';

export interface AgentConfig {
  provider: Provider;
  authMethod?: AuthMethod;
  model: string;
  systemPrompt?: string;
  tools?: string[];
  cwd?: string;
  maxTurns?: number;
  permissions?: Partial<PermissionConfig>;
  memoryMergeStrategy?: 'fallback' | 'both' | 'gen-only' | 'claude-only';
  verbose?: boolean;
  /** Compression configuration */
  compression?: Partial<CompressionConfig>;
  /** Enable LLM token streaming for real-time output */
  streaming?: boolean;
}

// Agent Events
export interface AgentEventText {
  type: 'text';
  text: string;
}

export interface AgentEventToolStart {
  type: 'tool_start';
  id: string;
  name: string;
  input: unknown;
}

export interface AgentEventToolResult {
  type: 'tool_result';
  id: string;
  name: string;
  result: {
    success: boolean;
    output: string;
    error?: string;
    metadata?: {
      title?: string;
      subtitle?: string;
      size?: number;
      statusCode?: number;
      contentType?: string;
      duration?: number;
    };
  };
}

export interface AgentEventThinking {
  type: 'thinking';
  text: string;
}

export interface AgentEventError {
  type: 'error';
  error: Error;
}

export interface AgentEventDone {
  type: 'done';
  text: string;
  usage?: {
    inputTokens: number;
    outputTokens: number;
  };
  cost?: CostEstimate;
}

export interface AgentEventAskUser {
  type: 'ask_user';
  id: string;
  questions: Array<{
    question: string;
    header: string;
    options: Array<{ label: string; description: string }>;
    multiSelect: boolean;
  }>;
}

export interface AgentEventReasoningDelta {
  type: 'reasoning_delta';
  text: string;  // Reasoning content from o1/o3/Gemini 3+ models
}

export interface AgentEventToolInputDelta {
  type: 'tool_input_delta';
  id: string;
  delta: string;  // Incremental JSON string fragment
}

/**
 * Permission request event - yielded when a tool needs user permission
 * This allows the UI to render the permission prompt without blocking the generator
 */
export interface AgentEventPermissionRequest {
  type: 'permission_request';
  id: string;           // Unique request ID for response correlation
  toolCallId: string;   // References the tool_start.id
  tool: string;
  input: unknown;
  suggestions: Array<{
    action: string;
    label: string;
    shortcut?: string;
  }>;
  metadata?: Record<string, unknown>;  // For Edit tool diff preview, etc.
}

/**
 * Waiting for permission event - yielded repeatedly while waiting for user response
 * This keeps the generator alive so Ink can render and process user input
 */
export interface AgentEventWaitingForPermission {
  type: 'waiting_for_permission';
  requestId: string;
}

export type AgentEvent =
  | AgentEventText
  | AgentEventToolStart
  | AgentEventToolResult
  | AgentEventThinking
  | AgentEventError
  | AgentEventDone
  | AgentEventAskUser
  | AgentEventReasoningDelta
  | AgentEventToolInputDelta
  | AgentEventPermissionRequest
  | AgentEventWaitingForPermission;
