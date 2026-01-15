/**
 * Agent Types
 */

import type { PermissionConfig } from '../permissions/types.js';

export interface AgentConfig {
  provider: 'openai' | 'anthropic' | 'gemini' | 'vertex-ai';
  model: string;
  systemPrompt?: string;
  tools?: string[];
  cwd?: string;
  maxTurns?: number;
  permissions?: Partial<PermissionConfig>;
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
}

export type AgentEvent =
  | AgentEventText
  | AgentEventToolStart
  | AgentEventToolResult
  | AgentEventThinking
  | AgentEventError
  | AgentEventDone;
