#!/usr/bin/env node
/**
 * mycode CLI - Modern Interactive Agent Interface
 * Beautiful terminal UI with session management
 */

import 'dotenv/config';
import * as readline from 'readline';
import { Agent } from '../agent/index.js';
import type { AgentConfig } from '../agent/types.js';
import type { Message, MessageContent } from '../providers/types.js';
import {
  printCompactHeader,
  printUserMessage,
  printAssistantMessage,
  printToolCall,
  printToolResult,
  printError,
  printInfo,
  printSuccess,
  promptConfirm,
  createSpinner,
  printTable,
  printHelp,
  printWelcome,
  printPermissionRequest,
  printSessionInfo,
  getPromptSymbol,
  theme,
} from './ui.js';
import { pickSession } from './session-picker.js';

// ============================================================================
// History Display
// ============================================================================

function printHistory(messages: Message[]): void {
  if (messages.length === 0) return;

  console.log();
  printInfo('Session history restored');
  console.log();

  for (const msg of messages) {
    if (msg.role === 'user') {
      if (typeof msg.content === 'string') {
        printUserMessage(msg.content);
      }
    } else if (msg.role === 'assistant') {
      if (typeof msg.content === 'string') {
        printAssistantMessage(msg.content);
      } else if (Array.isArray(msg.content)) {
        const textParts = (msg.content as MessageContent[])
          .filter((c) => c.type === 'text')
          .map((c) => (c as { type: 'text'; text: string }).text)
          .join('');
        if (textParts) {
          printAssistantMessage(textParts);
        }

        const toolCalls = (msg.content as MessageContent[]).filter((c) => c.type === 'tool_use');
        for (const tc of toolCalls) {
          const toolCall = tc as { type: 'tool_use'; name: string };
          console.log(theme.textMuted(`  ⚙ ${toolCall.name}`));
        }
      }
    }
  }
}

// ============================================================================
// Proxy Setup
// ============================================================================

async function setupProxy(): Promise<void> {
  const proxyUrl = process.env.HTTPS_PROXY || process.env.HTTP_PROXY;
  if (proxyUrl) {
    const { setGlobalDispatcher, ProxyAgent } = await import('undici');
    setGlobalDispatcher(new ProxyAgent(proxyUrl));
    printInfo(`Proxy: ${proxyUrl}`);
  }
}

// ============================================================================
// Agent Configuration
// ============================================================================

function getAgentConfig(): AgentConfig {
  let provider: 'openai' | 'anthropic' | 'gemini' = 'gemini';
  let model = 'gemini-2.0-flash';

  if (process.env.ANTHROPIC_API_KEY) {
    provider = 'anthropic';
    model = 'claude-sonnet-4-20250514';
  } else if (process.env.OPENAI_API_KEY) {
    provider = 'openai';
    model = 'gpt-4o';
  } else if (process.env.GOOGLE_API_KEY || process.env.GEMINI_API_KEY) {
    provider = 'gemini';
    model = 'gemini-2.0-flash';
  }

  if (process.env.MYCODE_PROVIDER) {
    provider = process.env.MYCODE_PROVIDER as 'openai' | 'anthropic' | 'gemini';
  }
  if (process.env.MYCODE_MODEL) {
    model = process.env.MYCODE_MODEL;
  }

  return {
    provider,
    model,
    cwd: process.cwd(),
    maxTurns: 20,
  };
}

// ============================================================================
// Session Commands
// ============================================================================

