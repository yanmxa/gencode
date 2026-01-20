/**
 * Main App Component - Compact Ink-based TUI
 * Inspired by Claude Code and Gemini CLI design patterns
 */
import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { Box, Text, useApp, useInput, Static } from 'ink';
import { Agent } from '../../core/agent/index.js';
import type { AgentConfig } from '../../core/agent/types.js';
import { formatTokens, formatCost } from '../../core/pricing/calculator.js';
import type { CostEstimate } from '../../core/pricing/types.js';
import {
  UserMessage,
  AssistantMessage,
  ToolCall,
  ToolResult,
  PendingToolCall,
  InfoMessage,
  WelcomeMessage,
  CompletionMessage,
  CommandListDisplay,
} from './Messages.js';
import { Header } from './Header.js';
import { ProgressBar } from './Spinner.js';
import { PromptInput, ConfirmPrompt } from './Input.js';
import { ModelSelector } from './ModelSelector.js';
import { ProviderManager } from './ProviderManager.js';
import { CommandSuggestions, getFilteredCommands, BUILTIN_COMMANDS } from './CommandSuggestions.js';
import {
  PermissionPrompt,
  PermissionRulesDisplay,
  PermissionAuditDisplay,
} from './PermissionPrompt.js';
import { TodoList } from './TodoList.js';
import { QuestionPrompt, AnswerDisplay } from './QuestionPrompt.js';
import { colors, icons } from './theme.js';
import { getTodos, formatAnswersForDisplay } from '../../core/tools/index.js';
import type { Question, QuestionAnswer } from '../../core/tools/types.js';
import type { ProviderName } from '../../core/providers/index.js';
import type { ApprovalAction, ApprovalSuggestion } from '../../core/permissions/types.js';
import type { Message, ToolResultContent, ToolUseContent } from '../../core/providers/types.js';
import type { SessionMetadata } from '../../core/session/types.js';
import { gatherContextFiles, buildInitPrompt, getContextSummary } from '../../core/memory/index.js';
// ModeIndicator kept for potential future use
import { PlanApproval } from './PlanApproval.js';
import type { ModeType, PlanApprovalOption, AllowedPrompt } from '../planning/types.js';
import { getPlanModeManager } from '../planning/index.js';
import { readPlanFile, parseFilesToChange } from '../planning/plan-file.js';
// Planning utilities kept for potential future use
import { getCheckpointManager } from '../../core/session/checkpointing/index.js';
import { InputHistoryManager } from '../../core/session/input/index.js';

// Types
interface HistoryItem {
  id: string;
  type: 'header' | 'welcome' | 'user' | 'assistant' | 'tool_call' | 'tool_result' | 'info' | 'completion' | 'todos' | 'commands';
  content: string;
  meta?: Record<string, unknown>;
}

interface ConfirmState {
  tool: string;
  input: Record<string, unknown>;
  suggestions: ApprovalSuggestion[];
  metadata?: Record<string, unknown>;
  resolve: (action: ApprovalAction) => void;
}

interface QuestionState {
  questions: Question[];
  resolve: (answers: QuestionAnswer[]) => void;
}

interface PlanApprovalState {
  planSummary: string;
  requestedPermissions: AllowedPrompt[];
  filesToChange: Array<{ path: string; action: 'create' | 'modify' | 'delete' }>;
  planFilePath: string;
  resolve: (option: PlanApprovalOption, customInput?: string) => void;
}

interface SettingsManager {
  save: (settings: { model?: string }) => Promise<void>;
  getCwd?: () => string;
  addPermissionRule?: (pattern: string, type: 'allow' | 'deny', level?: 'global' | 'project' | 'local') => Promise<void>;
  get?: () => { inputHistory?: { enabled?: boolean; maxSize?: number; savePath?: string; deduplicateConsecutive?: boolean } };
}

interface Session {
  id: string;
  title: string;
  updatedAt: string;
}

interface PermissionSettings {
  allow?: string[];
  deny?: string[];
}

interface HooksConfig {
  [event: string]: Array<{
    matcher?: string;
    hooks: Array<{
      type: 'command' | 'prompt';
      command?: string;
      prompt?: string;
      timeout?: number;
      statusMessage?: string;
      blocking?: boolean;
    }>;
  }>;
}

interface AppProps {
  config: AgentConfig;
  settingsManager?: SettingsManager;
  resumeLatest?: boolean;
  permissionSettings?: PermissionSettings;
  hooksConfig?: HooksConfig;
}

// ============================================================================
// Hooks
// ============================================================================
function useAgent(config: AgentConfig) {
  const [agent] = useState(() => new Agent(config));
  return agent;
}

// ============================================================================
// Utils
// ============================================================================
function genId(): string {
  return Math.random().toString(36).slice(2);
}

function formatRelativeTime(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  const hrs = Math.floor(mins / 60);
  const days = Math.floor(hrs / 24);

  if (mins < 60) return `${mins}m`;
  if (hrs < 24) return `${hrs}h`;
  return `${days}d`;
}

