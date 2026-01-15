/**
 * Unified types for LLM providers
 * Abstracts differences between OpenAI, Anthropic, and Gemini APIs
 */

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
}

// ============================================================================
// Streaming Types
// ============================================================================

export interface StreamChunkText {
  type: 'text';
  text: string;
}

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

export type ProviderConfig = OpenAIConfig | AnthropicConfig | GeminiConfig;
