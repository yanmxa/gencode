/**
 * Recode - Multi-LLM Agent SDK
 *
 * A unified SDK for building AI agents with support for
 * OpenAI, Anthropic, and Google Gemini models.
 */

// Providers
export {
  // Types
  type LLMProvider,
  type Message,
  type MessageRole,
  type MessageContent,
  type TextContent,
  type ToolUseContent,
  type ToolResultContent,
  type ToolDefinition,
  type ToolCall,
  type ToolResult,
  type CompletionOptions,
  type CompletionResponse,
  type StreamChunk,
  type StopReason,
  type OpenAIConfig,
  type AnthropicConfig,
  type GeminiConfig,
  type ProviderConfig,
  type ProviderName,
  // Providers
  OpenAIProvider,
  AnthropicProvider,
  GeminiProvider,
  // Factory
  createProvider,
  inferProvider,
  ModelAliases,
} from './providers/index.js';

// Tools
export {
  type Tool,
  type ToolContext,
  type ToolResult as ToolExecutionResult,
  ToolRegistry,
  createDefaultRegistry,
  builtinTools,
  readTool,
  writeTool,
  editTool,
  bashTool,
  globTool,
  grepTool,
} from './tools/index.js';

// Permissions
export {
  type PermissionMode,
  type PermissionRule,
  type PermissionConfig,
  type ConfirmCallback,
  PermissionManager,
  DEFAULT_PERMISSION_CONFIG,
} from './permissions/index.js';

// Agent
export {
  type AgentConfig,
  type AgentEvent,
  type AgentEventText,
  type AgentEventToolStart,
  type AgentEventToolResult,
  type AgentEventError,
  type AgentEventDone,
  Agent,
} from './agent/index.js';

// Session
export {
  type Session,
  type SessionMetadata,
  type SessionListItem,
  type SessionConfig,
  SessionManager,
  DEFAULT_SESSION_CONFIG,
} from './session/index.js';

// Checkpointing
export {
  type ChangeType,
  type FileCheckpoint,
  type CheckpointSession,
  type RewindOptions,
  type RewindResult,
  type CheckpointSummary,
  type RecordChangeInput,
  CheckpointManager,
  getCheckpointManager,
  initCheckpointManager,
  resetCheckpointManager,
} from './checkpointing/index.js';
