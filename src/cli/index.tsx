#!/usr/bin/env node
/**
 * GenCode CLI - Modern Ink-based Interactive Agent Interface
 * Beautiful terminal UI with React components
 */

import 'dotenv/config';
import { render } from 'ink';
import React from 'react';
import { App } from './components/App.js';
import type { AgentConfig } from '../core/agent/types.js';
import { SettingsManager, ProvidersConfigManager, type Settings, type Provider } from '../base/config/index.js';
import type { AuthMethod } from '../core/providers/types.js';
import { inferProvider, inferAuthMethod } from '../core/providers/index.js';

// ============================================================================
// Proxy Setup
// ============================================================================
async function setupProxy(): Promise<void> {
  const proxyUrl = process.env.HTTP_PROXY || process.env.HTTPS_PROXY || process.env.http_proxy || process.env.https_proxy;

  if (proxyUrl) {
    try {
      const { ProxyAgent, setGlobalDispatcher } = await import('undici');
      const agent = new ProxyAgent(proxyUrl);
      setGlobalDispatcher(agent);
    } catch {
      // undici not available, proxy won't work
    }
  }
}

// ============================================================================
// Configuration
// ============================================================================
function detectConfig(settings: Settings, providersConfig: ProvidersConfigManager): AgentConfig {
  let provider: Provider = 'google';
  let authMethod: AuthMethod | undefined;
  let model = 'gemini-2.0-flash';

  // Priority 3: Auto-detect from API keys (lowest priority)
  // Check Vertex AI first (requires explicit opt-in)
  const useVertex = process.env.CLAUDE_CODE_USE_VERTEX === '1' || process.env.CLAUDE_CODE_USE_VERTEX === 'true';
  const hasVertexProject = !!(
    process.env.ANTHROPIC_VERTEX_PROJECT_ID ||
    process.env.GCLOUD_PROJECT ||
    process.env.GOOGLE_CLOUD_PROJECT
  );

  if (useVertex && hasVertexProject) {
    provider = 'anthropic';
    authMethod = 'vertex';
    model = 'claude-sonnet-4-5@20250929';
  } else if (process.env.ANTHROPIC_API_KEY) {
    provider = 'anthropic';
    authMethod = 'api_key';
    model = 'claude-sonnet-4-20250514';
  } else if (process.env.OPENAI_API_KEY) {
    provider = 'openai';
    authMethod = 'api_key';
    model = 'gpt-4o';
  } else if (process.env.GOOGLE_API_KEY) {
    provider = 'google';
    authMethod = 'api_key';
    model = 'gemini-2.0-flash';
  }

  // Priority 2: Saved settings (medium priority)
  if (settings.provider) {
    provider = settings.provider;
  }
  if (settings.model) {
    model = settings.model;
    // Try to infer provider and authMethod from cached models first
    if (!settings.provider) {
      const cached = providersConfig.inferProviderFromCache(model);
      if (cached) {
        provider = cached.provider;
        authMethod = cached.authMethod;
      } else {
        // Fall back to model name inference
        provider = inferProvider(model);
        authMethod = inferAuthMethod(model);
      }
    }
  }

  // Priority 1: Environment variables (highest priority - current session override)
  if (process.env.GEN_PROVIDER) {
    provider = process.env.GEN_PROVIDER as Provider;
  }
  if (process.env.GEN_MODEL) {
    model = process.env.GEN_MODEL;
    // Infer provider from model if not explicitly set
    if (!process.env.GEN_PROVIDER) {
      const cached = providersConfig.inferProviderFromCache(model);
      if (cached) {
        provider = cached.provider;
        authMethod = cached.authMethod;
      } else {
        provider = inferProvider(model);
        authMethod = inferAuthMethod(model);
      }
    }
  }

  return {
    provider,
    authMethod,
    model,
    cwd: process.cwd(),
    maxTurns: 20,
    compression: settings.compression,
  };
}

// ============================================================================
// CLI Arguments
// ============================================================================
function parseArgs() {
  const args = process.argv.slice(2);

  // Extract prompt value from -p "message" or --prompt "message"
  let prompt: string | undefined;
  for (let i = 0; i < args.length; i++) {
    if ((args[i] === '-p' || args[i] === '--prompt') && args[i + 1]) {
      prompt = args[i + 1];
      break;
    }
  }

  return {
    continue: args.includes('-c') || args.includes('--continue'),
    resume: args.includes('-r') || args.includes('--resume'),
    help: args.includes('-h') || args.includes('--help'),
    prompt,
  };
}

function printUsage(): void {
  console.log();
  console.log('  gencode - AI-Powered Coding Assistant');
  console.log();
  console.log('  Usage: gencode [options]');
  console.log();
  console.log('  Options:');
  console.log('    -c, --continue       Resume the most recent session');
  console.log('    -r, --resume         Select a session interactively');
  console.log('    -p, --prompt <msg>   Run a single prompt (non-interactive)');
  console.log('    -h, --help           Show this help');
  console.log();
  console.log('  Examples:');
  console.log('    gencode                    Start new session');
  console.log('    gencode -c                 Continue last session');
  console.log('    gencode -r                 Pick a session');
  console.log('    gencode -p "2+2"           Run single prompt');
  console.log();
}

// ============================================================================
// Non-interactive mode
// ============================================================================
async function runNonInteractive(
  prompt: string,
  config: AgentConfig,
  options: { resumeLatest?: boolean; settings?: Settings } = {}
): Promise<void> {
  const { Agent } = await import('../core/agent/agent.js');

  const agent = new Agent(config);

  // Initialize hooks system if configured
  if (options.settings?.hooks) {
    agent.initializeHooks(options.settings.hooks);
  }

  // Resume previous session if -c flag is used
  if (options.resumeLatest) {
    const resumed = await agent.resumeLatest();
    if (resumed) {
      const session = agent.getSessionManager().getCurrent();
      if (process.env.GEN_DEBUG) {
        console.error(`[session] Resumed session: ${session?.metadata.id} (${session?.messages.length} messages)`);
      }
    } else {
      if (process.env.GEN_DEBUG) {
        console.error(`[session] No previous session found, starting new session`);
      }
    }
  }

  let response = '';
  for await (const event of agent.run(prompt)) {
    switch (event.type) {
      case 'text':
        response += event.text;
        break;
      case 'tool_start':
        console.error(`[tool] ${event.name}`);
        break;
      case 'error':
        console.error(`[error] ${event.error.message}`);
        break;
      case 'done':
        break;
    }
  }

  console.log(response);
}

// ============================================================================
// Main
// ============================================================================
async function main() {
  const args = parseArgs();

  if (args.help) {
    printUsage();
    process.exit(0);
  }

  await setupProxy();

  // Load saved settings and providers config
  const settingsManager = new SettingsManager();
  const settings = await settingsManager.load();

  const providersConfig = new ProvidersConfigManager();
  await providersConfig.load();

  const config = detectConfig(settings, providersConfig);

  // Debug: Log provider/model info
  if (process.env.GEN_DEBUG) {
    console.error(`[config] Provider: ${config.provider}, Auth: ${config.authMethod || 'api_key'}, Model: ${config.model}`);
  }

  // Non-interactive mode with -p flag
  if (args.prompt) {
    await runNonInteractive(args.prompt, config, { resumeLatest: args.continue, settings });
    return;
  }

  // Render the Ink app
  render(
    <App
      config={config}
      settingsManager={settingsManager}
      resumeLatest={args.continue}
      permissionSettings={settings.permissions}
      hooksConfig={settings.hooks}
    />
  );
}

main().catch((error) => {
  console.error('Fatal error:', error);
  process.exit(1);
});
