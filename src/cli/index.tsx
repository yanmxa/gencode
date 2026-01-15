#!/usr/bin/env node
/**
 * GenCode CLI - Modern Ink-based Interactive Agent Interface
 * Beautiful terminal UI with React components
 */

import 'dotenv/config';
import { render } from 'ink';
import React from 'react';
import { App } from './components/App.js';
import type { AgentConfig } from '../agent/types.js';

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
function detectConfig(): AgentConfig {
  let provider: 'openai' | 'anthropic' | 'gemini' = 'gemini';
  let model = 'gemini-2.0-flash';

  // Auto-detect from API keys
  if (process.env.ANTHROPIC_API_KEY) {
    provider = 'anthropic';
    model = 'claude-sonnet-4-20250514';
  } else if (process.env.OPENAI_API_KEY) {
    provider = 'openai';
    model = 'gpt-4o';
  } else if (process.env.GOOGLE_API_KEY) {
    provider = 'gemini';
    model = 'gemini-2.0-flash';
  }

  // Override from env vars
  if (process.env.GENCODE_PROVIDER) {
    provider = process.env.GENCODE_PROVIDER as 'openai' | 'anthropic' | 'gemini';
  }
  if (process.env.GENCODE_MODEL) {
    model = process.env.GENCODE_MODEL;
  }

  return {
    provider,
    model,
    cwd: process.cwd(),
    maxTurns: 20,
  };
}

// ============================================================================
// CLI Arguments
// ============================================================================
function parseArgs() {
  const args = process.argv.slice(2);
  return {
    continue: args.includes('-c') || args.includes('--continue'),
    resume: args.includes('-r') || args.includes('--resume'),
    help: args.includes('-h') || args.includes('--help'),
  };
}

function printUsage(): void {
  console.log();
  console.log('  gencode - AI-Powered Coding Assistant');
  console.log();
  console.log('  Usage: gencode [options]');
  console.log();
  console.log('  Options:');
  console.log('    -c, --continue    Resume the most recent session');
  console.log('    -r, --resume      Select a session interactively');
  console.log('    -h, --help        Show this help');
  console.log();
  console.log('  Examples:');
  console.log('    gencode              Start new session');
  console.log('    gencode -c           Continue last session');
  console.log('    gencode -r           Pick a session');
  console.log();
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

  const config = detectConfig();

  // Render the Ink app
  render(
    <App
      config={config}
      resumeLatest={args.continue}
    />
  );
}

main().catch((error) => {
  console.error('Fatal error:', error);
  process.exit(1);
});
