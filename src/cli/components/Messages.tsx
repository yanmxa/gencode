import { useState, useEffect } from 'react';
import { Box, Text } from 'ink';
import InkSpinner from 'ink-spinner';
import stripAnsi from 'strip-ansi';
import { colors, icons } from './theme.js';
import { renderMarkdown } from './markdown.js';
import { formatTokens, formatCost } from '../../core/pricing/calculator.js';
import type { CostEstimate } from '../../core/pricing/types.js';

// Trim leading whitespace while preserving ANSI codes
// The issue is that ANSI escape codes come before whitespace, so /^\s+/ doesn't match
function trimLeadingWhitespace(str: string): string {
  // ANSI escape sequence pattern: ESC [ ... m
  const ansiPattern = /^(\x1b\[[0-9;]*m)*/;
  const match = str.match(ansiPattern);
  if (match && match[0]) {
    // Extract ANSI prefix, trim the rest, then recombine
    const ansiPrefix = match[0];
    const rest = str.slice(ansiPrefix.length);
    const trimmedRest = rest.replace(/^\s+/, '');
    return ansiPrefix + trimmedRest;
  }
  return str.replace(/^\s+/, '');
}

// Truncate string with ellipsis
const truncate = (str: string, maxLen: number) =>
  str.length > maxLen ? str.slice(0, maxLen - 3) + '...' : str;

// Compress multi-line text to single line, then truncate
const truncateSingleLine = (str: string, maxLen: number) => {
  // Replace newlines and multiple whitespace with single space
  const singleLine = str.replace(/\s+/g, ' ').trim();
  return singleLine.length > maxLen ? singleLine.slice(0, maxLen - 3) + '...' : singleLine;
};

// Get tool-specific icon (Claude Code style)
function getToolIcon(name: string): string {
  switch (name) {
    case 'Bash':
      return icons.toolBash;
    case 'Read':
      return icons.toolRead;
    case 'Write':
      return icons.toolWrite;
    case 'Edit':
      return icons.toolEdit;
    case 'Glob':
      return icons.toolGlob;
    case 'Grep':
      return icons.toolGrep;
    case 'WebFetch':
    case 'WebSearch':
      return icons.toolWeb;
    case 'TodoWrite':
      return icons.toolTodo;
    case 'AskUserQuestion':
      return icons.toolQuestion;
    case 'Task':
      return icons.toolTask;
    case 'LSP':
      return icons.toolLsp;
    case 'NotebookEdit':
      return icons.toolNotebook;
    default:
      return icons.tool;
  }
}

// Word wrap text to terminal width
function wrapText(text: string, width: number): string[] {
  const lines: string[] = [];
  for (const line of text.split('\n')) {
    if (line.length <= width) {
      lines.push(line);
    } else {
      // Simple word wrap
      let currentLine = '';
      const words = line.split(' ');
      for (const word of words) {
        if (currentLine.length + word.length + 1 <= width) {
          currentLine += (currentLine ? ' ' : '') + word;
        } else {
          if (currentLine) lines.push(currentLine);
          currentLine = word;
        }
      }
      if (currentLine) lines.push(currentLine);
    }
  }
  return lines;
}

interface UserMessageProps {
  text: string;
}

export function UserMessage({ text }: UserMessageProps) {
  const lines = text.trimEnd().split('\n');
  // Subtle gray - ~8% darker than pure white
  const inputBg = '#EFEFEF';

  return (
    <Box flexDirection="column" marginTop={1} marginBottom={0}>
      {lines.map((line, i) => (
        <Box key={i} backgroundColor={inputBg}>
          <Text color={colors.brand}>{icons.userPrompt} </Text>
          <Text>{line}</Text>
        </Box>
      ))}
    </Box>
  );
}

interface AssistantMessageProps {
  text: string;
  streaming?: boolean;
}

export function AssistantMessage({ text, streaming }: AssistantMessageProps) {
  if (!text) return null;

  // Streaming: use simple text display (markdown incomplete during stream)
  if (streaming) {
    const termWidth = process.stdout.columns || 80;
    const contentWidth = termWidth - 4;
    // Trim both ends to remove leading/trailing newlines
    const lines = wrapText(text.trim(), contentWidth);

    return (
      <Box flexDirection="column" marginTop={1} marginBottom={0}>
        {lines.map((line, i) => (
          <Box key={i}>
            <Text>
              {i === 0 && <Text color={colors.brand}>{icons.assistant}</Text>}
              {i === 0 ? ' ' : '  '}
              {line}
              {i === lines.length - 1 && (
                <Text color={colors.brandLight}>{icons.cursor}</Text>
              )}
            </Text>
          </Box>
        ))}
      </Box>
    );
  }

  // Completed: render with markdown (Claude Code style)
  // Trim to remove any leading/trailing whitespace from rendered markdown
  // Use ANSI-aware trimming because chalk colors may precede whitespace
  const rendered = trimLeadingWhitespace(renderMarkdown(text));

  return (
    <Box flexDirection="column" marginTop={1} marginBottom={0}>
      <Box>
        <Text>
          <Text color={colors.brand}>{icons.assistant}</Text>
          {' '}
          <Text wrap="wrap">{rendered}</Text>
        </Text>
      </Box>
    </Box>
  );
}

interface ToolCallProps {
  name: string;
  input: Record<string, unknown>;
}

// Format path for display (Claude Code style - show full path with ~ for home)
function formatPath(fullPath: string): string {
  if (!fullPath) return '';
  const home = process.env.HOME || '';
  if (home && fullPath.startsWith(home)) {
    return '~' + fullPath.slice(home.length);
  }
  return fullPath;
}

// Format tool input for display (Claude Code style: ToolName(param))
function formatToolInput(name: string, input: Record<string, unknown>): string {
  switch (name) {
    case 'Read':
    case 'Write':
    case 'Edit': {
      const filePath = (input.file_path as string) || '';
      return formatPath(filePath);
    }

    case 'Glob':
      return `pattern: "${input.pattern || ''}"`;

    case 'Grep': {
      const pattern = input.pattern as string;
      return `pattern: "${pattern}"`;
    }

    case 'Bash':
      // Return raw command; truncateSingleLine will handle compression at display time
      return (input.command as string) || '';

    case 'WebFetch':
      return (input.url as string) || '';

    case 'WebSearch':
      return `"${input.query}"` || '';

    case 'TodoWrite': {
      const todos = (input.todos as Array<{ content: string; status: string }>) || [];
      return `${todos.length} task${todos.length !== 1 ? 's' : ''}`;
    }

    case 'Task':
      return (input.description as string) || '';

    case 'AskUserQuestion':
      return truncate(JSON.stringify(input), 60);

    default:
      return truncate(JSON.stringify(input), 40);
  }
}

export function ToolCall({ name, input }: ToolCallProps) {
  // Hide TodoWrite (shown in TodoList component)
  if (name === 'TodoWrite') return null;

  const displayInput = formatToolInput(name, input);

  // Claude Code style: ⏺ Read(filename)
  return (
    <Box marginTop={1}>
      <Text>
        <Text color={colors.tool}>{icons.toolCall}</Text>
        {' '}
        <Text bold>{name}</Text>
        <Text color={colors.textMuted}>(</Text>
        <Text color={colors.textSecondary}>{truncateSingleLine(displayInput, 60)}</Text>
        <Text color={colors.textMuted}>)</Text>
      </Text>
    </Box>
  );
}

// Pending tool call with spinning indicator
interface PendingToolCallProps {
  name: string;
  input: Record<string, unknown>;
}

export function PendingToolCall({ name, input }: PendingToolCallProps) {
  // Hide TodoWrite (shown in TodoList component)
  if (name === 'TodoWrite') return null;

  const displayInput = formatToolInput(name, input);

  // Claude Code style with spinner: ⏺ Read(filename) - but show spinner before
  return (
    <Box marginTop={1}>
      <Text>
        <Text color={colors.tool}>{icons.toolCall}</Text>
        {' '}
        <Text bold>{name}</Text>
        <Text color={colors.textMuted}>(</Text>
        <Text color={colors.textSecondary}>{truncateSingleLine(displayInput, 55)}</Text>
        <Text color={colors.textMuted}>)</Text>
        <Text color={colors.warning}> <InkSpinner type="dots" /></Text>
      </Text>
    </Box>
  );
}

interface ToolResultMetadata {
  title?: string;
  subtitle?: string;
  size?: number;
  statusCode?: number;
  contentType?: string;
  duration?: number;
}

interface ToolResultProps {
  name: string;
  success: boolean;
  output: string;
  error?: string;
  metadata?: ToolResultMetadata;
  expanded?: boolean;
  id?: string;
}

// Generate tool result summary (Claude Code style)
function getToolResultSummary(name: string, success: boolean, output: string, error?: string): string {
  if (!success && error) {
    return `Error: ${truncate(error, 60)}`;
  }

  const lines = output.split('\n').filter(line => line.trim());
  const lineCount = lines.length;

  switch (name) {
    case 'Read':
      return `Read ${lineCount} line${lineCount !== 1 ? 's' : ''}`;
    case 'Write':
      return `Wrote ${lineCount} line${lineCount !== 1 ? 's' : ''}`;
    case 'Edit':
      return `Edited file`;
    case 'Glob':
      return `Found ${lineCount} file${lineCount !== 1 ? 's' : ''}`;
    case 'Grep':
      return `Found ${lineCount} match${lineCount !== 1 ? 'es' : ''}`;
    case 'Bash':
      if (output.trim() === '') return 'Done';
      if (lineCount === 1) return truncate(output.trim(), 60);
      // Show first line + remaining count inline (Claude Code style)
      const firstLine = truncate(lines[0], 40);
      return `${firstLine} … +${lineCount - 1} lines`;
    case 'WebFetch':
    case 'WebSearch':
      return `Received response`;
    case 'Task':
      return `Task completed`;
    default:
      return success ? 'Done' : 'Failed';
  }
}

export function ToolResult({ name, success, output, error, metadata, expanded }: ToolResultProps) {
  // If metadata has subtitle (e.g., "Received 540.3KB (200 OK)"), show it
  if (metadata?.subtitle) {
    return (
      <Box marginLeft={2}>
        <Text color={colors.textMuted}>{icons.toolResult}  </Text>
        <Text color={colors.textSecondary}>{metadata.subtitle}</Text>
      </Box>
    );
  }

  // TodoWrite: Don't show result (TodoList component shows the full list)
  if (name === 'TodoWrite') {
    return null;
  }

  // Get summary for display (Claude Code style)
  const summary = getToolResultSummary(name, success, output, error);
  const statusColor = success ? colors.textSecondary : colors.error;

  // Determine if content can be expanded
  const contentToShow = (!success && error) ? error : output;
  const lines = contentToShow.split('\n').filter(line => line.trim());
  const canExpand = lines.length > 0;

  // When not expanded, show summary only (Claude Code style)
  if (!expanded) {
    return (
      <Box marginLeft={2}>
        <Text color={colors.textMuted}>{icons.toolResult}  </Text>
        <Text color={statusColor}>{summary}</Text>
        {canExpand && (
          <>
            <Text color={colors.textMuted}> (</Text>
            <Text color={colors.info}>tab</Text>
            <Text color={colors.textMuted}> to expand)</Text>
          </>
        )}
      </Box>
    );
  }

  // When expanded, show full content
  return (
    <Box marginLeft={2} flexDirection="column">
      {/* Summary line */}
      <Box>
        <Text color={colors.textMuted}>{icons.toolResult}  </Text>
        <Text color={statusColor}>{summary}</Text>
        <Text color={colors.textMuted}> (</Text>
        <Text color={colors.info}>tab</Text>
        <Text color={colors.textMuted}> to collapse)</Text>
      </Box>
      {/* Content lines */}
      {lines.map((line, idx) => (
        <Box key={idx}>
          <Text color={colors.textMuted}>   </Text>
          <Text color={colors.textSecondary}>{truncate(line, 70)}</Text>
        </Box>
      ))}
    </Box>
  );
}

interface InfoMessageProps {
  text: string;
  type?: 'info' | 'success' | 'warning' | 'error';
}

export function InfoMessage({ text, type = 'info' }: InfoMessageProps) {
  const config = {
    info: { color: colors.info, icon: icons.info },
    success: { color: colors.success, icon: icons.success },
    warning: { color: colors.warning, icon: icons.warning },
    error: { color: colors.error, icon: icons.error },
  };
  const { color, icon } = config[type];

  return (
    <Box marginTop={1}>
      <Text color={color}>{icon} </Text>
      <Text color={colors.textSecondary}>{text}</Text>
    </Box>
  );
}

export function Separator() {
  const width = process.stdout.columns || 80;
  return <Text color={colors.separator}>{'─'.repeat(width)}</Text>;
}

interface WelcomeMessageProps {
  model: string;
}

// Format model name for display (same as Header.tsx)
function formatModelNameForWelcome(model: string): string {
  if (model.includes('opus')) return 'Opus 4.5';
  if (model.includes('sonnet')) return 'Sonnet 4';
  if (model.includes('haiku')) return 'Haiku 4';
  if (model.includes('gpt-4')) return 'GPT-4';
  if (model.includes('gpt-3.5')) return 'GPT-3.5';
  if (model.includes('gemini')) return 'Gemini';
  return model;
}

export function WelcomeMessage({ model }: WelcomeMessageProps) {
  // Claude Code style: "Welcome to [Model]"
  const displayModel = formatModelNameForWelcome(model);
  return (
    <Box marginTop={1}>
      <Text color={colors.textMuted}>  Welcome to {displayModel}</Text>
    </Box>
  );
}

export function ShortcutsHint() {
  return (
    <Box marginTop={1}>
      <Text color={colors.textMuted}>  ? for shortcuts</Text>
    </Box>
  );
}

function formatDuration(ms: number): string {
  const totalSecs = Math.floor(ms / 1000);
  const mins = Math.floor(totalSecs / 60);
  const secs = totalSecs % 60;
  if (mins > 0) {
    return `${mins}m ${secs}s`;
  }
  return `${secs}s`;
}

interface CompletionMessageProps {
  durationMs: number;
  usage?: {
    inputTokens: number;
    outputTokens: number;
  };
  cost?: CostEstimate;
}

export function CompletionMessage({ durationMs, usage, cost }: CompletionMessageProps) {
  // Build the message parts (Claude Code style - clean and informative)
  const parts: string[] = [];

  // Duration
  parts.push(formatDuration(durationMs));

  // Token usage
  if (usage) {
    parts.push(
      `${formatTokens(usage.inputTokens)} in → ${formatTokens(usage.outputTokens)} out`
    );
  }

  // Cost
  if (cost && cost.totalCost > 0) {
    parts.push(`~${formatCost(cost.totalCost)}`);
  }

  return (
    <Box marginTop={1}>
      <Text color={colors.textMuted}>✓ </Text>
      <Text color={colors.textMuted}>{parts.join(' • ')}</Text>
    </Box>
  );
}

interface CommandDefinition {
  name: string;
  description?: string;
  argumentHint?: string;
  level: 'user' | 'project';
  namespace: 'gen' | 'claude';
}

interface CommandListDisplayProps {
  commands: CommandDefinition[];
}

export function CommandListDisplay({ commands }: CommandListDisplayProps) {
  if (commands.length === 0) {
    return (
      <Box marginTop={1}>
        <Text color={colors.textMuted}>No custom commands found</Text>
      </Box>
    );
  }

  // Group commands by namespace
  const genCommands = commands.filter(c => c.namespace === 'gen');
  const claudeCommands = commands.filter(c => c.namespace === 'claude');

  return (
    <Box flexDirection="column" marginTop={1}>
      <Text bold color={colors.info}>Available Custom Commands:</Text>

      {genCommands.length > 0 && (
        <Box flexDirection="column" marginTop={1}>
          <Text dimColor>GenCode Commands:</Text>
          {genCommands.map(cmd => (
            <Box key={cmd.name} marginLeft={2}>
              <Text color={colors.success}>/{cmd.name}</Text>
              {cmd.argumentHint && <Text dimColor> {cmd.argumentHint}</Text>}
              {cmd.description && <Text dimColor> - {cmd.description}</Text>}
              <Text dimColor> ({cmd.level})</Text>
            </Box>
          ))}
        </Box>
      )}

      {claudeCommands.length > 0 && (
        <Box flexDirection="column" marginTop={1}>
          <Text dimColor>Claude Code Commands:</Text>
          {claudeCommands.map(cmd => (
            <Box key={cmd.name} marginLeft={2}>
              <Text color={colors.success}>/{cmd.name}</Text>
              {cmd.argumentHint && <Text dimColor> {cmd.argumentHint}</Text>}
              {cmd.description && <Text dimColor> - {cmd.description}</Text>}
              <Text dimColor> ({cmd.level})</Text>
            </Box>
          ))}
        </Box>
      )}
    </Box>
  );
}
