/**
 * DiffPreview Component - GitHub-style split view diff display
 * Features:
 * - Split view (side-by-side) like GitHub/OpenCode
 * - Line numbers on both sides
 * - Color-coded backgrounds for add/remove
 * - Expandable/collapsible for large diffs
 */

import { useState, useMemo } from 'react';
import { Box, Text, useInput } from 'ink';
import { diffWords } from 'diff';
import { colors, icons } from './theme.js';

// Maximum lines to parse before truncation (prevents UI blocking)
const MAX_DIFF_LINES = 500;

// Word diff part for highlighting
interface WordPart {
  value: string;
  added?: boolean;
  removed?: boolean;
}

// Parsed line data for split view
interface SplitLine {
  leftNum: number | null;
  leftContent: string;
  leftType: 'remove' | 'context' | 'empty';
  leftParts?: WordPart[];  // Word-level diff parts for left side
  rightNum: number | null;
  rightContent: string;
  rightType: 'add' | 'context' | 'empty';
  rightParts?: WordPart[]; // Word-level diff parts for right side
}

interface DiffPreviewProps {
  filePath: string;
  diff: string;
  collapsed?: boolean;
  maxLines?: number;
  onToggleExpand?: () => void;
  /** If true, this component manages its own expand state via 'e' key. If false, parent controls collapsed prop. */
  selfManaged?: boolean;
}

/**
 * Compute word-level diffs between two lines
 * Returns parts for both left (removed) and right (added) sides
 */
function computeWordDiff(oldLine: string, newLine: string): { leftParts: WordPart[]; rightParts: WordPart[] } {
  const diffs = diffWords(oldLine, newLine);
  const leftParts: WordPart[] = [];
  const rightParts: WordPart[] = [];

  for (const part of diffs) {
    if (part.added) {
      // Only in new line (right side)
      rightParts.push({ value: part.value, added: true });
    } else if (part.removed) {
      // Only in old line (left side)
      leftParts.push({ value: part.value, removed: true });
    } else {
      // Unchanged - appears in both
      leftParts.push({ value: part.value });
      rightParts.push({ value: part.value });
    }
  }

  return { leftParts, rightParts };
}

/**
 * Parse hunk header to get line numbers
 * Format: @@ -oldStart,oldCount +newStart,newCount @@
 */
function parseHunkHeader(line: string): { oldStart: number; newStart: number; hunkInfo: string } {
  const match = line.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)?/);
  if (match) {
    return {
      oldStart: parseInt(match[1], 10),
      newStart: parseInt(match[2], 10),
      hunkInfo: match[3]?.trim() || '',
    };
  }
  return { oldStart: 1, newStart: 1, hunkInfo: '' };
}

/**
 * Parse unified diff into split view format
 * Groups consecutive removes and adds together for side-by-side display
 */
