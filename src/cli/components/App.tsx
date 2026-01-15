/**
 * Main App Component - Compact Ink-based TUI
 * Inspired by Claude Code and Gemini CLI design patterns
 */
import { useState, useEffect, useCallback, useRef } from 'react';
import { Box, Text, useApp, useInput, Static } from 'ink';
import { Agent } from '../../agent/index.js';
import type { AgentConfig } from '../../agent/types.js';
import {
  UserMessage,
  AssistantMessage,
  ToolCall,
  ToolResult,
  InfoMessage,
  WelcomeMessage,
  CompletionMessage,
} from './Messages.js';
import { Header } from './Header.js';
import { ProgressBar } from './Spinner.js';
import { PromptInput, ConfirmPrompt } from './Input.js';
import { ModelSelector } from './ModelSelector.js';
import { ProviderManager } from './ProviderManager.js';
import { CommandSuggestions, getFilteredCommands } from './CommandSuggestions.js';
import { colors, icons } from './theme.js';
import type { ProviderName } from '../../providers/index.js';

// Types
interface HistoryItem {
  id: string;
  type: 'header' | 'welcome' | 'user' | 'assistant' | 'tool_call' | 'tool_result' | 'info' | 'completion';
  content: string;
  meta?: Record<string, unknown>;
}

interface ConfirmState {
  tool: string;
  input: Record<string, unknown>;
  resolve: (confirmed: boolean) => void;
}

interface SettingsManager {
  save: (settings: { model?: string }) => Promise<void>;
}

interface Session {
  id: string;
  title: string;
  updatedAt: string;
}

interface AppProps {
  config: AgentConfig;
  settingsManager?: SettingsManager;
  resumeLatest?: boolean;
}

// ============================================================================
// Hooks
// ============================================================================
function useAgent(config: AgentConfig) {
  const [agent] = useState(() => new Agent(config));
  return agent;
}

// ============================================================================
// Help Component
// ============================================================================
function HelpPanel() {
  const commands: [string, string][] = [
    ['/model [name]', 'Switch model'],
    ['/provider', 'Manage providers'],
    ['/sessions', 'List sessions'],
    ['/resume [n]', 'Resume session'],
    ['/new', 'New session'],
    ['/save', 'Save session'],
    ['/clear', 'Clear chat'],
    ['/init', 'Generate AGENT.md'],
    ['/memory', 'Show memory files'],
  ];

  return (
    <Box flexDirection="column">
      {commands.map(([cmd, desc]) => (
        <Text key={cmd}>
          <Text color={colors.primary}>{cmd.padEnd(14)}</Text>
          <Text color={colors.textMuted}>{desc}</Text>
        </Text>
      ))}
    </Box>
  );
}

interface SessionsTableProps {
  sessions: Session[];
}

function SessionsTable({ sessions }: SessionsTableProps) {
  const formatTime = (dateStr: string) => {
    const diff = Date.now() - new Date(dateStr).getTime();
    const mins = Math.floor(diff / 60000);
    const hrs = Math.floor(mins / 60);
    const days = Math.floor(hrs / 24);
    if (mins < 60) return `${mins}m`;
    if (hrs < 24) return `${hrs}h`;
    return `${days}d`;
  };

  return (
    <Box flexDirection="column">
      {sessions.slice(0, 6).map((s, i) => (
        <Text key={s.id}>
          <Text color={colors.textMuted}>{String(i + 1).padEnd(2)}</Text>
          <Text color={colors.primary}>{s.id.slice(0, 7).padEnd(8)}</Text>
          <Text>{s.title.slice(0, 25).padEnd(26)}</Text>
          <Text color={colors.textMuted}>{formatTime(s.updatedAt)}</Text>
        </Text>
      ))}
    </Box>
  );
}