// ============================================================================
// Help Component
// ============================================================================
function HelpPanel() {
  const commands: [string, string][] = [
    ['/plan [desc]', 'Enter plan mode'],
    ['/normal', 'Exit to normal mode'],
    ['/accept', 'Enter auto-accept mode'],
    ['/model [name]', 'Switch model'],
    ['/provider', 'Manage providers'],
    ['/sessions', 'List sessions'],
    ['/tasks', 'List background tasks'],
    ['/resume [n]', 'Resume session'],
    ['/new', 'New session'],
    ['/save', 'Save session'],
    ['/clear', 'Clear chat'],
    ['/init', 'Generate GEN.md'],
    ['/memory', 'Show memory files'],
    ['/changes', 'List file changes'],
    ['/rewind [n|all]', 'Undo file changes'],
    ['/context', 'Show context stats'],
    ['/compact', 'Compact conversation'],
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
  return (
    <Box flexDirection="column">
      {sessions.slice(0, 6).map((s, i) => (
        <Text key={s.id}>
          <Text color={colors.textMuted}>{String(i + 1).padEnd(2)}</Text>
          <Text color={colors.primary}>{s.id.slice(0, 7).padEnd(8)}</Text>
          <Text>{s.title.slice(0, 25).padEnd(26)}</Text>
          <Text color={colors.textMuted}>{formatRelativeTime(s.updatedAt)}</Text>
        </Text>
      ))}
    </Box>
  );
}

// ============================================================================
// Tasks Table Component
// ============================================================================
interface TasksTableProps {
  tasks: Array<{
    id: string;
    description: string;
    status: 'pending' | 'running' | 'completed' | 'error' | 'cancelled';
    startedAt: Date | string;
    completedAt?: Date | string;
    durationMs?: number;
  }>;
}

function TasksTable({ tasks }: TasksTableProps) {
  const getStatusDisplay = (status: string): { icon: string; color: string; label: string } => {
    switch (status) {
      case 'running':
        return { icon: '●', color: colors.info, label: 'Running' };
      case 'pending':
        return { icon: '○', color: colors.textMuted, label: 'Pending' };
      case 'completed':
        return { icon: '✔', color: colors.success, label: 'Done' };
      case 'error':
        return { icon: '✖', color: colors.error, label: 'Failed' };
      case 'cancelled':
        return { icon: '⊘', color: colors.warning, label: 'Stopped' };
      default:
        return { icon: '·', color: colors.textMuted, label: 'Unknown' };
    }
  };

  const formatElapsedTime = (task: TasksTableProps['tasks'][0]): string => {
    const startTime = typeof task.startedAt === 'string'
      ? new Date(task.startedAt).getTime()
      : task.startedAt.getTime();

    if (task.durationMs !== undefined) {
      // Task completed, show duration
      const seconds = Math.floor(task.durationMs / 1000);
      if (seconds < 60) return `${seconds}s`;
      const minutes = Math.floor(seconds / 60);
      const secs = seconds % 60;
      return `${minutes}m ${secs}s`;
    } else {
      // Task running, show elapsed time
      const elapsed = Date.now() - startTime;
      const seconds = Math.floor(elapsed / 1000);
      if (seconds < 60) return `${seconds}s`;
      const minutes = Math.floor(seconds / 60);
      const secs = seconds % 60;
      return `${minutes}m ${secs}s`;
    }
  };

  const getTypeLabel = (task: TasksTableProps['tasks'][0]): string => {
    // Extract task type from description
    if (task.description.toLowerCase().includes('bash:')) return 'bash';
    if (task.description.toLowerCase().includes('test')) return 'test';
    if (task.description.toLowerCase().includes('build')) return 'build';
    return 'task';
  };

  return (
    <Box flexDirection="column">
      {/* Header */}
      <Box marginBottom={1}>
        <Text>
          <Text color={colors.textMuted}>{'STATUS'.padEnd(9)}</Text>
          <Text color={colors.textMuted}>{'ID'.padEnd(9)}</Text>
          <Text color={colors.textMuted}>{'DESCRIPTION'.padEnd(33)}</Text>
          <Text color={colors.textMuted}>{'TIME'.padEnd(10)}</Text>
          <Text color={colors.textMuted}>TYPE</Text>
        </Text>
      </Box>

      {/* Tasks */}
      {tasks.slice(0, 10).map((task, index) => {
        const status = getStatusDisplay(task.status);
        const typeLabel = getTypeLabel(task);

        return (
          <Box key={task.id} marginBottom={index < tasks.length - 1 ? 0 : 0}>
            <Text>
              <Text color={status.color}>{status.icon} </Text>
              <Text color={status.color}>{status.label.padEnd(7)}</Text>
              <Text color={colors.primary} dimColor>
                {task.id.slice(0, 8).padEnd(9)}
              </Text>
              <Text>{task.description.slice(0, 32).padEnd(33)}</Text>
              <Text color={colors.textSecondary}>
                {formatElapsedTime(task).padEnd(10)}
              </Text>
              <Text color={colors.textMuted} dimColor>
                {typeLabel}
              </Text>
            </Text>
          </Box>
        );
      })}

      {/* Footer */}
      {tasks.length > 10 && (
        <Box marginTop={1}>
          <Text color={colors.textMuted}>
            ... and {tasks.length - 10} more task{tasks.length - 10 !== 1 ? 's' : ''}
          </Text>
        </Box>
      )}

      {tasks.length === 0 && (
        <Text color={colors.textMuted}>No tasks found</Text>
      )}
    </Box>
  );
}

// ============================================================================
// Memory Files Display Component
// ============================================================================
interface MemoryFileInfo {
  path: string;
  level: string;
  size: number;
  type: 'file' | 'rule';
}

function MemoryFilesDisplay({ files }: { files: MemoryFileInfo[] }) {
  const formatSize = (bytes: number): string => {
    if (bytes < 1024) return `${bytes}B`;
    return `${(bytes / 1024).toFixed(1)}KB`;
  };

  const memoryFiles = files.filter((f) => f.type === 'file');
  const ruleFiles = files.filter((f) => f.type === 'rule');

  return (
    <Box flexDirection="column">
      {memoryFiles.length > 0 && (
        <>
          <Text color={colors.info}>Loaded Memory Files:</Text>
          {memoryFiles.map((f, i) => (
            <Text key={f.path}>
              <Text color={colors.textMuted}>  [{i + 1}] </Text>
              <Text color={colors.primary}>{f.path} </Text>
              <Text color={colors.textMuted}>({f.level}, {formatSize(f.size)})</Text>
            </Text>
          ))}
        </>
      )}
      {ruleFiles.length > 0 && (
        <Box flexDirection="column" marginTop={memoryFiles.length > 0 ? 1 : 0}>
          <Text color={colors.info}>
            Loaded Rules:
          </Text>
          {ruleFiles.map((f, i) => (
            <Text key={f.path}>
              <Text color={colors.textMuted}>  [{i + 1}] </Text>
              <Text color={colors.warning}>{f.path} </Text>
              <Text color={colors.textMuted}>({f.level}, {formatSize(f.size)})</Text>
            </Text>
          ))}
        </Box>
      )}
      {files.length === 0 && (
        <Text color={colors.textMuted}>No memory files loaded</Text>
      )}
    </Box>
  );
}

// ============================================================================
// Token Estimation Utilities
// ============================================================================

/**
 * Language-aware token estimation for streaming text
 * Provides better estimates for multilingual content than the simple 4:1 ratio
 *
 * @param text - Text chunk to estimate tokens for
 * @returns Estimated token count
 */
function estimateTokenDelta(text: string): number {
  // ASCII/English: ~4 chars per token
  const asciiChars = (text.match(/[\x00-\x7F]/g) || []).length;

  // CJK (Chinese, Japanese, Korean): ~1.5 chars per token
  const cjkChars = (text.match(/[\u4E00-\u9FFF\u3040-\u309F\u30A0-\u30FF]/g) || []).length;

  // Other Unicode (including emojis): ~3 chars per token
  const otherChars = text.length - asciiChars - cjkChars;

  return Math.max(1, Math.ceil(asciiChars / 4 + cjkChars / 1.5 + otherChars / 3));
}

// ============================================================================
// Main App
// ============================================================================
export function App({ config, settingsManager, resumeLatest, permissionSettings, hooksConfig }: AppProps) {
  const { exit } = useApp();
  const agent = useAgent(config);

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

  // Performance optimization: Throttle streaming text updates
  const streamBufferRef = useRef(''); // Buffer for accumulated text
  const lastFlushTimeRef = useRef(0); // Last time we flushed to UI
  const flushTimerRef = useRef<NodeJS.Timeout | null>(null); // Pending flush timer
  const FLUSH_INTERVAL_MS = 16; // ~60 FPS throttling

  const [messageQueue, setMessageQueue] = useState<string[]>([]);
  const [processingStartTime, setProcessingStartTime] = useState<number | undefined>(undefined);
  const [tokenCount, setTokenCount] = useState(0);
  const [confirmState, setConfirmState] = useState<ConfirmState | null>(null);
  const [questionState, setQuestionState] = useState<QuestionState | null>(null);
  const [showModelSelector, setShowModelSelector] = useState(false);
  const [showProviderManager, setShowProviderManager] = useState(false);
  const [currentModel, setCurrentModel] = useState(config.model);
  const [currentProvider, setCurrentProvider] = useState(config.provider);
  const [cmdSuggestionIndex, setCmdSuggestionIndex] = useState(0);
  const [inputKey, setInputKey] = useState(0); // Force cursor to end after autocomplete
  const [pendingTool, setPendingTool] = useState<{ name: string; input: Record<string, unknown> } | null>(null);
  const pendingToolRef = useRef<{ name: string; input: Record<string, unknown> } | null>(null);
  const [todos, setTodos] = useState<ReturnType<typeof getTodos>>([]);
  const [expandedToolResults, setExpandedToolResults] = useState<Set<string>>(new Set());

  // Custom commands for autocomplete
  const [customCommands, setCustomCommands] = useState<Array<{ name: string; description: string; argumentHint?: string }>>([]);

  // Input history management
  const historyManagerRef = useRef<InputHistoryManager | null>(null);
  const [historyTempInput, setHistoryTempInput] = useState(''); // Store original input when navigating

  // Operating mode state (normal → plan → accept → normal)
  const [currentMode, setCurrentMode] = useState<ModeType>('normal');
  const currentModeRef = useRef<ModeType>('normal'); // Track mode for confirm callback
  const [planApprovalState, setPlanApprovalState] = useState<PlanApprovalState | null>(null);

  // Keep ref in sync with state
  useEffect(() => {
    currentModeRef.current = currentMode;
  }, [currentMode]);

  // Check if showing command suggestions
  const showCmdSuggestions = input.startsWith('/') && !isProcessing;
  const cmdSuggestions = showCmdSuggestions ? getFilteredCommands(input, customCommands) : [];

  // Find argument hint for matched command (when input matches command exactly or starts with command + space)
  const matchedCommandHint = useMemo(() => {
    if (!input.startsWith('/')) return undefined;
    const inputCmd = input.split(' ')[0]; // Get command part before any arguments
    const allCommands = [...BUILTIN_COMMANDS, ...customCommands];
    const matched = allCommands.find((cmd) => cmd.name === inputCmd);
    return matched?.argumentHint;
  }, [input, customCommands]);

  // Reset suggestion index when input changes
  useEffect(() => {
    setCmdSuggestionIndex(0);
  }, [input]);

  // Initialize input history manager
  useEffect(() => {
    const initHistory = async () => {
      const settings = settingsManager?.get?.();
      const historyConfig = settings?.inputHistory;
      const manager = new InputHistoryManager(historyConfig);
      await manager.load();
      historyManagerRef.current = manager;
    };

    initHistory();

    // Cleanup: flush history on unmount
    return () => {
      if (historyManagerRef.current) {
        void historyManagerRef.current.flush();
      }
    };
  }, [settingsManager]);

  // Add to history
  const addHistory = useCallback((item: Omit<HistoryItem, 'id'>) => {
    setHistory((prev) => [...prev, { ...item, id: genId() }]);
  }, []);

  // Track if warning has been shown (to avoid spam)
  const contextWarningShownRef = useRef(false);

  // Listen to session manager events for context warnings
  useEffect(() => {
    const sessionMgr = agent.getSessionManager();

    const handleContextWarning = (data: { usagePercent: number }) => {
      if (!contextWarningShownRef.current) {
        addHistory({
          type: 'info',
          content: `⚠️  Context usage at ${Math.round(data.usagePercent)}% - Consider using /compact`,
        });
        contextWarningShownRef.current = true;
      }
    };

    const handleAutoCompacting = (data: { strategy: string; usagePercent: number }) => {
      addHistory({
        type: 'info',
        content: `📦 Auto-compacting (${Math.round(data.usagePercent)}% usage, strategy: ${data.strategy})...`,
      });
    };

    const handleCompactionComplete = (data: { strategy: string }) => {
      addHistory({
        type: 'info',
        content: `✓ Compaction complete (${data.strategy})`,
      });
      // Reset warning flag after compaction
      contextWarningShownRef.current = false;
    };

    sessionMgr.on('context-warning', handleContextWarning);
    sessionMgr.on('auto-compacting', handleAutoCompacting);
    sessionMgr.on('compaction-complete', handleCompactionComplete);

    return () => {
      sessionMgr.off('context-warning', handleContextWarning);
      sessionMgr.off('auto-compacting', handleAutoCompacting);
      sessionMgr.off('compaction-complete', handleCompactionComplete);
    };
  }, [agent, addHistory]);

  // Listen to plan mode manager events for approval UI
  useEffect(() => {
    const planModeManager = getPlanModeManager();

    const unsubscribe = planModeManager.subscribe(async (event: { type: string; phase?: string }) => {
      // When phase changes to 'approval', show the approval UI
      if (event.type === 'phase_change' && event.phase === 'approval') {
        const planFilePath = planModeManager.getPlanFilePath();
        const requestedPermissions = planModeManager.getRequestedPermissions();

        if (!planFilePath) {
          addHistory({ type: 'info', content: 'Error: No plan file found' });
          return;
        }

        try {
          // Read plan file
          const planFile = await readPlanFile(planFilePath);
          if (!planFile) {
            addHistory({ type: 'info', content: `Error: Could not read plan file: ${planFilePath}` });
            return;
          }

          // Parse files to change
          const filesToChange = parseFilesToChange(planFile.content);

          // Extract first paragraph as summary (or first 200 chars)
          const lines = planFile.content.split('\n').filter((l: string) => l.trim() && !l.startsWith('#'));
          const planSummary = lines[0]?.slice(0, 200) || 'Implementation plan ready';

          // Show approval UI
          setPlanApprovalState({
            planSummary,
            requestedPermissions,
            filesToChange,
            planFilePath,
            resolve: (option: PlanApprovalOption, customInput?: string) => {
              handlePlanApprovalDecision(option, customInput);
            },
          });
        } catch (error) {
          addHistory({
            type: 'info',
            content: `Error reading plan: ${error instanceof Error ? error.message : String(error)}`,
          });
        }
      }
    });

    return () => {
      unsubscribe();
    };
  }, [addHistory]);

  // Flush buffered streaming text to UI (throttled to ~60 FPS)
  const flushStreamBuffer = useCallback(() => {
    if (streamBufferRef.current) {
      streamingTextRef.current = streamBufferRef.current;
      setStreamingText(streamBufferRef.current);
      lastFlushTimeRef.current = Date.now();
    }
    // Clear pending timer
    if (flushTimerRef.current) {
      clearTimeout(flushTimerRef.current);
      flushTimerRef.current = null;
    }
  }, []);

  // Add streaming text with throttling for performance
  const addStreamingText = useCallback((text: string) => {
    // Accumulate in buffer
    streamBufferRef.current += text;

    const now = Date.now();
    const timeSinceLastFlush = now - lastFlushTimeRef.current;

    if (timeSinceLastFlush >= FLUSH_INTERVAL_MS) {
      // Flush immediately if enough time has passed
      flushStreamBuffer();
    } else if (!flushTimerRef.current) {
      // Schedule a flush for the next interval
      const delay = FLUSH_INTERVAL_MS - timeSinceLastFlush;
      flushTimerRef.current = setTimeout(flushStreamBuffer, delay);
    }
    // Otherwise, wait for the scheduled flush
  }, [flushStreamBuffer, FLUSH_INTERVAL_MS]);

  // Convert Message[] to HistoryItem[] for displaying session history
  const convertMessagesToHistory = useCallback((
    messages: Message[],
    metadata?: SessionMetadata
  ): HistoryItem[] => {
    const items: HistoryItem[] = [];

    for (let i = 0; i < messages.length; i++) {
      const msg = messages[i];

      // Skip system messages (they're for the LLM, not for display)
      if (msg.role === 'system') {
        continue;
      }

      if (msg.role === 'user') {
        // User messages can be plain text or contain tool results
        if (typeof msg.content === 'string') {
          items.push({
            id: genId(),
            type: 'user',
            content: msg.content,
          });
        } else {
          // Check for tool results
          const toolResults = msg.content.filter((c) => c.type === 'tool_result');
          const textContent = msg.content.filter((c) => c.type === 'text')
            .map((c) => (c as { text: string }).text)
            .join('\n');

          if (textContent) {
            items.push({
              id: genId(),
              type: 'user',
              content: textContent,
            });
          }

          // Add tool results
          for (const result of toolResults) {
            const r = result as ToolResultContent;
            items.push({
              id: genId(),
              type: 'tool_result',
              content: r.content,
              meta: { toolUseId: r.toolUseId, isError: r.isError },
            });
          }
        }
      } else if (msg.role === 'assistant') {
        // Assistant messages can be text or contain tool calls
        if (typeof msg.content === 'string') {
          items.push({
            id: genId(),
            type: 'assistant',
            content: msg.content,
          });
        } else {
          // Separate text and tool calls
          const textContent = msg.content.filter((c) => c.type === 'text')
            .map((c) => (c as { text: string }).text)
            .join('\n');
          const toolCalls = msg.content.filter((c) => c.type === 'tool_use');

          if (textContent) {
            items.push({
              id: genId(),
              type: 'assistant',
              content: textContent,
            });
          }

          // Add tool calls
          for (const call of toolCalls) {
            const c = call as ToolUseContent;
            items.push({
              id: genId(),
              type: 'tool_call',
              content: c.name,
              meta: { id: c.id, name: c.name, input: c.input },
            });
          }
        }
      }

      // Inject completion message if this message index has a completion
      if (metadata?.completions) {
        const completion = metadata.completions.find((c) => c.afterMessageIndex === i);
        if (completion) {
          items.push({
            id: genId(),
            type: 'completion',
            content: '',
            meta: {
              durationMs: completion.durationMs,
              usage: completion.usage,
              cost: completion.cost,
            },
          });
        }
      }
    }

    return items;
  }, []);

  // Initialize
  useEffect(() => {
    const init = async () => {
      // Load custom commands for autocomplete
      try {
        const { getCommandManager } = await import('../../ext/commands/index.js');
        const cmdMgr = await getCommandManager(cwd);
        const commands = await cmdMgr.listCommands();
        setCustomCommands(
          commands
            // Only include namespaced commands (those with ':')
            .filter((cmd) => cmd.name.includes(':'))
            .map((cmd) => ({
              name: `/${cmd.name}`,
              description: cmd.description || '',
              argumentHint: cmd.argumentHint,
            }))
        );
      } catch (error) {
        // Silently fail if command loading fails
        console.error('Failed to load custom commands:', error);
      }

      // Initialize permission system with settings
      await agent.initializePermissions(permissionSettings);

      // Initialize hooks system with configuration
      if (hooksConfig) {
        agent.initializeHooks(hooksConfig);
      }

      // Set enhanced confirm callback with approval options
      agent.setEnhancedConfirmCallback(async (tool, toolInput, suggestions) => {
        // Auto-approve in accept mode
        if (currentModeRef.current === 'accept') {
          return 'allow_once';
        }

        return new Promise<ApprovalAction>((resolve) => {
          setConfirmState({
            tool,
            input: toolInput as Record<string, unknown>,
            suggestions,
            resolve,
          });
        });
      });

      // Set askUser callback for AskUserQuestion tool
      agent.setAskUserCallback(async (questions) => {
        return new Promise<QuestionAnswer[]>((resolve) => {
          setQuestionState({ questions, resolve });
        });
      });

      // Set askPermission callback for tools that need metadata-based permission (e.g., Edit with diff)
      agent.setAskPermissionCallback(async (request) => {
        // Auto-approve in accept mode
        if (currentModeRef.current === 'accept') {
          return 'allow_once';
        }

        // Get default suggestions (same as enhanced confirm)
        const suggestions: ApprovalSuggestion[] = [
          { action: 'allow_once', label: 'Yes', shortcut: '1' },
          { action: 'allow_always', label: "Yes, and don't ask again", shortcut: '2' },
          { action: 'deny', label: 'No', shortcut: '3' },
        ];

        return new Promise<ApprovalAction>((resolve) => {
          setConfirmState({
            tool: request.tool,
            input: request.input as Record<string, unknown>,
            suggestions,
            metadata: request.metadata,
            resolve,
          });
        });
      });

      // Set callback to save permission rules to settings.local.json
      if (settingsManager?.addPermissionRule) {
        agent.setSaveRuleCallback(async (tool, pattern) => {
          // Format as Claude Code style pattern: Tool(pattern) or just Tool
          const rulePattern = pattern ? `${tool}(${pattern})` : tool;
          await settingsManager.addPermissionRule!(rulePattern, 'allow', 'local');
        });
      }

      if (resumeLatest) {
        const resumed = await agent.resumeLatest();
        if (resumed) {
          // Get the restored messages and display them
          const messages = agent.getHistory();
          const session = agent.getSessionManager().getCurrent();
          const historyItems = convertMessagesToHistory(messages, session?.metadata);

          // Add all historical messages
          setHistory((prev) => [...prev, ...historyItems]);
        }
      }
    };
    init();
  }, [agent, resumeLatest, addHistory, permissionSettings, settingsManager, convertMessagesToHistory, cwd]);

  // Handle question answers (AskUserQuestion)
  const handleQuestionComplete = useCallback((answers: QuestionAnswer[]) => {
    if (questionState) {
      // Show confirmation in history
      addHistory({
        type: 'info',
        content: formatAnswersForDisplay(answers),
      });
      questionState.resolve(answers);
      setQuestionState(null);
    }
  }, [questionState, addHistory]);

  // Handle question cancel
  const handleQuestionCancel = useCallback(() => {
    if (questionState) {
      // Clear pending tool display (no more spinner)
      pendingToolRef.current = null;
      setPendingTool(null);
      // Add canceled message to history
      addHistory({ type: 'info', content: 'Question canceled' });
      questionState.resolve([]); // Return empty answers on cancel
      setQuestionState(null);
    }
  }, [questionState, addHistory]);

  // Handle permission decision
  const handlePermissionDecision = (action: ApprovalAction) => {
    if (confirmState) {
      confirmState.resolve(action);
      setConfirmState(null);
    }
  };

  // Handle plan approval decision
  const handlePlanApprovalDecision = useCallback(async (option: PlanApprovalOption, customInput?: string) => {
    // Close approval UI
    setPlanApprovalState(null);

    switch (option) {
      case 'approve_clear':
        // Option 1: Clear context + auto-accept edits
        addHistory({ type: 'info', content: 'Plan approved (clearing context, auto-accepting edits)' });
        agent.exitPlanMode(true); // Approve with permissions
        agent.clearHistory(); // Clear conversation context
        await agent.startSession(); // Start fresh session
        setHistory(initialHistory); // Clear UI history
        setCurrentMode('accept'); // Switch to auto-accept mode
        break;

      case 'approve_manual_keep':
        // Option 2: Keep context + manually approve edits (NEW)
        addHistory({ type: 'info', content: 'Plan approved (keeping context, manual approval enabled)' });
        agent.exitPlanMode(true); // Approve with permissions
        // Keep conversation history
        // Keep plan file and exploration context
        setCurrentMode('normal'); // Manual approval mode
        break;

      case 'approve':
        // Option 3: Clear plan details + auto-accept edits
        addHistory({ type: 'info', content: 'Plan approved (auto-accepting edits)' });
        agent.exitPlanMode(true); // Approve with permissions
        // Keep conversation but clear plan-specific details
        setCurrentMode('accept'); // Switch to auto-accept mode
        break;

      case 'approve_manual':
        // Option 4: Clear plan details + manually approve edits
        addHistory({ type: 'info', content: 'Plan approved (manual approval enabled)' });
        agent.exitPlanMode(true); // Approve with permissions
        // Keep conversation but clear plan-specific details
        setCurrentMode('normal'); // Manual approval mode
        break;

      case 'modify':
        // Option 5: Go back to modify the plan
        if (customInput) {
          addHistory({ type: 'info', content: `Modifying plan: ${customInput}` });
          addHistory({ type: 'user', content: customInput });
          await runAgent(customInput);
        } else {
          addHistory({ type: 'info', content: 'Please provide feedback on what to change' });
        }
        // Stay in plan mode
        break;

      case 'cancel':
        // Cancel plan mode entirely
        addHistory({ type: 'info', content: 'Plan cancelled' });
        agent.exitPlanMode(false); // Exit without approving
        setCurrentMode('normal');
        break;
    }
  }, [agent, addHistory, initialHistory]);

  // Handle model selection
  const handleModelSelect = async (model: string, providerId?: ProviderName, authMethod?: string) => {
    agent.setModel(model, providerId, authMethod);
    setCurrentModel(model);

    // Update provider state to keep UI in sync
    const newProvider = agent.getProvider();
    setCurrentProvider(newProvider);

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

      case 'tasks': {
        const { TaskManager } = await import('../../core/session/tasks/task-manager.js');
        const taskManager = new TaskManager();
        const filter = (arg as 'all' | 'running' | 'completed' | 'error') || 'all';
        const tasks = taskManager.listTasks(filter);

        if (tasks.length === 0) {
          addHistory({ type: 'info', content: 'No background tasks' });
        } else {
          addHistory({ type: 'info', content: '__TASKS__', meta: { input: tasks } });
        }
        return true;
      }

      case 'resume': {
        let success = false;

        if (!arg) {
          success = await agent.resumeLatest();
        } else {
          const index = parseInt(arg, 10);
          if (isNaN(index)) {
            success = await agent.resumeSession(arg);
          } else {
            const sessions = await agent.listSessions();
            if (index >= 1 && index <= sessions.length) {
              success = await agent.resumeSession(sessions[index - 1].id);
            }
          }
        }

        if (success) {
          // Get the restored messages and display them
          const messages = agent.getHistory();
          const session = agent.getSessionManager().getCurrent();
          const historyItems = convertMessagesToHistory(messages, session?.metadata);

          // Add all historical messages
          setHistory((prev) => [...prev, ...historyItems]);
        } else {
          addHistory({ type: 'info', content: 'Failed to restore session' });
        }
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
          setCurrentProvider(agent.getProvider());
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

      case 'permissions': {
        const permManager = agent.getPermissionManager();

        if (arg === 'audit') {
          // Show audit log
          const auditLog = permManager.getAuditLog(20);
          if (auditLog.length === 0) {
            addHistory({ type: 'info', content: 'No permission decisions recorded yet' });
          } else {
            const entries = auditLog.map((e) => ({
              time: e.timestamp.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }),
              tool: e.tool,
              input: e.inputSummary,
              decision: e.decision,
              rule: e.matchedRule,
            }));
            addHistory({
              type: 'info',
              content: '__PERMISSION_AUDIT__',
              meta: { entries },
            });
          }
        } else if (arg === 'stats') {
          // Show statistics
          const stats = permManager.getAuditStats();
          addHistory({
            type: 'info',
            content: `Permissions: ${stats.allowed + stats.confirmed} allowed, ${stats.denied + stats.rejected} denied`,
          });
        } else {
          // Show rules
          const rules = permManager.getRules();
          const prompts = permManager.getAllowedPrompts();
          const displayRules = rules.map((r) => ({
            type: r.scope === 'session' ? 'Session' : r.description?.startsWith('Settings') ? 'Settings' : 'Built-in',
            tool: typeof r.tool === 'string' ? r.tool : r.tool.toString(),
            pattern: typeof r.pattern === 'string' ? r.pattern : r.pattern?.toString(),
            scope: r.scope ?? 'session',
            mode: r.mode,
          }));
          addHistory({
            type: 'info',
            content: '__PERMISSIONS__',
            meta: { rules: displayRules, prompts },
          });
        }
        return true;
      }

      case 'init': {
        // Gather context files and generate GEN.md
        addHistory({ type: 'info', content: 'Analyzing codebase...' });

        const contextFiles = await gatherContextFiles(cwd);
        addHistory({ type: 'info', content: getContextSummary(contextFiles) });

        // Check if GEN.md already exists
        const memoryManager = agent.getMemoryManager();
        const existingPath = await memoryManager.getExistingProjectMemoryPath(cwd);
        let existingContent: string | undefined;

        if (existingPath) {
          try {
            const fs = await import('fs/promises');
            existingContent = await fs.readFile(existingPath, 'utf-8');
            addHistory({
              type: 'info',
              content: `Found existing: ${existingPath.replace(cwd, '.')}`,
            });
          } catch {
            // File doesn't exist or can't be read
          }
        }

        // Build init prompt and run through agent
        const initPrompt = buildInitPrompt(contextFiles, existingContent);
        addHistory({ type: 'info', content: 'Generating GEN.md...' });
        addHistory({ type: 'user', content: '/init' });
        await runAgent(initPrompt);
        return true;
      }

      case 'memory': {
        // Show loaded memory files
        const memoryManager = agent.getMemoryManager();
        const loadedFiles = memoryManager.getLoadedFileList();

        if (loadedFiles.length === 0) {
          // Try to load memory first
          await agent.loadMemory();
          const filesAfterLoad = memoryManager.getLoadedFileList();
          if (filesAfterLoad.length === 0) {
            addHistory({ type: 'info', content: 'No memory files found' });
          } else {
            addHistory({
              type: 'info',
              content: '__MEMORY__',
              meta: { files: filesAfterLoad },
            });
          }
        } else {
          addHistory({
            type: 'info',
            content: '__MEMORY__',
            meta: { files: loadedFiles },
          });
        }
        return true;
      }

      case 'plan': {
        // Enter plan mode
        await agent.enterPlanMode(arg);
        setCurrentMode('plan');
        return true;
      }

      case 'normal': {
        // Exit to normal mode
        if (agent.isPlanModeActive()) {
          agent.exitPlanMode(false);
        }
        setCurrentMode('normal');
        return true;
      }

      case 'accept': {
        // Enter auto-accept mode
        if (agent.isPlanModeActive()) {
          agent.exitPlanMode(false);
        }
        setCurrentMode('accept');
        return true;
      }

      case 'changes': {
        // List file changes (checkpoints)
        const checkpointManager = getCheckpointManager();
        if (!checkpointManager.hasCheckpoints()) {
          addHistory({ type: 'info', content: '\nNo file changes in this session' });
        } else {
          addHistory({ type: 'info', content: '\n' + checkpointManager.formatCheckpointList(false) });
        }
        return true;
      }

      case 'rewind': {
        // Rewind file changes
        const checkpointManager = getCheckpointManager();

        if (!checkpointManager.hasCheckpoints()) {
          addHistory({ type: 'info', content: 'No file changes to rewind' });
          return true;
        }

        if (arg === 'all') {
          // Rewind all changes
          const result = await checkpointManager.rewind({ all: true });

          // Build output message showing both successes and failures
          const messages: string[] = [''];  // Start with empty line for spacing

          if (result.revertedFiles.length > 0) {
            const files = result.revertedFiles.map((f) => {
              const fileName = f.path.split('/').pop() || f.path;
              return `  • ${fileName} (${f.action})`;
            }).join('\n');
            messages.push(`Reverted ${result.revertedFiles.length} file(s):\n${files}`);
          }

          if (result.errors.length > 0) {
            const errors = result.errors.map((e) => {
              const fileName = e.path.split('/').pop() || e.path;
              return `  • ${fileName}: ${e.error}`;
            }).join('\n');
            messages.push(`\nFailed to revert ${result.errors.length} file(s):\n${errors}`);
          }

          if (messages.length > 1) {
            addHistory({ type: 'info', content: messages.join('\n') });
          } else {
            addHistory({ type: 'info', content: '\nNo changes to rewind' });
          }
        } else if (arg) {
          // Rewind specific checkpoint by index
          const index = parseInt(arg, 10);
          if (!isNaN(index) && index >= 1) {
            const checkpoints = checkpointManager.getCheckpoints();
            if (index <= checkpoints.length) {
              const checkpoint = checkpoints[index - 1];
              const result = await checkpointManager.rewind({ checkpointId: checkpoint.id });
              if (result.success && result.revertedFiles.length > 0) {
                const f = result.revertedFiles[0];
                const fileName = f.path.split('/').pop() || f.path;
                addHistory({ type: 'info', content: `\nReverted: ${fileName} (${f.action})` });
              } else if (result.errors.length > 0) {
                const fileName = result.errors[0].path.split('/').pop() || result.errors[0].path;
                addHistory({ type: 'info', content: `\nFailed: ${fileName} - ${result.errors[0].error}` });
              } else {
                addHistory({ type: 'info', content: '\nFailed to rewind change' });
              }
            } else {
              addHistory({ type: 'info', content: '\nInvalid index: ${index}' });
            }
          } else {
            addHistory({ type: 'info', content: '\nUsage: /rewind [n|all]' });
          }
        } else {
          // Show changes and usage in one message
          addHistory({ type: 'info', content: '\n' + checkpointManager.formatCheckpointList(true) });
        }
        return true;
      }

      case 'compact': {
        // Manually trigger conversation compaction
        const sessionMgr = agent.getSessionManager();
        const current = sessionMgr.getCurrent();

        if (!current || current.messages.length === 0) {
          addHistory({ type: 'info', content: 'No messages to compact' });
          return true;
        }

        // Perform manual compaction
        const modelInfo = agent.getModelInfo();
        if (modelInfo) {
          try {
            await sessionMgr.performCompaction(modelInfo);

            const stats = sessionMgr.getCompressionStats();
            const saved = stats?.totalMessages ? stats.totalMessages - stats.activeMessages : 0;
            const savedPercent = stats?.totalMessages
              ? ((saved / stats.totalMessages) * 100).toFixed(0)
              : '0';

            // Create visual bars (ASCII only)
            const barWidth = 15;
            const activeBar = stats?.totalMessages
              ? Math.round((stats.activeMessages / stats.totalMessages) * barWidth)
              : barWidth;
            const totalBar = barWidth;

            const activeVisual = '#'.repeat(activeBar) + '.'.repeat(barWidth - activeBar);
            const totalVisual = '#'.repeat(totalBar);

            // Format with simple ASCII box
            const w = 50;
            const pad = (text: string) => text + ' '.repeat(Math.max(0, w - text.length - 3));

            const lines = [
              '+' + '-'.repeat(w - 2) + '+',
              '| ' + pad('Compaction Complete') + '|',
              '+' + '-'.repeat(w - 2) + '+',
              '| ' + pad(`Active Messages    ${String(stats?.activeMessages || 0).padStart(3)}  [${activeVisual}]`) + '|',
              '| ' + pad(`Total Messages     ${String(stats?.totalMessages || 0).padStart(3)}  [${totalVisual}]`) + '|',
              '| ' + pad(`Summaries          ${String(stats?.summaryCount || 0).padStart(3)}`) + '|',
              '| ' + pad('') + '|',
              '| ' + pad(`Saved: ${savedPercent}%`) + '|',
              '+' + '-'.repeat(w - 2) + '+',
            ];

            addHistory({
              type: 'info',
              content: '\n' + lines.join('\n'),
            });
          } catch (error) {
            addHistory({
              type: 'info',
              content: `Compaction failed: ${error instanceof Error ? error.message : String(error)}`,
            });
          }
        } else {
          addHistory({ type: 'info', content: 'Model information not available' });
        }
        return true;
      }

      case 'context': {
        // Show context usage statistics
        const sessionMgr = agent.getSessionManager();
        const stats = sessionMgr.getCompressionStats();

        if (!stats) {
          addHistory({ type: 'info', content: 'No compression statistics available' });
          return true;
        }

        const activeRatio = stats.totalMessages > 0
          ? ((stats.activeMessages / stats.totalMessages) * 100).toFixed(0)
          : '100';

        const isCompressed = stats.activeMessages < stats.totalMessages;

        // Create progress bar (ASCII only)
        const barWidth = 20;
        const filledWidth = stats.totalMessages > 0
          ? Math.round((stats.activeMessages / stats.totalMessages) * barWidth)
          : barWidth;
        const progressBar = '#'.repeat(filledWidth) + '.'.repeat(barWidth - filledWidth);

        const statusText = isCompressed ? 'Compressed' : 'Uncompressed';
        const statusColor = isCompressed ? '\x1b[32m' : '\x1b[90m';

        // Format with simple ASCII box
        const w = 50;
        const visibleLength = (text: string) => text.replace(/\x1b\[[0-9;]*m/g, '').length;
        const pad = (text: string) => {
          const visible = visibleLength(text);
          return text + ' '.repeat(Math.max(0, w - visible - 3));
        };

        const statusLine = `Status: ${statusColor}${statusText}\x1b[0m`;

        const lines = [
          '+' + '-'.repeat(w - 2) + '+',
          '| ' + pad('Context Usage Statistics') + '|',
          '+' + '-'.repeat(w - 2) + '+',
          '| ' + pad(`Active Messages     ${String(stats.activeMessages).padStart(3)}`) + '|',
          '| ' + pad(`Total Messages      ${String(stats.totalMessages).padStart(3)}`) + '|',
          '| ' + pad(`Summaries           ${String(stats.summaryCount).padStart(3)}`) + '|',
          '| ' + pad('') + '|',
          '| ' + pad(`Usage  [${progressBar}] ${activeRatio.padStart(3)}%`) + '|',
          '| ' + pad('') + '|',
          '| ' + pad(statusLine) + '|',
          '+' + '-'.repeat(w - 2) + '+',
        ];

        addHistory({
          type: 'info',
          content: '\n' + lines.join('\n'),
        });
        return true;
      }

      case 'commands':
      case 'cmd': {
        const { getCommandManager } = await import('../../ext/commands/index.js');
        const cmdMgr = await getCommandManager(cwd);
        const commands = await cmdMgr.listCommands();

        if (commands.length === 0) {
          addHistory({ type: 'info', content: 'No custom commands found' });
        } else {
          addHistory({ type: 'info', content: '__COMMANDS__', meta: { commands } });
        }
        return true;
      }

      default: {
        // Check for custom commands
        const debugEnabled = process.env.GEN_DEBUG?.includes('commands');
        if (debugEnabled) {
          console.error(`[debug:commands] Checking custom command: /${command}`);
          console.error(`[debug:commands] CWD: ${cwd}`);
        }

        const { getCommandManager } = await import('../../ext/commands/index.js');
        const cmdMgr = await getCommandManager(cwd);

        if (debugEnabled) {
          console.error(`[debug:commands] Command manager initialized for: ${cwd}`);
        }

        if (await cmdMgr.hasCommand(command)) {
          const argString = parts.slice(1).join(' ');
          const parsed = await cmdMgr.parseCommand(command, argString);

          if (parsed) {
            // Apply pre-authorized tools
            if (parsed.preAuthorizedTools.length > 0) {
              const permManager = agent.getPermissionManager();

              // Convert tool names/patterns to PromptPermissions
              const prompts = parsed.preAuthorizedTools.map((toolSpec: string) => {
                // Check if it's a pattern like "Bash(gh:*)"
                const patternMatch = toolSpec.match(/^(\w+)\(([^)]+)\)$/);
                if (patternMatch) {
                  const [, tool, pattern] = patternMatch;
                  return { tool, prompt: pattern };
                } else {
                  // Simple tool name like "Read" or "Write"
                  return { tool: toolSpec, prompt: `use ${toolSpec} tool` };
                }
              });

              permManager.addAllowedPrompts(prompts);
            }

            // Apply model override
            if (parsed.modelOverride) {
              agent.setModel(parsed.modelOverride);
              setCurrentModel(parsed.modelOverride);
              setCurrentProvider(agent.getProvider());
            }

            // Add command to history
            addHistory({ type: 'user', content: `/${command} ${argString}`.trim() });

            // Execute expanded prompt
            await runAgent(parsed.expandedPrompt);
            return true;
          }
        }

        return false;
      }
    }
  };

  // Interrupt ref for ESC handling
  const interruptFlagRef = useRef(false);
  // AbortController for cancellation support
  const abortControllerRef = useRef<AbortController | null>(null);

  // Run agent
  const runAgent = async (prompt: string) => {
    setIsProcessing(true);
    setIsThinking(true);
    setStreamingText('');
    streamingTextRef.current = '';
    // Clear streaming buffer and any pending flush timers
    streamBufferRef.current = '';
    if (flushTimerRef.current) {
      clearTimeout(flushTimerRef.current);
      flushTimerRef.current = null;
    }
    interruptFlagRef.current = false;

    // Create AbortController for this run
    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    const startTime = Date.now();
    setProcessingStartTime(startTime);
    setTokenCount(0);

    try {
      for await (const event of agent.run(prompt, abortController.signal)) {
        // Check for interrupt
        if (interruptFlagRef.current || abortController.signal.aborted) {
          break;
        }

        switch (event.type) {
          case 'text':
            setIsThinking(false);
            // Use throttled streaming text update for better performance
            addStreamingText(event.text);
            // Estimate token count with language-aware estimation
            setTokenCount((prev) => prev + estimateTokenDelta(event.text));
            break;

          case 'tool_start':
            setIsThinking(false);
            if (streamingTextRef.current) {
              addHistory({ type: 'assistant', content: streamingTextRef.current });
              streamingTextRef.current = '';
              setStreamingText('');
            }
            // Set pending tool for spinner animation (use both state and ref)
            const toolInfo = { name: event.name, input: event.input as Record<string, unknown> };
            pendingToolRef.current = toolInfo;
            setPendingTool(toolInfo);
            break;

          case 'tool_result':
            // For TodoWrite: add todos first, then hide tool_call/tool_result
            if (event.name === 'TodoWrite') {
              const currentTodos = getTodos();
              setTodos(currentTodos);
              if (currentTodos.length > 0) {
                addHistory({
                  type: 'todos',
                  content: '',
                  meta: { todos: currentTodos },
                });
              }
            } else {
              // Add tool_call to history (now completed) - use ref for correct value
              if (pendingToolRef.current) {
                addHistory({
                  type: 'tool_call',
                  content: pendingToolRef.current.name,
                  meta: { toolName: pendingToolRef.current.name, input: pendingToolRef.current.input },
                });
              }
              // Add tool_result to history
              addHistory({
                type: 'tool_result',
                content: event.result.output,
                meta: {
                  toolName: event.name,
                  success: event.result.success,
                  error: event.result.error,
                  metadata: event.result.metadata,
                },
              });
            }
            pendingToolRef.current = null;
            setPendingTool(null);
            setIsThinking(true);
            break;

          case 'reasoning_delta':
            // Display reasoning content from o1/o3/Gemini 3+ models
            setIsThinking(false);
            addHistory({
              type: 'info',
              content: `💭 Reasoning: ${event.text}`,
            });
            break;

          case 'tool_input_delta':
            // Progressive display of tool input JSON (optional enhancement)
            // For now, we just accumulate and display when complete
            // Could be enhanced to show partial JSON in real-time
            break;

          case 'error':
            setIsThinking(false);
            addHistory({ type: 'info', content: `Error: ${event.error.message}` });
            break;

          case 'done':
            // Flush any remaining buffered text immediately
            flushStreamBuffer();

            if (streamingTextRef.current) {
              addHistory({ type: 'assistant', content: streamingTextRef.current });
              streamingTextRef.current = '';
              streamBufferRef.current = '';
              setStreamingText('');
            }
            // Use real token count from usage if available (overrides estimate)
            if (event.usage) {
              setTokenCount(event.usage.outputTokens);
            }
            // Add completion message with duration and cost info
            const durationMs = Date.now() - startTime;
            addHistory({
              type: 'completion',
              content: '',
              meta: { durationMs, usage: event.usage, cost: event.cost },
            });
            setProcessingStartTime(undefined);
            break;
        }
      }
    } catch (error) {
      addHistory({
        type: 'info',
        content: `Error: ${error instanceof Error ? error.message : String(error)}`,
      });
    } finally {
      // Clean up AbortController
      abortControllerRef.current = null;
    }

    setIsProcessing(false);
    setIsThinking(false);

    // Process next message in queue if any
    setMessageQueue((queue) => {
      if (queue.length > 0) {
        const [nextMessage, ...rest] = queue;
        // Schedule next message processing
        setTimeout(() => {
          addHistory({ type: 'user', content: nextMessage });
          runAgent(nextMessage);
        }, 0);
        return rest;
      }
      return queue;
    });
  };

  // Handle submit
  const handleSubmit = async (text: string) => {
    const trimmed = text.trim();
    if (!trimmed) return;

    // Add to input history
    if (historyManagerRef.current) {
      historyManagerRef.current.add(trimmed);
      historyManagerRef.current.reset(); // Reset navigation state
    }

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

    // Handle # prefix for quick memory adds
    // ## note -> user memory (~/.gen/GEN.md)
    // # note -> project memory (./GEN.md)
    if (trimmed.startsWith('#') && !trimmed.startsWith('#!/')) {
      const memoryManager = agent.getMemoryManager();
      let level: 'user' | 'project';
      let content: string;

      if (trimmed.startsWith('## ')) {
        level = 'user';
        content = trimmed.slice(3).trim();
      } else if (trimmed.startsWith('# ')) {
        level = 'project';
        content = trimmed.slice(2).trim();
      } else {
        // Just # with no space, treat as project
        level = 'project';
        content = trimmed.slice(1).trim();
      }

      if (!content) {
        addHistory({ type: 'info', content: 'Empty memory entry ignored' });
        return;
      }

      try {
        const savedPath = await memoryManager.quickAdd(content, level, cwd);
        const displayPath = savedPath.replace(process.env.HOME || '', '~');
        addHistory({
          type: 'info',
          content: `Added to ${level} memory: ${displayPath}`,
        });
      } catch (error) {
        addHistory({
          type: 'info',
          content: `Failed to add to memory: ${error instanceof Error ? error.message : String(error)}`,
        });
      }
      return;
    }

    if (trimmed.startsWith('/')) {
      const handled = await handleCommand(trimmed);
      if (!handled) {
        addHistory({ type: 'info', content: `Unknown: ${trimmed}` });
      }
      return;
    }

    // Queue message if already processing
    if (isProcessing) {
      setMessageQueue((queue) => [...queue, trimmed]);
      // Don't add history item - we'll show queue count in the UI
      return;
    }

    addHistory({ type: 'user', content: trimmed });
    await runAgent(trimmed);
  };

  // Keyboard shortcuts
  useInput((inputChar, key) => {
    // ESC to interrupt processing or cancel history navigation
    if (key.escape) {
      if (isProcessing) {
        // Abort the operation
        if (abortControllerRef.current) {
          abortControllerRef.current.abort();
        }
        interruptFlagRef.current = true;
        setIsProcessing(false);
        setStreamingText('');
        streamingTextRef.current = '';
        streamBufferRef.current = '';
        // Clear pending tool (stop spinner)
        pendingToolRef.current = null;
        setPendingTool(null);
        // Clean up incomplete tool_use messages to prevent API errors
        agent.cleanupIncompleteMessages();
        addHistory({ type: 'info', content: 'Interrupted' });
      } else if (historyManagerRef.current?.isNavigating()) {
        // Cancel history navigation - restore original input
        historyManagerRef.current.reset();
        setInput(historyTempInput);
        setHistoryTempInput('');
      }
    }

    // Shift+Tab to cycle modes: normal → plan → accept → normal
    if (key.shift && key.tab && !isProcessing && !confirmState && !questionState && !planApprovalState) {
      const cycleMode = async () => {
        const nextMode: Record<ModeType, ModeType> = {
          normal: 'plan',
          plan: 'accept',
          accept: 'normal',
        };
        const newMode = nextMode[currentMode];

        // Handle plan mode transitions
        if (newMode === 'plan') {
          await agent.enterPlanMode();
        } else if (currentMode === 'plan') {
          agent.exitPlanMode(false);
        }

        setCurrentMode(newMode);
      };
      cycleMode();
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
    // Input history navigation (when NOT showing command suggestions)
    else if (!isProcessing && !confirmState && !questionState && !planApprovalState && historyManagerRef.current) {
      if (key.upArrow) {
        // Save current input on first navigation
        if (!historyManagerRef.current.isNavigating()) {
          setHistoryTempInput(input);
        }

        const prevEntry = historyManagerRef.current.previous();
        if (prevEntry !== null) {
          setInput(prevEntry);
          setInputKey((k) => k + 1); // Force cursor to end
        }
      } else if (key.downArrow && historyManagerRef.current.isNavigating()) {
        const nextEntry = historyManagerRef.current.next();
        if (nextEntry === null) {
          // Reached end - restore original input
          setInput(historyTempInput);
          setHistoryTempInput('');
        } else {
          setInput(nextEntry);
        }
        setInputKey((k) => k + 1); // Force cursor to end
      }
    }
  });

  // Render history item
  const renderHistoryItem = (item: HistoryItem) => {
    switch (item.type) {
      case 'header': {
        // Calculate context stats for header
        const sessionMgr = agent.getSessionManager();
        const compressionStats = sessionMgr.getCompressionStats();
        const tokenUsage = sessionMgr.getTokenUsage();
        const modelInfo = agent.getModelInfo();

        const contextStats = compressionStats && modelInfo && compressionStats.activeMessages > 0 ? {
          activeMessages: compressionStats.activeMessages,
          totalMessages: compressionStats.totalMessages,
          usagePercent: (tokenUsage.total / modelInfo.contextWindow) * 100,
        } : undefined;

        return (
          <Header
            provider={(item.meta?.provider as string) || ''}
            model={(item.meta?.model as string) || ''}
            cwd={(item.meta?.cwd as string) || ''}
            contextStats={contextStats}
          />
        );
      }
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
            error={(item.meta?.error as string) || undefined}
            metadata={item.meta?.metadata as Record<string, unknown> | undefined}
            expanded={expandedToolResults.has(item.id)}
            id={item.id}
          />
        );
      case 'info':
        if (item.content === '__HELP__') return <HelpPanel />;
        if (item.content === '__SESSIONS__' && item.meta?.input) {
          return <SessionsTable sessions={item.meta.input as Session[]} />;
        }
        if (item.content === '__TASKS__' && item.meta?.input) {
          return <TasksTable tasks={item.meta.input as any[]} />;
        }
        if (item.content === '__PERMISSIONS__' && item.meta?.rules) {
          return (
            <PermissionRulesDisplay
              rules={item.meta.rules as { type: string; tool: string; pattern?: string; scope: string; mode: string }[]}
              allowedPrompts={item.meta.prompts as { tool: string; prompt: string }[] | undefined}
            />
          );
        }
        if (item.content === '__PERMISSION_AUDIT__' && item.meta?.entries) {
          return (
            <PermissionAuditDisplay
              entries={item.meta.entries as { time: string; tool: string; input: string; decision: string; rule?: string }[]}
            />
          );
        }
        if (item.content === '__MEMORY__' && item.meta?.files) {
          return <MemoryFilesDisplay files={item.meta.files as MemoryFileInfo[]} />;
        }
        if (item.content === '__COMMANDS__' && item.meta?.commands) {
          return <CommandListDisplay commands={item.meta.commands as any[]} />;
        }
        // Check if content is a formatted box (starts with box border)
        if (item.content.trim().startsWith('+---')) {
          return (
            <Box marginTop={1}>
              <Text color={colors.textSecondary}>{item.content}</Text>
            </Box>
          );
        }
        return <InfoMessage text={item.content} />;
      case 'completion':
        return (
          <CompletionMessage
            durationMs={(item.meta?.durationMs as number) || 0}
            usage={item.meta?.usage as { inputTokens: number; outputTokens: number } | undefined}
            cost={item.meta?.cost as CostEstimate | undefined}
          />
        );
      case 'todos':
        return <TodoList todos={item.meta?.todos as ReturnType<typeof getTodos>} />;
      default:
        return null;
    }
  };

  return (
    <Box flexDirection="column" paddingBottom={2}>
      <Static items={history}>
        {(item) => <Box key={item.id}>{renderHistoryItem(item)}</Box>}
      </Static>

      {pendingTool && !confirmState && <PendingToolCall name={pendingTool.name} input={pendingTool.input} />}

      {streamingText && <AssistantMessage text={streamingText} streaming />}

      {confirmState && (
        <PermissionPrompt
          tool={confirmState.tool}
          input={confirmState.input}
          suggestions={confirmState.suggestions}
          onDecision={handlePermissionDecision}
          projectPath={settingsManager?.getCwd?.() ?? process.cwd()}
          metadata={confirmState.metadata}
        />
      )}

      {questionState && (
        <QuestionPrompt
          questions={questionState.questions}
          onComplete={handleQuestionComplete}
          onCancel={handleQuestionCancel}
        />
      )}

      {/* Plan approval UI */}
      {planApprovalState && (
        <Box marginTop={1}>
          <PlanApproval
            planSummary={planApprovalState.planSummary}
            requestedPermissions={planApprovalState.requestedPermissions}
            filesToChange={planApprovalState.filesToChange}
            planFilePath={planApprovalState.planFilePath}
            onDecision={(option, customInput) => {
              planApprovalState.resolve(option, customInput);
              setPlanApprovalState(null);
            }}
          />
        </Box>
      )}

      {showModelSelector && (
        <Box marginTop={1}>
          <ModelSelector
            currentModel={currentModel}
            currentProvider={currentProvider}
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
              setCurrentProvider(agent.getProvider());
              addHistory({ type: 'info', content: `Switched to ${providerId}: ${model}` });
            }}
          />
        </Box>
      )}

      {!confirmState && !questionState && !showModelSelector && !showProviderManager && (
        <Box flexDirection="column" marginTop={2}>
          {/* Queue display above input */}
          {messageQueue.length > 0 && (
            <Box flexDirection="column" marginBottom={1}>
              {messageQueue.map((msg, i) => (
                <Text key={i} color={colors.textMuted}>
                  ⏳ <Text color={colors.text}>{msg.length > 60 ? msg.slice(0, 60) + '...' : msg}</Text>
                </Text>
              ))}
            </Box>
          )}

          <PromptInput
            key={inputKey}
            value={input}
            onChange={setInput}
            onSubmit={handleSubmit}
            hint={matchedCommandHint}
          />
          {showCmdSuggestions && cmdSuggestions.length > 0 && (
            <CommandSuggestions input={input} selectedIndex={cmdSuggestionIndex} customCommands={customCommands} />
          )}
          {currentMode === 'plan' && !isProcessing && (
            <Text color={colors.warning} dimColor>
              {icons.modePlan} plan mode on (shift+tab to cycle)
            </Text>
          )}
          {currentMode === 'accept' && !isProcessing && (
            <Text color={colors.success} dimColor>
              {icons.modeAccept} accept edits on (shift+tab to cycle)
            </Text>
          )}
        </Box>
      )}

      {isProcessing && !confirmState && !questionState ? (
        <ProgressBar startTime={processingStartTime} tokenCount={tokenCount} isThinking={isThinking} />
      ) : showCmdSuggestions && cmdSuggestions.length > 0 ? (
        <Box marginTop={1}>
          <Text color={colors.textMuted}>  Tab to complete · ↑↓ navigate</Text>
        </Box>
      ) : null}
    </Box>
  );
}