function parseDiffToSplitView(diff: string): { lines: SplitLine[]; hunks: string[] } {
  const result: SplitLine[] = [];
  const hunks: string[] = [];
  let oldLineNum = 0;
  let newLineNum = 0;

  const rawLines = diff.split('\n');
  let i = 0;

  while (i < rawLines.length) {
    const line = rawLines[i];

    // Skip file headers
    if (line.startsWith('---') || line.startsWith('+++')) {
      i++;
      continue;
    }

    // Hunk header
    if (line.startsWith('@@')) {
      const { oldStart, newStart, hunkInfo } = parseHunkHeader(line);
      oldLineNum = oldStart;
      newLineNum = newStart;
      hunks.push(hunkInfo || line);
      i++;
      continue;
    }

    // Context line (unchanged)
    if (line.startsWith(' ') || (!line.startsWith('+') && !line.startsWith('-') && line.length > 0)) {
      const content = line.startsWith(' ') ? line.slice(1) : line;
      result.push({
        leftNum: oldLineNum,
        leftContent: content,
        leftType: 'context',
        rightNum: newLineNum,
        rightContent: content,
        rightType: 'context',
      });
      oldLineNum++;
      newLineNum++;
      i++;
      continue;
    }

    // Collect consecutive removes and adds for side-by-side pairing
    const removes: { num: number; content: string }[] = [];
    const adds: { num: number; content: string }[] = [];

    // Collect all removes
    while (i < rawLines.length && rawLines[i].startsWith('-')) {
      removes.push({ num: oldLineNum, content: rawLines[i].slice(1) });
      oldLineNum++;
      i++;
    }

    // Collect all adds
    while (i < rawLines.length && rawLines[i].startsWith('+')) {
      adds.push({ num: newLineNum, content: rawLines[i].slice(1) });
      newLineNum++;
      i++;
    }

    // Pair removes with adds for side-by-side display
    const maxLen = Math.max(removes.length, adds.length);
    for (let j = 0; j < maxLen; j++) {
      const remove = removes[j];
      const add = adds[j];

      // Compute word-level diff when we have a paired remove/add
      let leftParts: WordPart[] | undefined;
      let rightParts: WordPart[] | undefined;
      if (remove && add) {
        const wordDiff = computeWordDiff(remove.content, add.content);
        leftParts = wordDiff.leftParts;
        rightParts = wordDiff.rightParts;
      }

      result.push({
        leftNum: remove?.num ?? null,
        leftContent: remove?.content ?? '',
        leftType: remove ? 'remove' : 'empty',
        leftParts,
        rightNum: add?.num ?? null,
        rightContent: add?.content ?? '',
        rightType: add ? 'add' : 'empty',
        rightParts,
      });
    }
  }

  return { lines: result, hunks };
}

/**
 * Truncate string with ellipsis, respecting terminal width
 */
function truncateStr(str: string, maxLen: number): string {
  if (str.length <= maxLen) return str.padEnd(maxLen);
  return str.slice(0, maxLen - 1) + '…';
}

/**
 * Truncate word parts to fit within maxLen, preserving highlighting info
 */
function truncateWordParts(parts: WordPart[], maxLen: number): WordPart[] {
  const result: WordPart[] = [];
  let totalLen = 0;

  for (const part of parts) {
    if (totalLen >= maxLen) break;

    const remaining = maxLen - totalLen;
    if (part.value.length <= remaining) {
      result.push(part);
      totalLen += part.value.length;
    } else {
      // Truncate this part
      result.push({
        ...part,
        value: part.value.slice(0, remaining - 1) + '…',
      });
      totalLen = maxLen;
      break;
    }
  }

  // Pad the last part or add padding part
  if (totalLen < maxLen) {
    const padding = ' '.repeat(maxLen - totalLen);
    if (result.length > 0) {
      const lastPart = result[result.length - 1];
      result[result.length - 1] = { ...lastPart, value: lastPart.value + padding };
    } else {
      result.push({ value: padding });
    }
  }

  return result;
}

/**
 * Single row in split view
 * Note: Ink has issues with multiple adjacent Text elements, so we build each side as a single string
 */
function SplitRow({
  line,
  lineNumWidth,
  contentWidth,
}: {
  line: SplitLine;
  lineNumWidth: number;
  contentWidth: number;
}) {
  const padNum = (num: number | null) => {
    if (num === null) return ' '.repeat(lineNumWidth);
    return String(num).padStart(lineNumWidth, ' ');
  };

  // Colors for line types
  const leftColor = line.leftType === 'remove' ? colors.diffRemove : colors.textSecondary;
  const rightColor = line.rightType === 'add' ? colors.diffAdd : colors.textSecondary;

  // Prefixes
  const leftPrefix = line.leftType === 'remove' ? '-' : ' ';
  const rightPrefix = line.rightType === 'add' ? '+' : ' ';

  // Build full lines as single strings (Ink has issues with multiple adjacent Text elements)
  const leftLine = `${padNum(line.leftNum)} ${leftPrefix} ${truncateStr(line.leftContent, contentWidth)}`;
  const rightLine = `${padNum(line.rightNum)} ${rightPrefix} ${truncateStr(line.rightContent, contentWidth)}`;

  return (
    <Box>
      <Text color={leftColor} bold={line.leftType === 'remove'}>{leftLine}</Text>
      <Text color={colors.toolBorder}>│</Text>
      <Text color={rightColor} bold={line.rightType === 'add'}>{rightLine}</Text>
    </Box>
  );
}