async function handleSessionCommand(agent: Agent, command: string): Promise<boolean> {
  const parts = command.slice(1).split(/\s+/);
  const cmd = parts[0]?.toLowerCase();
  const arg = parts[1];

  switch (cmd) {
    case 'sessions':
    case 'list': {
      const showAll = arg === '--all' || arg === '-a';
      const sessions = await agent.getSessionManager().list({ all: showAll });
      if (sessions.length === 0) {
        printInfo(showAll ? 'No sessions found.' : 'No sessions for this project. Use --all to see all.');
      } else {
        console.log();
        printInfo(`${sessions.length} session${sessions.length > 1 ? 's' : ''} ${showAll ? '(all projects)' : '(this project)'}`);
        console.log();
        printTable(
          ['#', 'ID', 'Title', 'Msgs', 'Updated'],
          sessions.map((s, i) => [
            String(i + 1),
            s.id.slice(0, 8),
            s.title.slice(0, 35),
            String(s.messageCount),
            formatTime(s.updatedAt),
          ])
        );
      }
      return true;
    }

    case 'resume': {
      let success = false;
      if (arg) {
        const index = parseInt(arg, 10);
        if (!isNaN(index)) {
          const sessions = await agent.listSessions();
          if (index >= 1 && index <= sessions.length) {
            success = await agent.resumeSession(sessions[index - 1].id);
          }
        } else {
          success = await agent.resumeSession(arg);
        }
      } else {
        success = await agent.resumeLatest();
      }

      if (success) {
        printHistory(agent.getHistory());
      } else {
        printError('Failed to resume. Use /sessions to list available sessions.');
      }
      return true;
    }

    case 'new': {
      const title = parts.slice(1).join(' ') || undefined;
      const sessionId = await agent.startSession(title);
      printSuccess(`New session: ${sessionId.slice(0, 8)}`);
      return true;
    }

    case 'fork': {
      const title = parts.slice(1).join(' ') || undefined;
      const forkedId = await agent.forkSession(title);
      if (forkedId) {
        printSuccess(`Forked session: ${forkedId.slice(0, 8)}`);
      } else {
        printError('No active session to fork.');
      }
      return true;
    }

    case 'delete': {
      if (!arg) {
        printError('Usage: /delete <session-id or #>');
        return true;
      }

      let sessionId = arg;
      const index = parseInt(arg, 10);
      if (!isNaN(index)) {
        const sessions = await agent.listSessions();
        if (index >= 1 && index <= sessions.length) {
          sessionId = sessions[index - 1].id;
        }
      }

      const deleted = await agent.deleteSession(sessionId);
      if (deleted) {
        printSuccess(`Deleted: ${sessionId.slice(0, 8)}`);
      } else {
        printError(`Failed to delete: ${sessionId}`);
      }
      return true;
    }

    case 'save': {
      await agent.saveSession();
      printSuccess('Session saved');
      return true;
    }

    case 'info': {
      const sessionId = agent.getSessionId();
      if (sessionId) {
        printSessionInfo(sessionId, agent.getHistory().length);
      } else {
        printInfo('No active session');
      }
      return true;
    }

    case 'help': {
      printHelp();
      return true;
    }

    case 'clear': {
      agent.clearHistory();
      await agent.startSession();
      console.clear();
      const config = getAgentConfig();
      printCompactHeader(config.provider, config.model, config.cwd ?? process.cwd());
      printSuccess('Conversation cleared');
      return true;
    }

    default:
      return false;
  }
}

// ============================================================================
// Utilities
// ============================================================================

function formatTime(dateStr: string): string {
  const now = Date.now();
  const date = new Date(dateStr).getTime();
  const diff = now - date;

  const minutes = Math.floor(diff / 60000);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (minutes < 60) return `${minutes}m`;
  if (hours < 24) return `${hours}h`;
  if (days < 7) return `${days}d`;
  return new Date(dateStr).toLocaleDateString();
}

// ============================================================================
// CLI Arguments
// ============================================================================

function parseArgs(): { continue: boolean; resume: boolean; help: boolean } {
  const args = process.argv.slice(2);
  return {
    continue: args.includes('-c') || args.includes('--continue'),
    resume: args.includes('-r') || args.includes('--resume'),
    help: args.includes('-h') || args.includes('--help'),
  };
}

function printUsage(): void {
  console.log();
  console.log(theme.brand.bold('  mycode') + theme.textMuted(' - AI-Powered Coding Assistant'));
  console.log();
  console.log(theme.highlight('  Usage:') + theme.textMuted(' mycode [options]'));
  console.log();
  console.log(theme.highlight('  Options:'));
  console.log(`    ${theme.primary('-c, --continue')}    Resume the most recent session`);
  console.log(`    ${theme.primary('-r, --resume')}      Select a session interactively`);
  console.log(`    ${theme.primary('-h, --help')}        Show this help`);
  console.log();
  console.log(theme.highlight('  Examples:'));
  console.log(theme.textMuted('    mycode              Start new session'));
  console.log(theme.textMuted('    mycode -c           Continue last session'));
  console.log(theme.textMuted('    mycode -r           Pick a session'));
  console.log();
}

