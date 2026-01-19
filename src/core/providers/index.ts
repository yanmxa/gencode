/**
 * LLM Providers - Unified interface for OpenAI, Anthropic, Gemini, and Vertex AI
 */

export * from './types.js';
export { OpenAIProvider } from './openai.js';
export { AnthropicProvider } from './anthropic.js';
export { GoogleProvider } from './google.js';
export { AnthropicVertexProvider } from './vertex-ai.js';

import type {
  LLMProvider,
  Provider,
  AuthMethod,
  OpenAIConfig,
  AnthropicConfig,
  GoogleConfig,
  VertexAIConfig,
} from './types.js';
import { OpenAIProvider } from './openai.js';
import { AnthropicProvider } from './anthropic.js';
import { GoogleProvider } from './google.js';
import { AnthropicVertexProvider } from './vertex-ai.js';

// Legacy type alias for backward compatibility
/** @deprecated Use Provider instead */
export type ProviderName = Provider;

export type ProviderConfigMap = {
  openai: OpenAIConfig;
  anthropic: AnthropicConfig;
  google: GoogleConfig;
};

export interface CreateProviderOptions {
  provider: Provider;
  authMethod?: AuthMethod;
  config?: OpenAIConfig | AnthropicConfig | GoogleConfig | VertexAIConfig;
}

/**
 * Create a provider instance by provider and auth method
 * If authMethod is not provided, defaults to 'api_key'
 */
export function createProvider(options: CreateProviderOptions): LLMProvider {
  const { provider, authMethod = 'api_key', config } = options;

  // Map provider + authMethod to the correct implementation
  if (provider === 'anthropic') {
    if (authMethod === 'vertex') {
      return new AnthropicVertexProvider(config as VertexAIConfig);
    } else if (authMethod === 'api_key') {
      return new AnthropicProvider(config as AnthropicConfig);
    }
    throw new Error(`Unsupported auth method for anthropic: ${authMethod}`);
  }

  if (provider === 'openai') {
    if (authMethod === 'api_key') {
      return new OpenAIProvider(config as OpenAIConfig);
    }
    throw new Error(`Unsupported auth method for openai: ${authMethod}`);
  }

  if (provider === 'google') {
    if (authMethod === 'api_key') {
      return new GoogleProvider(config as GoogleConfig);
    }
    throw new Error(`Unsupported auth method for google: ${authMethod}`);
  }

  throw new Error(`Unknown provider: ${provider}`);
}

/**
 * Infer provider from model name
 * Note: This only returns the provider, not the auth method
 * For Vertex AI models (claude-*@version), this returns 'anthropic'
 */
export function inferProvider(model: string): Provider {
  const modelLower = model.toLowerCase();

  // OpenAI models
  if (
    modelLower.includes('gpt') ||
    modelLower.includes('o1') ||
    modelLower.includes('o3') ||
    modelLower.startsWith('text-') ||
    modelLower.startsWith('davinci') ||
    modelLower.startsWith('curie')
  ) {
    return 'openai';
  }

  // Anthropic models (including Vertex AI format with @)
  if (modelLower.includes('claude')) {
    return 'anthropic';
  }

  // Google models (Gemini)
  if (modelLower.includes('gemini') || modelLower.includes('palm')) {
    return 'google';
  }

  // Default to OpenAI (most common)
  return 'openai';
}

/**
 * Infer auth method from model name
 * Returns undefined if auth method cannot be inferred
 */
export function inferAuthMethod(model: string): AuthMethod | undefined {
  const modelLower = model.toLowerCase();

  // Vertex AI models (Claude models with @ version suffix like claude-sonnet-4-5@20250929)
  if (modelLower.includes('claude') && modelLower.includes('@')) {
    return 'vertex';
  }

  // For other models, we can't reliably infer auth method
  return undefined;
}

/**
 * Common model aliases
 */
export const ModelAliases: Record<
  string,
  { provider: Provider; authMethod?: AuthMethod; model: string }
> = {
  // OpenAI
  'gpt-4o': { provider: 'openai', model: 'gpt-4o' },
  'gpt-4o-mini': { provider: 'openai', model: 'gpt-4o-mini' },
  'gpt-4-turbo': { provider: 'openai', model: 'gpt-4-turbo' },
  'o1': { provider: 'openai', model: 'o1' },
  'o1-mini': { provider: 'openai', model: 'o1-mini' },
  'o3-mini': { provider: 'openai', model: 'o3-mini' },

  // Anthropic (Direct API)
  'claude-opus': { provider: 'anthropic', model: 'claude-opus-4-5-20251101' },
  'claude-sonnet': { provider: 'anthropic', model: 'claude-sonnet-4-20250514' },
  'claude-haiku': { provider: 'anthropic', model: 'claude-haiku-4-20250514' },
  'claude-3.5-sonnet': { provider: 'anthropic', model: 'claude-3-5-sonnet-20241022' },

  // Google (Gemini models)
  'gemini-2.0-flash': { provider: 'google', model: 'gemini-2.0-flash' },
  'gemini-1.5-pro': { provider: 'google', model: 'gemini-1.5-pro' },
  'gemini-1.5-flash': { provider: 'google', model: 'gemini-1.5-flash' },

  // Anthropic via Vertex AI
  'vertex-sonnet': { provider: 'anthropic', authMethod: 'vertex', model: 'claude-sonnet-4-5@20250929' },
  'vertex-haiku': { provider: 'anthropic', authMethod: 'vertex', model: 'claude-haiku-4-5@20251001' },
  'vertex-opus': { provider: 'anthropic', authMethod: 'vertex', model: 'claude-opus-4-1@20250805' },
};