/**
 * DiffPreview component - GitHub-style split view diff display
 * Features side-by-side comparison, line numbers, and expand/collapse
 */
export function DiffPreview({
  filePath,
  diff,
  collapsed = true,
  maxLines = 20,
  onToggleExpand,
  selfManaged = false,
}: DiffPreviewProps) {
  // Internal state for self-managed mode (initialized from prop)
  const [internalCollapsed, setInternalCollapsed] = useState(collapsed);

  // Use internal state if self-managed, otherwise use the live prop value
  const isCollapsed = selfManaged ? internalCollapsed : collapsed;

  // Handle keyboard input for expand/collapse only when self-managed
  useInput((input, key) => {
    if (selfManaged && input.toLowerCase() === 'e') {
      setInternalCollapsed(!internalCollapsed);
      onToggleExpand?.();
    }
  });

  // Memoized diff parsing with error handling and size limit
  const { lines, parseError, wasTruncated } = useMemo(() => {
    try {
      // Truncate large diffs before parsing to prevent UI blocking
      const rawLines = diff.split('\n');
      let truncatedDiff = diff;
      let wasTruncated = false;

      if (rawLines.length > MAX_DIFF_LINES) {
        truncatedDiff = rawLines.slice(0, MAX_DIFF_LINES).join('\n');
        wasTruncated = true;
      }

      const result = parseDiffToSplitView(truncatedDiff);
      return { lines: result.lines, parseError: null, wasTruncated };
    } catch (error) {
      console.error('Failed to parse diff:', error);
      return {
        lines: [],
        parseError: error instanceof Error ? error.message : String(error),
        wasTruncated: false,
      };
    }
  }, [diff]);

  // Error fallback UI
  if (parseError) {
    const shortPath = filePath.replace(process.env.HOME || '', '~');
    return (
      <Box flexDirection="column" borderStyle="round" borderColor={colors.error} paddingX={1}>
        <Box>
          <Text color={colors.tool}>{icons.toolEdit} </Text>
          <Text color={colors.text} bold>{shortPath}</Text>
        </Box>
        <Text color={colors.error}>Failed to parse diff: {parseError}</Text>
        <Text color={colors.textSecondary}>{diff.slice(0, 300)}...</Text>
      </Box>
    );
  }

  // Count additions and deletions
  const addCount = lines.filter(l => l.rightType === 'add').length;
  const removeCount = lines.filter(l => l.leftType === 'remove').length;

  // Calculate widths
  const termWidth = process.stdout.columns || 120;
  const availableWidth = termWidth - 10; // borders and padding
  const halfWidth = Math.floor(availableWidth / 2) - 6; // line num + prefix + separator
  const lineNumWidth = 4;
  const contentWidth = Math.max(halfWidth - lineNumWidth - 2, 20);

  // Determine if we should show collapse/expand
  const isTruncated = isCollapsed && lines.length > maxLines;
  const displayLines = isTruncated ? lines.slice(0, maxLines) : lines;
  const hiddenCount = lines.length - maxLines;

  // Shorten file path for display
  const shortPath = filePath.replace(process.env.HOME || '', '~');

  // Calculate separator line width
  const separatorWidth = Math.min(termWidth - 6, contentWidth * 2 + lineNumWidth * 2 + 10);

  return (
    <Box flexDirection="column" borderStyle="round" borderColor={colors.toolBorder} paddingX={1}>
      {/* Header with file path and stats */}
      <Box justifyContent="space-between">
        <Box>
          <Text color={colors.tool}>{icons.toolEdit} </Text>
          <Text color={colors.text} bold>{shortPath}</Text>
        </Box>
        <Box>
          <Text color={colors.diffAdd} bold>+{addCount}</Text>
          <Text color={colors.textMuted}> / </Text>
          <Text color={colors.diffRemove} bold>-{removeCount}</Text>
        </Box>
      </Box>

      {/* Column headers */}
      <Box marginTop={1}>
        <Text color={colors.textMuted}>{' '.repeat(lineNumWidth + 2)}</Text>
        <Text color={colors.diffRemove} bold>Original</Text>
        <Text color={colors.textMuted}>{' '.repeat(Math.max(0, contentWidth - 7))}</Text>
        <Text color={colors.toolBorder}>│</Text>
        <Text color={colors.textMuted}>{' '.repeat(lineNumWidth + 2)}</Text>
        <Text color={colors.diffAdd} bold>Modified</Text>
      </Box>

      {/* Split view diff content */}
      <Box flexDirection="column">
        {displayLines.map((line, idx) => (
          <SplitRow
            key={idx}
            line={line}
            lineNumWidth={lineNumWidth}
            contentWidth={contentWidth}
          />
        ))}
      </Box>

      {/* Expand/collapse indicator */}
      {isTruncated && (
        <Box justifyContent="center" marginTop={1}>
          <Text color={colors.info} bold>
            {icons.expand} {hiddenCount} more lines
          </Text>
          <Text color={colors.textMuted}> (</Text>
          <Text color={colors.warning}>e</Text>
          <Text color={colors.textMuted}> to expand)</Text>
        </Box>
      )}

      {/* Show collapse hint when expanded */}
      {!isCollapsed && lines.length > maxLines && (
        <Box justifyContent="center" marginTop={1}>
          <Text color={colors.textMuted}>(</Text>
          <Text color={colors.warning}>e</Text>
          <Text color={colors.textMuted}> to collapse)</Text>
        </Box>
      )}

      {/* Show truncation warning for very large diffs */}
      {wasTruncated && (
        <Box justifyContent="center" marginTop={1}>
          <Text color={colors.warning}>
            {icons.warning} Diff truncated to {MAX_DIFF_LINES} lines for performance
          </Text>
        </Box>
      )}
    </Box>
  );
}

