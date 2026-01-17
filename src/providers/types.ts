/**
 * Unified types for LLM providers
 * Abstracts differences between OpenAI, Anthropic, and Gemini APIs
 */

import type { CostEstimate } from '../pricing/types.js';

// ============================================================================
// Provider Types
// ============================================================================

/**
 * Provider - Semantic layer (only 3 providers)
 */
export type Provider = 'anthropic' | 'openai' | 'gemini';

/**
 * Authentication method for providers
 */
export type AuthMethod = 'api_key' | 'vertex' | 'bedrock' | 'azure' | 'oauth';

/**
 * Provider class metadata (static property on each provider implementation)
 */
export interface ProviderClassMeta {
  /** Which provider this class implements */
  provider: Provider;
  /** Authentication method */
  authMethod: AuthMethod;
  /** Environment variables required for this auth method */
  envVars: string[];
  /** Display name for UI */
  displayName: string;
  /** Optional description */
  description?: string;
}

// ============================================================================
// Message Types
// ============================================================================

export type MessageRole = 'system' | 'user' | 'assistant';

export interface TextContent {
  type: 'text';
  text: string;
}

export interface ToolUseContent {
  type: 'tool_use';
  id: string;
  name: string;
  input: Record<string, unknown>;
  // Gemini 3+ thought signature for maintaining reasoning chain
  thoughtSignature?: string;
}

export interface ToolResultContent {
  type: 'tool_result';
  toolUseId: string;
  content: string;
  isError?: boolean;
}

export type MessageContent = TextContent | ToolUseContent | ToolResultContent;

export interface Message {
  role: MessageRole;
  content: string | MessageContent[];
}

// ============================================================================
// Tool Types
// ============================================================================

export interface JSONSchema {
  type: string;
  properties?: Record<string, JSONSchema>;
  required?: string[];
  description?: string;
  items?: JSONSchema;
  enum?: unknown[];
  [key: string]: unknown;
}

export interface ToolDefinition {
  name: string;
  description: string;
  parameters: JSONSchema;
}

export interface ToolCall {
  id: string;
  name: string;
  input: Record<string, unknown>;
}

export interface ToolResult {
  toolUseId: string;
  content: string;
  isError?: boolean;
}

// ============================================================================
// Completion Types
// ============================================================================

export interface CompletionOptions {
  model: string;
  messages: Message[];
  tools?: ToolDefinition[];
  systemPrompt?: string;
  maxTokens?: number;
  temperature?: number;
  stream?: boolean;
}

export type StopReason = 'end_turn' | 'tool_use' | 'max_tokens' | 'stop_sequence';

export interface CompletionResponse {
  content: MessageContent[];
  stopReason: StopReason;
  usage?: {
    inputTokens: number;
    outputTokens: number;
  };
  cost?: CostEstimate;
}

// ============================================================================
// Streaming Types
// ============================================================================

export interface StreamChunkText {
  type: 'text';
  text: string;
}/* The above code appears to be a comment block in TypeScript. It includes a placeholder
text "ANTHROPIC_VERTEX_PROJECT_ID" and " */


export interface StreamChunkToolStart {
  type: 'tool_start';
  id: string;
  name: string;
}

export interface StreamChunkToolInput {
  type: 'tool_input';
  id: string;
  input: string; // Partial JSON string
}

export interface StreamChunkDone {
  type: 'done';
  response: CompletionResponse;
}

export interface StreamChunkError {
  type: 'error';
  error: Error;
}

export type StreamChunk =
  | StreamChunkText
  | StreamChunkToolStart
  | StreamChunkToolInput
  | StreamChunkDone
  | StreamChunkError;

// ============================================================================
// Provider Interface
// ============================================================================

export interface ModelInfo {
  id: string;
  name: string;
  description?: string;
}

export interface LLMProvider {
  readonly name: string;

  /**
   * Generate a completion (non-streaming)
   */
  complete(options: CompletionOptions): Promise<CompletionResponse>;

  /**
   * Generate a streaming completion
   */
  stream(options: CompletionOptions): AsyncGenerator<StreamChunk, void, unknown>;

  /**
   * List available models from the provider
   */
  listModels(): Promise<ModelInfo[]>;
}

// ============================================================================
// Provider Configuration
// ============================================================================

export interface OpenAIConfig {
  apiKey?: string;
  baseURL?: string;
  organization?: string;
}

export interface AnthropicConfig {
  apiKey?: string;
  baseURL?: string;
}

export interface GeminiConfig {
  apiKey?: string;
}

export interface VertexAIConfig {
  projectId?: string;
  region?: string;
  accessToken?: string;
}

export type ProviderConfig = OpenAIConfig | AnthropicConfig | GeminiConfig | VertexAIConfig;
