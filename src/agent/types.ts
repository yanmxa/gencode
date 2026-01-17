/**
 * Agent Types
 */

import type { PermissionConfig } from '../permissions/types.js';
import type { CostEstimate } from '../pricing/types.js';
import type { Provider, AuthMethod } from '../providers/types.js';

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

export type AgentEvent =
  | AgentEventText
  | AgentEventToolStart
  | AgentEventToolResult
  | AgentEventThinking
  | AgentEventError
  | AgentEventDone
  | AgentEventAskUser;
