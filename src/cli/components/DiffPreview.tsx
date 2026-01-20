/**
 * DiffPreview Component - Simplified
 * Displays unified diff with syntax highlighting for Edit tool
 */

import { Box, Text } from 'ink';
import { colors } from './theme.js';

interface DiffLineData {
  type: 'header' | 'hunk' | 'add' | 'remove' | 'context';
  content: string;
}

interface DiffPreviewProps {
  filePath: string;
  diff: string;
  collapsed?: boolean;
  maxLines?: number;
}

/**
 * Parse unified diff format into typed lines
 */
function parseDiff(diff: string): DiffLineData[] {
  const lines: DiffLineData[] = [];

  for (const line of diff.split('\n')) {
    if (line.startsWith('---') || line.startsWith('+++')) {
      lines.push({ type: 'header', content: line });
    } else if (line.startsWith('@@')) {
      lines.push({ type: 'hunk', content: line });
    } else if (line.startsWith('+')) {
      lines.push({ type: 'add', content: line });
    } else if (line.startsWith('-')) {
      lines.push({ type: 'remove', content: line });
    } else {
      lines.push({ type: 'context', content: line });
    }
  }

  return lines;
}

/**
 * Render a single diff line with appropriate color
 */
function DiffLine({ line }: { line: DiffLineData }) {
  const colorMap: Record<DiffLineData['type'], string> = {
    header: colors.textMuted,
    hunk: colors.diffHunk,
    add: colors.diffAdd,
    remove: colors.diffRemove,
    context: colors.textSecondary,
  };

  const color = colorMap[line.type];

  // Don't show prefix for header lines (they already have --- or +++)
  if (line.type === 'header') {
    return <Text color={color}>{line.content}</Text>;
  }

  // For hunk headers, show as-is with cyan color
  if (line.type === 'hunk') {
    return <Text color={colors.diffHunk}>{line.content}</Text>;
  }

  // For add/remove/context, show with proper prefix highlighting
  const prefix = line.content.charAt(0);
  const content = line.content.slice(1);

  return (
    <Text>
      <Text color={color} bold={line.type === 'add' || line.type === 'remove'}>
        {prefix}
      </Text>
      <Text color={color}>{content}</Text>
    </Text>
  );
}

/**
 * DiffPreview component - displays unified diff with syntax highlighting
 * Uses Ink's native borderStyle for proper alignment
 */
export function DiffPreview({ filePath, diff, collapsed = true, maxLines = 50 }: DiffPreviewProps) {
  const lines = parseDiff(diff);

  // Count additions and deletions
  const addCount = lines.filter(l => l.type === 'add').length;
  const removeCount = lines.filter(l => l.type === 'remove').length;

  // Determine if we should show collapse/expand
  const isTruncated = collapsed && lines.length > maxLines;
  const displayLines = isTruncated ? lines.slice(0, maxLines) : lines;
  const hiddenCount = lines.length - maxLines;

  return (
    <Box
      flexDirection="column"
      borderStyle="single"
      borderColor={colors.toolBorder}
      paddingX={1}
    >
      {/* Header with file path and stats */}
      <Box>
        <Text color={colors.toolHeader}>[E] </Text>
        <Text color={colors.textSecondary}>{filePath}</Text>
        <Text>  </Text>
        <Text color={colors.diffAdd}>+{addCount}</Text>
        <Text color={colors.textMuted}>/</Text>
        <Text color={colors.diffRemove}>-{removeCount}</Text>
      </Box>

      {/* Diff content */}
      <Box flexDirection="column" marginTop={1}>
        {displayLines.map((line, idx) => (
          <Box key={idx}>
            <DiffLine line={line} />
          </Box>
        ))}
      </Box>

      {/* Truncation indicator */}
      {isTruncated && (
        <Box justifyContent="flex-end" marginTop={1}>
          <Text color={colors.textMuted}>
            ▼ {hiddenCount} more lines
          </Text>
        </Box>
      )}
    </Box>
  );
}
