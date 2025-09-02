#!/usr/bin/env node
/**
 * MyCode CLI - Interactive Agent Interface with Session Management
 */

import 'dotenv/config';
import * as readline from 'readline';
import { Agent } from '../agent/index.js';
import type { AgentConfig } from '../agent/types.js';
import {
  printHeader,
  printSeparator,
  printUserMessage,
  printAssistantMessage,
  printToolCall,
  printToolResult,
  printError,
  printInfo,
  printSuccess,
  promptConfirm,
  createSpinner,
  colors,
  printTable,
} from './ui.js';

// ============================================================================
// Proxy Setup
// ============================================================================

async function setupProxy(): Promise<void> {
  const proxyUrl = process.env.HTTPS_PROXY || process.env.HTTP_PROXY;
  if (proxyUrl) {
    const { setGlobalDispatcher, ProxyAgent } = await import('undici');
    setGlobalDispatcher(new ProxyAgent(proxyUrl));
    printInfo(`Using proxy: ${proxyUrl}`);
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
      const sessions = await agent.listSessions();
      if (sessions.length === 0) {
        printInfo('No sessions found.');
      } else {
        console.log();
        printInfo(`Found ${sessions.length} session(s):`);
        console.log();
        printTable(
          ['#', 'ID', 'Title', 'Messages', 'Updated'],
          sessions.map((s, i) => [
            String(i + 1),
            s.id.slice(0, 12),
            s.title.slice(0, 30),
            String(s.messageCount),
            new Date(s.updatedAt).toLocaleString(),
          ])
        );
      }
      return true;
    }

    case 'resume': {
      let success = false;
      if (arg) {
        // Resume by ID or index
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
        // Resume latest
        success = await agent.resumeLatest();
      }

      if (success) {
        const history = agent.getHistory();
        printSuccess(`Resumed session: ${agent.getSessionId()}`);
        printInfo(`Loaded ${history.length} message(s)`);
      } else {
        printError('Failed to resume session. Use /sessions to list available sessions.');
      }
      return true;
    }

    case 'new': {
      const title = parts.slice(1).join(' ') || undefined;
      const sessionId = await agent.startSession(title);
      printSuccess(`Started new session: ${sessionId}`);
      return true;
    }

    case 'fork': {
      const title = parts.slice(1).join(' ') || undefined;
      const forkedId = await agent.forkSession(title);
      if (forkedId) {
        printSuccess(`Forked to new session: ${forkedId}`);
      } else {
        printError('No active session to fork.');
      }
      return true;
    }

    case 'delete': {
      if (!arg) {
        printError('Usage: /delete <session-id or index>');
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
        printSuccess(`Deleted session: ${sessionId}`);
      } else {
        printError(`Failed to delete session: ${sessionId}`);
      }
      return true;
    }

    case 'save': {
      await agent.saveSession();
      printSuccess('Session saved.');
      return true;
    }

    case 'info': {
      const sessionId = agent.getSessionId();
      if (sessionId) {
        const history = agent.getHistory();
        printInfo(`Session ID: ${colors.highlight(sessionId)}`);
        printInfo(`Messages: ${history.length}`);
      } else {
        printInfo('No active session.');
      }
      return true;
    }

    case 'help': {
      console.log();
      printInfo('Session Commands:');
      console.log(colors.muted('  /sessions, /list') + '     List all sessions');
      console.log(colors.muted('  /resume [id|#]') + '      Resume a session (latest if no arg)');
      console.log(colors.muted('  /new [title]') + '        Start a new session');
      console.log(colors.muted('  /fork [title]') + '       Fork current session');
      console.log(colors.muted('  /delete <id|#>') + '      Delete a session');
      console.log(colors.muted('  /save') + '               Save current session');
      console.log(colors.muted('  /info') + '               Show current session info');
      console.log(colors.muted('  /clear') + '              Clear current conversation');
      console.log(colors.muted('  /help') + '               Show this help');
      console.log(colors.muted('  exit, quit') + '          Exit the CLI');
      return true;
    }

    case 'clear': {
      agent.clearHistory();
      await agent.startSession();
      console.clear();
      printHeader();
      printSuccess('Conversation cleared. Started new session.');
      return true;
    }

    default:
      return false;
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
  await setupProxy();

  printHeader();

  const config = getAgentConfig();
  printInfo(`Provider: ${colors.highlight(config.provider)} | Model: ${colors.highlight(config.model)}`);
  printInfo(`Working directory: ${colors.muted(config.cwd)}`);
  printSeparator();
  console.log();
  printInfo('Type /help for commands. Sessions auto-saved to ~/.mycode/sessions/');
  console.log();

  const agent = new Agent(config);

  // Check for --resume flag
  const args = process.argv.slice(2);
  if (args.includes('--resume') || args.includes('-r')) {
    const resumed = await agent.resumeLatest();
    if (resumed) {
      printSuccess(`Resumed latest session: ${agent.getSessionId()}`);
      printInfo(`Loaded ${agent.getHistory().length} message(s)`);
    } else {
      printInfo('No previous session found. Starting new session.');
    }
  }

  // Set up permission confirmation
  agent.setConfirmCallback(async (tool: string, input: unknown) => {
    console.log();
    console.log(colors.warning(`Permission required for ${colors.highlight(tool)}`));
    const inputStr = JSON.stringify(input, null, 2);
    const lines = inputStr.split('\n').slice(0, 5);
    for (const line of lines) {
      console.log(colors.muted('  ' + line));
    }
    return await promptConfirm('Allow this operation?');
  });

  // REPL loop
  const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  const prompt = (): void => {
    rl.question(colors.primary('❯ '), async (input) => {
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
          printError(`Unknown command: ${trimmed}. Type /help for available commands.`);
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
  printInfo('Interrupted. Session saved. Goodbye!');
  process.exit(0);
});

main().catch((error) => {
  printError(error.message);
  process.exit(1);
});
