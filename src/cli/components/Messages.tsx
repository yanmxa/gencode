import { Box, Text } from 'ink';
import { colors, icons } from './theme.js';

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
  return (
    <Box flexDirection="column" marginTop={1} marginBottom={0}>
      {lines.map((line, i) => (
        <Box key={i}>
          <Text color={colors.brand}>{icons.userPrompt} </Text>
          <Text backgroundColor="#1E293B" color={colors.text}> {line} </Text>
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

  // Get terminal width for wrapping
  const termWidth = process.stdout.columns || 80;
  const contentWidth = termWidth - 4; // Account for prefix

  // Wrap text to terminal width
  const lines = wrapText(text.trimEnd(), contentWidth);

  return (
    <Box flexDirection="column" marginTop={1} marginBottom={0}>
      {lines.map((line, i) => (
        <Box key={i}>
          {i === 0 && <Text color={colors.success}>{icons.assistant} </Text>}
          {i > 0 && <Text>  </Text>}
          <Text>
            {line}
            {streaming && i === lines.length - 1 ? (
              <Text color={colors.brandLight}>{icons.cursor}</Text>
            ) : (
              ''
            )}
          </Text>
        </Box>
      ))}
    </Box>
  );
}

interface ToolCallProps {
  name: string;
  input: Record<string, unknown>;
}

export function ToolCall({ name, input }: ToolCallProps) {
  // WebFetch: Show "Fetch(url)" instead of JSON (Claude Code style)
  if (name === 'WebFetch' && input?.url) {
    const url = input.url as string;
    const shortUrl = url.length > 60 ? url.slice(0, 57) + '...' : url;
    return (
      <Box marginLeft={2}>
        <Text color={colors.tool}>{icons.fetch}</Text>
        <Text> Fetch(</Text>
        <Text color={colors.info}>{shortUrl}</Text>
        <Text>)</Text>
      </Box>
    );
  }

  // Default: Show tool name with JSON input
  const inputStr = JSON.stringify(input);
  const shortInput = inputStr.length > 50 ? inputStr.slice(0, 47) + '...' : inputStr;

  return (
    <Box marginLeft={2}>
      <Text dimColor>
        <Text color={colors.tool}>{icons.tool}</Text> {name}{' '}
        <Text color={colors.textMuted}>{shortInput}</Text>
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
  metadata?: ToolResultMetadata;
}

export function ToolResult({ name, success, output, metadata }: ToolResultProps) {
  const statusColor = success ? colors.success : colors.error;
  const statusIcon = success ? icons.success : icons.error;

  // If metadata has subtitle (e.g., "Received 540.3KB (200 OK)"), show it
  if (metadata?.subtitle) {
    return (
      <Box marginLeft={2}>
        <Text dimColor>
          <Text color={statusColor}>{statusIcon}</Text>{' '}
          <Text>{metadata.subtitle}</Text>
        </Text>
      </Box>
    );
  }

  // Default: Show first line of output
  const firstLine = output.split('\n')[0]?.trim() || '';
  const displayOutput = firstLine.length > 50 ? firstLine.slice(0, 47) + '...' : firstLine;

  return (
    <Box marginLeft={2}>
      <Text dimColor>
        <Text color={statusColor}>{statusIcon}</Text> {name}{' '}
        <Text color={colors.textMuted}>{displayOutput}</Text>
      </Text>
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
    <Box>
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

export function WelcomeMessage({ model }: WelcomeMessageProps) {
  return (
    <Box marginTop={1} marginBottom={0}>
      <Text color={colors.textMuted}>Welcome to </Text>
      <Text color={colors.brand}>{model}</Text>
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
}

export function CompletionMessage({ durationMs }: CompletionMessageProps) {
  // Pick a random verb (stable per render via useMemo would be better, but keep simple)
  const verb = COMPLETION_VERBS[Math.floor(Math.random() * COMPLETION_VERBS.length)];
  return (
    <Box marginTop={1}>
      <Text color={colors.textMuted}>
        ✻ {verb} for {formatDuration(durationMs)}
      </Text>
    </Box>
  );
}