async function selectSession(agent: Agent, showAll = false): Promise<boolean> {
  // eslint-disable-next-line no-constant-condition
  while (true) {
    const sessions = await agent.getSessionManager().list({ all: showAll });

    if (sessions.length === 0) {
      if (showAll) {
        printInfo('No sessions found');
      } else {
        printInfo('No sessions for this project. Press A for all.');
      }
      return false;
    }

    const result = await pickSession({ sessions, showAllProjects: showAll });

    switch (result.action) {
      case 'select':
        if (result.sessionId) {
          const success = await agent.resumeSession(result.sessionId);
          if (success) {
            printHistory(agent.getHistory());
            return true;
          } else {
            printError('Failed to resume session');
            return false;
          }
        }
        return false;

      case 'toggle-all':
        showAll = !showAll;
        continue;

      case 'new':
        printInfo('Starting new session');
        return false;

      case 'cancel':
        printInfo('Starting new session');
        return false;
    }
  }
}

// ============================================================================
// Main REPL
// ============================================================================

async function runAgent(agent: Agent, prompt: string): Promise<void> {
  const spinner = createSpinner('Thinking...');
  spinner.start();

  let currentText = '';
  let spinnerStopped = false;

  const stopSpinner = () => {
    if (!spinnerStopped) {
      spinner.stop();
      spinnerStopped = true;
    }
  };

  try {
    for await (const event of agent.run(prompt)) {
      switch (event.type) {
        case 'text':
          stopSpinner();
          currentText += event.text;
          break;

        case 'tool_start':
          stopSpinner();
          printToolCall(event.name, event.input);
          break;

        case 'tool_result':
          printToolResult(event.name, event.result.success, event.result.output);
          spinner.text = 'Thinking...';
          spinner.start();
          spinnerStopped = false;
          break;

        case 'error':
          stopSpinner();
          printError(event.error.message);
          return;

        case 'done':
          stopSpinner();
          if (currentText) {
            printAssistantMessage(currentText);
          }
          return;
      }
    }
  } catch (error) {
    stopSpinner();
    printError(error instanceof Error ? error.message : String(error));
  }
}

async function main(): Promise<void> {
  const args = parseArgs();

  if (args.help) {
    printUsage();
    process.exit(0);
  }

  await setupProxy();

  const config = getAgentConfig();
  printCompactHeader(config.provider, config.model, config.cwd ?? process.cwd());

  const agent = new Agent(config);

  // Handle session flags
  if (args.continue) {
    const resumed = await agent.resumeLatest();
    if (resumed) {
      printHistory(agent.getHistory());
    } else {
      printInfo('No previous session. Starting new.');
    }
  } else if (args.resume) {
    await selectSession(agent);
  } else {
    printWelcome();
  }

  console.log();

  // Set up permission confirmation
  agent.setConfirmCallback(async (tool: string, input: unknown) => {
    printPermissionRequest(tool, input);
    return await promptConfirm('Allow this operation?');
  });

  // REPL loop
  const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  const promptSymbol = getPromptSymbol();

  const prompt = (): void => {
    rl.question(promptSymbol, async (input) => {
      const trimmed = input.trim();

      if (!trimmed) {
        prompt();
        return;
      }

      // Exit commands
      if (trimmed.toLowerCase() === 'exit' || trimmed.toLowerCase() === 'quit') {
        await agent.saveSession();
        console.log();
        printInfo('Session saved. Goodbye!');
        rl.close();
        process.exit(0);
      }

      // Slash commands
      if (trimmed.startsWith('/')) {
        const handled = await handleSessionCommand(agent, trimmed);
        if (!handled) {
          printError(`Unknown command: ${trimmed}. Type /help for help.`);
        }
        console.log();
        prompt();
        return;
      }

      // Regular prompt
      printUserMessage(trimmed);
      await runAgent(agent, trimmed);
      console.log();
      prompt();
    });
  };

  prompt();
}

// Handle graceful shutdown
process.on('SIGINT', async () => {
  console.log();
  printInfo('Session saved. Goodbye!');
  process.exit(0);
});

main().catch((error) => {
  printError(error.message);
  process.exit(1);
});