// ============================================================================
// Main App
// ============================================================================
export function App({ config, settingsManager, resumeLatest }: AppProps) {
  const { exit } = useApp();
  const agent = useAgent(config);

  // Generate unique ID
  const genId = () => Math.random().toString(36).slice(2);

  // Initial header item
  const cwd = config.cwd || process.cwd();
  const home = process.env.HOME || '';
  const cwdDisplay = cwd.startsWith(home) ? '~' + cwd.slice(home.length) : cwd;
  const cwdShort = cwdDisplay.length > 35 ? '...' + cwdDisplay.slice(-32) : cwdDisplay;

  const initialHistory: HistoryItem[] = [
    {
      id: 'header',
      type: 'header',
      content: '',
      meta: { provider: config.provider, model: config.model, cwd: cwdShort },
    },
    {
      id: 'welcome',
      type: 'welcome',
      content: config.model,
      meta: { model: config.model },
    },
  ];

  // State
  const [history, setHistory] = useState<HistoryItem[]>(initialHistory);
  const [input, setInput] = useState('');
  const [isProcessing, setIsProcessing] = useState(false);
  const [isThinking, setIsThinking] = useState(false);
  const [streamingText, setStreamingText] = useState('');
  const streamingTextRef = useRef(''); // Track current streaming text for closure
  const [confirmState, setConfirmState] = useState<ConfirmState | null>(null);
  const [showModelSelector, setShowModelSelector] = useState(false);
  const [showProviderManager, setShowProviderManager] = useState(false);
  const [currentModel, setCurrentModel] = useState(config.model);
  const [cmdSuggestionIndex, setCmdSuggestionIndex] = useState(0);
  const [inputKey, setInputKey] = useState(0); // Force cursor to end after autocomplete

  // Check if showing command suggestions
  const showCmdSuggestions = input.startsWith('/') && !isProcessing;
  const cmdSuggestions = showCmdSuggestions ? getFilteredCommands(input) : [];

  // Reset suggestion index when input changes
  useEffect(() => {
    setCmdSuggestionIndex(0);
  }, [input]);

  // Add to history
  const addHistory = useCallback((item: Omit<HistoryItem, 'id'>) => {
    setHistory((prev) => [...prev, { ...item, id: genId() }]);
  }, []);

  // Initialize
  useEffect(() => {
    const init = async () => {
      agent.setConfirmCallback(async (tool: string, toolInput: unknown) => {
        return new Promise<boolean>((resolve) => {
          setConfirmState({ tool, input: toolInput as Record<string, unknown>, resolve });
        });
      });

      if (resumeLatest) {
        const resumed = await agent.resumeLatest();
        if (resumed) {
          addHistory({ type: 'info', content: 'Session restored' });
        }
      }
    };
    init();
  }, [agent, resumeLatest, addHistory]);

  // Handle confirm
  const handleConfirm = (confirmed: boolean) => {
    if (confirmState) {
      confirmState.resolve(confirmed);
      setConfirmState(null);
    }
  };

  // Handle model selection
  const handleModelSelect = async (model: string, providerId?: ProviderName) => {
    agent.setModel(model);
    setCurrentModel(model);
    setShowModelSelector(false);

    if (providerId) {
      addHistory({ type: 'info', content: `${providerId}: ${model}` });
    } else {
      addHistory({ type: 'info', content: `Model: ${model}` });
    }

    // Save to settings for next startup
    if (settingsManager) {
      await settingsManager.save({ model });
    }
  };

  const handleModelCancel = () => {
    setShowModelSelector(false);
  };

  // Handle command
  const handleCommand = async (cmd: string): Promise<boolean> => {
    const parts = cmd.slice(1).split(/\s+/);
    const command = parts[0]?.toLowerCase();
    const arg = parts[1];

    switch (command) {
      case 'help':
        addHistory({ type: 'info', content: '__HELP__' });
        return true;

      case 'sessions':
      case 'list': {
        const showAll = arg === '--all' || arg === '-a';
        const sessions = await agent.getSessionManager().list({ all: showAll });
        if (sessions.length === 0) {
          addHistory({ type: 'info', content: 'No sessions' });
        } else {
          addHistory({ type: 'info', content: '__SESSIONS__', meta: { input: sessions } });
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
        addHistory({ type: 'info', content: success ? 'Restored' : 'Failed' });
        return true;
      }

      case 'new': {
        const title = parts.slice(1).join(' ') || undefined;
        const sessionId = await agent.startSession(title);
        addHistory({ type: 'info', content: `New: ${sessionId.slice(0, 8)}` });
        return true;
      }

      case 'save':
        await agent.saveSession();
        addHistory({ type: 'info', content: 'Saved' });
        return true;

      case 'info': {
        const sessionId = agent.getSessionId();
        addHistory({
          type: 'info',
          content: sessionId
            ? `${sessionId.slice(0, 8)} · ${agent.getHistory().length} msgs`
            : 'No session',
        });
        return true;
      }

      case 'clear':
        agent.clearHistory();
        await agent.startSession();
        setHistory(initialHistory);
        return true;

      case 'model': {
        if (arg) {
          // Direct model switch: /model gpt-4o
          agent.setModel(arg);
          setCurrentModel(arg);
          addHistory({ type: 'info', content: `Model: ${arg}` });
        } else {
          // Show interactive model selector
          setShowModelSelector(true);
        }
        return true;
      }

      case 'provider': {
        setShowProviderManager(true);
        return true;
      }

      case 'init': {
        addHistory({ type: 'info', content: '/init command not available in this version' });
        return true;
      }

      case 'memory': {
        addHistory({ type: 'info', content: '/memory command not available in this version' });
        return true;
      }

      default:
        return false;
    }
  };

  // Interrupt ref for ESC handling
  const interruptFlagRef = useRef(false);

  // Run agent
  const runAgent = async (prompt: string) => {
    setIsProcessing(true);
    setIsThinking(true);
    setStreamingText('');
    streamingTextRef.current = '';
    interruptFlagRef.current = false;
    const startTime = Date.now();

    try {
      for await (const event of agent.run(prompt)) {
        // Check for interrupt
        if (interruptFlagRef.current) {
          break;
        }

        switch (event.type) {
          case 'text':
            setIsThinking(false);
            streamingTextRef.current += event.text;
            setStreamingText(streamingTextRef.current);
            break;

          case 'tool_start':
            setIsThinking(false);
            if (streamingTextRef.current) {
              addHistory({ type: 'assistant', content: streamingTextRef.current });
              streamingTextRef.current = '';
              setStreamingText('');
            }
            addHistory({
              type: 'tool_call',
              content: event.name,
              meta: { toolName: event.name, input: event.input },
            });
            break;

          case 'tool_result':
            addHistory({
              type: 'tool_result',
              content: event.result.output,
              meta: {
                toolName: event.name,
                success: event.result.success,
                metadata: event.result.metadata,
              },
            });
            setIsThinking(true);
            break;

          case 'error':
            setIsThinking(false);
            addHistory({ type: 'info', content: `Error: ${event.error.message}` });
            break;

          case 'done':
            if (streamingTextRef.current) {
              addHistory({ type: 'assistant', content: streamingTextRef.current });
              streamingTextRef.current = '';
              setStreamingText('');
            }
            // Add completion message with duration
            const durationMs = Date.now() - startTime;
            addHistory({ type: 'completion', content: '', meta: { durationMs } });
            break;
        }
      }
    } catch (error) {
      addHistory({
        type: 'info',
        content: `Error: ${error instanceof Error ? error.message : String(error)}`,
      });
    }

    setIsProcessing(false);
    setIsThinking(false);
  };

  // Handle submit
  const handleSubmit = async (text: string) => {
    const trimmed = text.trim();
    if (!trimmed) return;

    // Auto-complete command on Enter if no exact match
    if (trimmed.startsWith('/') && cmdSuggestions.length > 0) {
      const exactMatch = cmdSuggestions.find(
        (c) => c.name === trimmed || c.name.startsWith(trimmed + ' ')
      );
      if (!exactMatch) {
        // No exact match, complete to best match
        const bestMatch = cmdSuggestions[cmdSuggestionIndex];
        setInput(bestMatch.name + ' ');
        setInputKey((k) => k + 1); // Force cursor to end
        return;
      }
    }

    setInput('');

    if (trimmed.toLowerCase() === 'exit' || trimmed.toLowerCase() === 'quit') {
      await agent.saveSession();
      setTimeout(() => exit(), 50);
      return;
    }

    if (trimmed.startsWith('/')) {
      const handled = await handleCommand(trimmed);
      if (!handled) {
        addHistory({ type: 'info', content: `Unknown: ${trimmed}` });
      }
      return;
    }

    addHistory({ type: 'user', content: trimmed });
    await runAgent(trimmed);
  };

  // Keyboard shortcuts
  useInput((inputChar, key) => {
    if (key.ctrl && inputChar === 'c') {
      agent.saveSession().then(() => exit());
    }

    // ESC to interrupt processing
    if (key.escape && isProcessing) {
      interruptFlagRef.current = true;
      setIsProcessing(false);
      setStreamingText('');
      streamingTextRef.current = '';
      addHistory({ type: 'info', content: 'Interrupted' });
    }

    // Command suggestion navigation
    if (showCmdSuggestions && cmdSuggestions.length > 0) {
      if (key.upArrow) {
        setCmdSuggestionIndex((i) => Math.max(0, i - 1));
      } else if (key.downArrow) {
        setCmdSuggestionIndex((i) => Math.min(cmdSuggestions.length - 1, i + 1));
      } else if (key.tab) {
        // Autocomplete with selected suggestion
        const selected = cmdSuggestions[cmdSuggestionIndex];
        if (selected) {
          setInput(selected.name + ' ');
          setCmdSuggestionIndex(0);
          setInputKey((k) => k + 1); // Force cursor to end
        }
      }
    }
  });

  // Render history item
  const renderHistoryItem = (item: HistoryItem) => {
    switch (item.type) {
      case 'header':
        return (
          <Header
            provider={(item.meta?.provider as string) || ''}
            model={(item.meta?.model as string) || ''}
            cwd={(item.meta?.cwd as string) || ''}
          />
        );
      case 'welcome':
        return <WelcomeMessage model={(item.meta?.model as string) || item.content} />;
      case 'user':
        return <UserMessage text={item.content} />;
      case 'assistant':
        return <AssistantMessage text={item.content} />;
      case 'tool_call':
        return (
          <ToolCall
            name={(item.meta?.toolName as string) || ''}
            input={item.meta?.input as Record<string, unknown>}
          />
        );
      case 'tool_result':
        return (
          <ToolResult
            name={(item.meta?.toolName as string) || ''}
            success={(item.meta?.success as boolean) ?? true}
            output={item.content}
            metadata={item.meta?.metadata as Record<string, unknown> | undefined}
          />
        );
      case 'info':
        if (item.content === '__HELP__') return <HelpPanel />;
        if (item.content === '__SESSIONS__' && item.meta?.input) {
          return <SessionsTable sessions={item.meta.input as Session[]} />;
        }
        return <InfoMessage text={item.content} />;
      case 'completion':
        return <CompletionMessage durationMs={(item.meta?.durationMs as number) || 0} />;
      default:
        return null;
    }
  };

  return (
    <Box flexDirection="column">
      <Static items={history}>
        {(item) => <Box key={item.id}>{renderHistoryItem(item)}</Box>}
      </Static>

      {streamingText && <AssistantMessage text={streamingText} streaming />}

      {confirmState && (
        <Box flexDirection="column" marginTop={1}>
          <Text color={colors.warning}>
            {icons.warning} {confirmState.tool}
          </Text>
          <ConfirmPrompt message="Allow?" onConfirm={handleConfirm} />
        </Box>
      )}

      {showModelSelector && (
        <Box marginTop={1}>
          <ModelSelector
            currentModel={currentModel}
            onSelect={handleModelSelect}
            onCancel={handleModelCancel}
            listModels={() => agent.listModels()}
          />
        </Box>
      )}

      {showProviderManager && (
        <Box marginTop={1}>
          <ProviderManager
            onClose={() => setShowProviderManager(false)}
            onProviderChange={(providerId, model) => {
              agent.setModel(model);
              setCurrentModel(model);
              addHistory({ type: 'info', content: `Switched to ${providerId}: ${model}` });
            }}
          />
        </Box>
      )}

      {!confirmState && !showModelSelector && !showProviderManager && (
        <Box flexDirection="column" marginTop={1}>
          <PromptInput
            key={inputKey}
            value={input}
            onChange={setInput}
            onSubmit={handleSubmit}
          />
          {showCmdSuggestions && cmdSuggestions.length > 0 && (
            <CommandSuggestions input={input} selectedIndex={cmdSuggestionIndex} />
          )}
        </Box>
      )}

      {isProcessing ? (
        <ProgressBar />
      ) : showCmdSuggestions && cmdSuggestions.length > 0 ? (
        <Box marginTop={1}>
          <Text color={colors.textMuted}>  Tab to complete · ↑↓ navigate</Text>
        </Box>
      ) : null}
    </Box>
  );
}
