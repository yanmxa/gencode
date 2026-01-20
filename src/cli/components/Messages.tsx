import { useState, useEffect } from 'react';
import { Box, Text } from 'ink';
import InkSpinner from 'ink-spinner';
import { colors, icons } from './theme.js';
import { renderMarkdown } from './markdown.js';
import { formatTokens, formatCost } from '../../core/pricing/calculator.js';
import type { CostEstimate } from '../../core/pricing/types.js';

// Truncate string with ellipsis
const truncate = (str: string, maxLen: number) =>
  str.length > maxLen ? str.slice(0, maxLen - 3) + '...' : str;

// Compress multi-line text to single line, then truncate
const truncateSingleLine = (str: string, maxLen: number) => {
  // Replace newlines and multiple whitespace with single space
  const singleLine = str.replace(/\s+/g, ' ').trim();
  return singleLine.length > maxLen ? singleLine.slice(0, maxLen - 3) + '...' : singleLine;
};

// Get tool-specific icon
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
            {i === 0 && <Text color={colors.success}>{icons.assistant} </Text>}
            {i > 0 && <Text>  </Text>}
            <Text>
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

  // Completed: render with markdown
  const rendered = renderMarkdown(text);

  return (
    <Box flexDirection="column" marginTop={1} marginBottom={0}>
      <Box>
        <Text color={colors.success}>{icons.assistant}</Text>
        <Text> </Text>
        <Text wrap="wrap">{rendered}</Text>
      </Box>
    </Box>
  );
}

interface ToolCallProps {
  name: string;
  input: Record<string, unknown>;
}

// Format tool input for display
function formatToolInput(name: string, input: Record<string, unknown>): string {
  switch (name) {
    case 'Read':
    case 'Write':
    case 'Edit':
      return (input.file_path as string) || '';

    case 'Glob':
      return (input.pattern as string) || '';

    case 'Grep': {
      const pattern = `"${input.pattern}"`;
      return input.path ? `${pattern} in ${input.path}` : pattern;
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

    case 'AskUserQuestion':
      return truncate(JSON.stringify(input), 60);

    default:
      return truncate(JSON.stringify(input), 40);
  }
}

export function ToolCall({ name, input }: ToolCallProps) {
  // Hide TodoWrite (shown in TodoList component)
  if (name === 'TodoWrite') return null;

  const toolIcon = getToolIcon(name);
  const displayInput = formatToolInput(name, input);

  return (
    <Box marginTop={1}>
      <Text color={colors.statusSuccess}>{icons.statusCheck} </Text>
      <Text color={colors.toolHeader}>{toolIcon} </Text>
      <Text bold>{name}</Text>
      <Text>  </Text>
      <Text color={colors.textSecondary}>{truncateSingleLine(displayInput, 60)}</Text>
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

  const toolIcon = getToolIcon(name);
  const displayInput = formatToolInput(name, input);

  return (
    <Box marginTop={1}>
      <Text color={colors.statusRunning}><InkSpinner type="dots" /> </Text>
      <Text color={colors.toolHeader}>{toolIcon} </Text>
      <Text bold>{name}</Text>
      <Text>  </Text>
      <Text color={colors.textSecondary}>{truncateSingleLine(displayInput, 60)}</Text>
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

export function ToolResult({ name, success, output, error, metadata, expanded }: ToolResultProps) {
  const statusColor = success ? colors.statusSuccess : colors.statusError;

  // If metadata has subtitle (e.g., "Received 540.3KB (200 OK)"), show it
  if (metadata?.subtitle) {
    return (
      <Box marginLeft={2} marginTop={1}>
        <Text color={colors.textMuted}>└─ </Text>
        <Text color={colors.textSecondary}>{metadata.subtitle}</Text>
      </Box>
    );
  }

  // TodoWrite: Don't show result (TodoList component shows the full list)
  if (name === 'TodoWrite') {
    return null;
  }

  // Determine content to display: prioritize error field when failed
  const contentToShow = (!success && error) ? error : output;
  const lines = contentToShow.split('\n').filter(line => line.trim());
  const maxDisplayLines = 3;
  const isTruncated = !expanded && lines.length > maxDisplayLines;
  const remainingLines = lines.length - maxDisplayLines;

  // When expanded, show all lines
  const displayLines = expanded ? lines : lines.slice(0, maxDisplayLines);

  // First line with tree connector
  const firstLine = displayLines[0] || (success ? 'Done' : 'Failed');

  return (
    <Box marginLeft={2} flexDirection="column" marginTop={1}>
      <Box>
        <Text color={colors.textMuted}>└─ </Text>
        <Text color={statusColor}>{truncate(firstLine.trim(), 70)}</Text>
      </Box>
      {/* Additional lines */}
      {displayLines.slice(1).map((line, idx) => (
        <Box key={idx}>
          <Text color={colors.textMuted}>   </Text>
          <Text color={colors.textSecondary}>{truncate(line.trim(), 70)}</Text>
        </Box>
      ))}
      {/* Truncation indicator */}
      {isTruncated && (
        <Box>
          <Text color={colors.textMuted}>   ... {remainingLines} more lines</Text>
        </Box>
      )}
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

export function WelcomeMessage({ model: _model }: WelcomeMessageProps) {
  return (
    <Box marginTop={1} marginBottom={0}>
      <Text color={colors.textMuted}>? for help · Ctrl+C to exit</Text>
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

// Random verbs for completion message (Claude Code style)
const COMPLETION_VERBS = [
  'Baked',
  'Crafted',
  'Brewed',
  'Cooked',
  'Forged',
  'Built',
  'Woven',
  'Assembled',
  'Conjured',
  'Rendered',
  'Compiled',
  'Distilled',
];

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
  // Pick a random verb (stable per render via useMemo would be better, but keep simple)
  const verb = COMPLETION_VERBS[Math.floor(Math.random() * COMPLETION_VERBS.length)];

  // Build the message parts
  const parts = [`✻ ${verb} for ${formatDuration(durationMs)}`];

  if (usage) {
    parts.push(
      `Tokens: ${formatTokens(usage.inputTokens)} in / ${formatTokens(usage.outputTokens)} out`
    );
  }

  if (cost && cost.totalCost > 0) {
    parts.push(`(~${formatCost(cost.totalCost)})`);
  }

  return (
    <Box marginTop={1}>
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
