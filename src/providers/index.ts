/**
 * LLM Providers - Unified interface for OpenAI, Anthropic, Gemini, and Vertex AI
 */

export * from './types.js';
export { OpenAIProvider } from './openai.js';
export { AnthropicProvider } from './anthropic.js';
export { GeminiProvider } from './gemini.js';
export { VertexAIProvider } from './vertex-ai.js';

import type { LLMProvider, OpenAIConfig, AnthropicConfig, GeminiConfig, VertexAIConfig } from './types.js';
import { OpenAIProvider } from './openai.js';
import { AnthropicProvider } from './anthropic.js';
import { GeminiProvider } from './gemini.js';
import { VertexAIProvider } from './vertex-ai.js';

export type ProviderName = 'openai' | 'anthropic' | 'gemini' | 'vertex-ai';
export type ProviderConfigMap = {
  openai: OpenAIConfig;
  anthropic: AnthropicConfig;
  gemini: GeminiConfig;
  'vertex-ai': VertexAIConfig;
};

export interface CreateProviderOptions<T extends ProviderName = ProviderName> {
  provider: T;
  config?: ProviderConfigMap[T];
}

/**
 * Create a provider instance by name
 */
export function createProvider(options: CreateProviderOptions): LLMProvider {
  switch (options.provider) {
    case 'openai':
      return new OpenAIProvider(options.config as OpenAIConfig);
    case 'anthropic':
      return new AnthropicProvider(options.config as AnthropicConfig);
    case 'gemini':
      return new GeminiProvider(options.config as GeminiConfig);
    case 'vertex-ai':
      return new VertexAIProvider(options.config as VertexAIConfig);
    default:
      throw new Error(`Unknown provider: ${options.provider}`);
  }
}

/**
 * Infer provider from model name
 */
export function inferProvider(model: string): ProviderName {
  const modelLower = model.toLowerCase();

  // Vertex AI models (Claude models with @ version suffix like claude-sonnet-4-5@20250929)
  if (modelLower.includes('claude') && modelLower.includes('@')) {
    return 'vertex-ai';
  }

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

  // Anthropic models
  if (modelLower.includes('claude')) {
    return 'anthropic';
  }

  // Gemini models
  if (modelLower.includes('gemini') || modelLower.includes('palm')) {
    return 'gemini';
  }

  // Default to OpenAI (most common)
  return 'openai';
}

/**
 * Common model aliases
 */
export const ModelAliases: Record<string, { provider: ProviderName; model: string }> = {
  // OpenAI
  'gpt-4o': { provider: 'openai', model: 'gpt-4o' },
  'gpt-4o-mini': { provider: 'openai', model: 'gpt-4o-mini' },
  'gpt-4-turbo': { provider: 'openai', model: 'gpt-4-turbo' },
  'o1': { provider: 'openai', model: 'o1' },
  'o1-mini': { provider: 'openai', model: 'o1-mini' },
  'o3-mini': { provider: 'openai', model: 'o3-mini' },

  // Anthropic
  'claude-opus': { provider: 'anthropic', model: 'claude-opus-4-5-20251101' },
  'claude-sonnet': { provider: 'anthropic', model: 'claude-sonnet-4-20250514' },
  'claude-haiku': { provider: 'anthropic', model: 'claude-haiku-4-20250514' },
  'claude-3.5-sonnet': { provider: 'anthropic', model: 'claude-3-5-sonnet-20241022' },

  // Gemini
  'gemini-2.0-flash': { provider: 'gemini', model: 'gemini-2.0-flash' },
  'gemini-1.5-pro': { provider: 'gemini', model: 'gemini-1.5-pro' },
  'gemini-1.5-flash': { provider: 'gemini', model: 'gemini-1.5-flash' },

  // Vertex AI (Claude on GCP)
  'vertex-sonnet': { provider: 'vertex-ai', model: 'claude-sonnet-4-5@20250929' },
  'vertex-haiku': { provider: 'vertex-ai', model: 'claude-haiku-4-5@20251001' },
  'vertex-opus': { provider: 'vertex-ai', model: 'claude-opus-4-1@20250805' },
};