/**
 * Compact unified diff view (fallback for narrow terminals)
 */
export function UnifiedDiffPreview({
  filePath,
  diff,
  collapsed = true,
  maxLines = 30,
}: DiffPreviewProps) {
  const [isCollapsed, setIsCollapsed] = useState(collapsed);

  // Use 'e' key to avoid conflict with Tab in PermissionPrompt
  useInput((input, key) => {
    if (input.toLowerCase() === 'e') {
      setIsCollapsed(!isCollapsed);
    }
  });

  const rawLines = diff.split('\n');
  const displayLines = isCollapsed ? rawLines.slice(0, maxLines) : rawLines;
  const isTruncated = isCollapsed && rawLines.length > maxLines;

  const shortPath = filePath.replace(process.env.HOME || '', '~');

  return (
    <Box flexDirection="column" borderStyle="round" borderColor={colors.toolBorder} paddingX={1}>
      <Text color={colors.tool}>{icons.toolEdit} </Text>
      <Text color={colors.text} bold>{shortPath}</Text>

      <Box flexDirection="column" marginTop={1}>
        {displayLines.map((line, idx) => {
          let color = colors.textSecondary;
          if (line.startsWith('+')) color = colors.diffAdd;
          else if (line.startsWith('-')) color = colors.diffRemove;
          else if (line.startsWith('@@')) color = colors.diffHunk;

          return (
            <Text key={idx} color={color}>
              {line}
            </Text>
          );
        })}
      </Box>

      {isTruncated && (
        <Box justifyContent="center" marginTop={1}>
          <Text color={colors.info}>{icons.expand} {rawLines.length - maxLines} more lines</Text>
          <Text color={colors.textMuted}> (e to expand)</Text>
        </Box>
      )}
    </Box>
  );
}
