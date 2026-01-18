/**
 * Prompt Loader - Load prompts from .txt files
 *
 * This module provides utilities for loading system prompts and tool descriptions
 * from separate .txt files, enabling easier maintenance and prompt engineering.
 */

import { readFileSync, existsSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import { homedir } from 'os';
import * as os from 'os';

const __dirname = dirname(fileURLToPath(import.meta.url));

// Path to providers.json config
const PROVIDERS_CONFIG_PATH = join(homedir(), '.gen', 'providers.json');

// Resolve prompts directory - check both src and dist locations
function getPromptsDir(): string {
  // If running from dist, look for prompts in src
  if (__dirname.includes('/dist/')) {
    const srcPath = __dirname.replace('/dist/', '/src/');
    if (existsSync(srcPath)) {
      return srcPath;
    }
  }
  return __dirname;
}

const promptsDir = getPromptsDir();

export type ProviderType = 'anthropic' | 'openai' | 'gemini' | 'generic';

/**
 * Providers config structure from ~/.gen/providers.json
 */
interface ProvidersConfig {
  connections: Record<string, unknown>;
  models: Record<string, { list: Array<{ id: string }> }>;
}

/**
 * Load providers config from ~/.gen/providers.json
 */
function loadProvidersConfig(): ProvidersConfig | null {
  try {
    if (existsSync(PROVIDERS_CONFIG_PATH)) {
      const data = readFileSync(PROVIDERS_CONFIG_PATH, 'utf-8');
      return JSON.parse(data);
    }
  } catch {
    // Ignore parse errors
  }
  return null;
}

/**
 * Look up which provider owns a given model ID
 * Searches through ~/.gen/providers.json to find the provider
 *
 * @param model - The model ID (e.g., "claude-sonnet-4-5@20250929")
 * @returns The provider name (e.g., "anthropic") or null if not found
 */
export function getProviderForModel(model: string): string | null {
  const config = loadProvidersConfig();
  if (!config?.models) {
    return null;
  }

  for (const [provider, cache] of Object.entries(config.models)) {
    if (cache.list?.some((m) => m.id === model)) {
      return provider;
    }
  }

  return null;
}

/**
 * Map provider names to prompt types
 * Falls back to 'generic' for unknown providers
 * Handles both "provider" and "provider:authMethod" formats
 */
export function mapProviderToPromptType(provider: string): ProviderType {
  // Extract provider prefix (e.g., "google:api_key" → "google")
  const providerPrefix = provider.split(':')[0];

  switch (providerPrefix) {
    case 'anthropic':
      return 'anthropic';
    case 'openai':
      return 'openai';
    case 'google':
      return 'gemini'; // Google provider uses Gemini models, so use gemini prompt
    default:
      return 'generic';
  }
}

/**
 * Get prompt type for a model
 * Flow: model → provider (from providers.json) → prompt type
 *
 * @param model - The model ID
 * @param fallbackProvider - Provider to use if model lookup fails
 * @returns The prompt type to use
 */
export function getPromptTypeForModel(model: string, fallbackProvider?: string): ProviderType {
  // First, try to look up the provider for this model
  const provider = getProviderForModel(model);

  if (provider) {
    return mapProviderToPromptType(provider);
  }

  // Fall back to the provided provider if model lookup fails
  if (fallbackProvider) {
    return mapProviderToPromptType(fallbackProvider);
  }

  // Default to generic
  return 'generic';
}

/**
 * Load a prompt file from a category subdirectory
 */
export function loadPrompt(category: string, name: string): string {
  const path = join(promptsDir, category, `${name}.txt`);
  return readFileSync(path, 'utf-8');
}

/**
 * Load the complete system prompt for a specific provider
 */
export function loadSystemPrompt(provider: ProviderType): string {
  const base = loadPrompt('system', 'base');
  const providerSpecific = loadPrompt('system', provider);
  return `${base}\n\n${providerSpecific}`;
}

/**
 * Load a tool description from the tools directory
 */
export function loadToolDescription(toolName: string): string {
  try {
    return loadPrompt('tools', toolName.toLowerCase());
  } catch {
    // Fallback if file doesn't exist (for tools without description files)
    return `Tool: ${toolName}`;
  }
}

/**
 * Environment info to inject into system prompts
 */
export interface EnvironmentInfo {
  cwd: string;
  platform: string;
  osVersion: string;
  date: string;
  isGitRepo: boolean;
}

/**
 * Get current environment info
 */
export function getEnvironmentInfo(cwd: string, isGitRepo: boolean = false): EnvironmentInfo {
  return {
    cwd,
    platform: process.platform,
    osVersion: `${os.type()} ${os.release()}`,
    date: new Date().toISOString().split('T')[0],
    isGitRepo,
  };
}

/**
 * Format environment info for injection into system prompt
 */
export function formatEnvironmentInfo(env: EnvironmentInfo): string {
  return `<env>
Working directory: ${env.cwd}
Is directory a git repo: ${env.isGitRepo ? 'Yes' : 'No'}
Platform: ${env.platform}
OS Version: ${env.osVersion}
Today's date: ${env.date}
</env>`;
}

/**
 * Build the complete system prompt with environment info
 */
export function buildSystemPrompt(
  provider: ProviderType,
  cwd: string,
  isGitRepo: boolean = false
): string {
  const prompt = loadSystemPrompt(provider);
  const envInfo = formatEnvironmentInfo(getEnvironmentInfo(cwd, isGitRepo));
  return prompt.replace('{{ENVIRONMENT}}', envInfo);
}

/**
 * Format memory context for injection into system prompt
 * Uses <claudeMd> tag for optimal compatibility with Claude models
 */
export function formatMemoryContext(memoryContext: string): string {
  if (!memoryContext) {
    return '';
  }

  return `
<claudeMd>
Codebase and user instructions are shown below. Be sure to adhere to these instructions. IMPORTANT: These instructions OVERRIDE any default behavior and you MUST follow them exactly as written.

${memoryContext}

IMPORTANT: this context may or may not be relevant to your tasks. You should not respond to this context unless it is highly relevant to your task.
</claudeMd>`;
}

/**
 * Build the complete system prompt with environment info and memory context
 */
export function buildSystemPromptWithMemory(
  provider: ProviderType,
  cwd: string,
  isGitRepo: boolean = false,
  memoryContext?: string
): string {
  let prompt = buildSystemPrompt(provider, cwd, isGitRepo);

  if (memoryContext) {
    prompt += formatMemoryContext(memoryContext);
  }

  return prompt;
}

/**
 * Build system prompt for a model
 * Flow: model → provider (from providers.json) → prompt
 *
 * This is the recommended way to build system prompts as it automatically
 * looks up the provider for the given model from ~/.gen/providers.json
 *
 * @param model - The model ID (e.g., "claude-sonnet-4-5@20250929")
 * @param cwd - Current working directory
 * @param isGitRepo - Whether the cwd is a git repository
 * @param memoryContext - Optional memory context to include
 * @param fallbackProvider - Provider to use if model lookup fails
 */
export function buildSystemPromptForModel(
  model: string,
  cwd: string,
  isGitRepo: boolean = false,
  memoryContext?: string,
  fallbackProvider?: string
): string {
  const promptType = getPromptTypeForModel(model, fallbackProvider);
  return buildSystemPromptWithMemory(promptType, cwd, isGitRepo, memoryContext);
}

/**
 * Debug utility to verify prompt loading at runtime
 * Set GEN_DEBUG_PROMPTS=1 for summary, GEN_DEBUG_PROMPTS=2 for full content
 */
export function debugPromptLoading(model: string, fallbackProvider?: string): void {
  const debugLevel = process.env.GEN_DEBUG_PROMPTS;
  if (!debugLevel || debugLevel === '0') {
    return;
  }

  const promptType = getPromptTypeForModel(model, fallbackProvider);
  const basePrompt = loadPrompt('system', 'base');
  const providerPrompt = loadPrompt('system', promptType);

  console.error('[PROMPT DEBUG] ================================');
  console.error(`[PROMPT DEBUG] Model: ${model}`);
  console.error(`[PROMPT DEBUG] Fallback Provider: ${fallbackProvider || 'none'}`);
  console.error(`[PROMPT DEBUG] Resolved Prompt Type: ${promptType}`);
  console.error(`[PROMPT DEBUG] base.txt lines: ${basePrompt.split('\n').length}`);
  console.error(`[PROMPT DEBUG] ${promptType}.txt lines: ${providerPrompt.split('\n').length}`);
  console.error(`[PROMPT DEBUG] Total chars: ${basePrompt.length + providerPrompt.length}`);

  // Verify key content
  const checks = [
    { name: 'Token minimization', pattern: /minimize output tokens/i },
    { name: 'CommonMark', pattern: /CommonMark/i },
    { name: 'Examples', pattern: /<example>/ },
    { name: 'Environment placeholder', pattern: /\{\{ENVIRONMENT\}\}/ },
  ];

  for (const check of checks) {
    const found = check.pattern.test(basePrompt);
    console.error(`[PROMPT DEBUG] ✓ ${check.name}: ${found ? 'OK' : 'MISSING'}`);
  }
  console.error('[PROMPT DEBUG] ================================');

  // Print full content if level >= 2
  if (debugLevel === '2') {
    console.error('\n[PROMPT DEBUG] === base.txt ===\n');
    console.error(basePrompt);
    console.error(`\n[PROMPT DEBUG] === ${promptType}.txt ===\n`);
    console.error(providerPrompt);
    console.error('\n[PROMPT DEBUG] === END ===\n');
  }
}
