/**
 * Prompt Loader - Load prompts from .txt files
 *
 * This module provides utilities for loading system prompts and tool descriptions
 * from separate .txt files, enabling easier maintenance and prompt engineering.
 */

import { readFileSync, existsSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import * as os from 'os';

const __dirname = dirname(fileURLToPath(import.meta.url));

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

export type ProviderType = 'anthropic' | 'openai' | 'gemini';

/**
 * Map provider names to prompt types
 * vertex-ai uses anthropic prompts since it's Claude on GCP
 */
export function mapProviderToPromptType(provider: string): ProviderType {
  switch (provider) {
    case 'anthropic':
    case 'vertex-ai':
      return 'anthropic';
    case 'openai':
      return 'openai';
    case 'gemini':
      return 'gemini';
    default:
      return 'openai'; // Default to openai prompts
  }
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
