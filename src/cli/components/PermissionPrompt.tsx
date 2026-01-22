/**
 * Permission Prompt Component - Claude Code style UI
 *
 * Design (horizontal line style):
 * ─────────────────────────────────────────────────────
 *  ⏺ Bash
 *    osascript -e '
 *      tell application "Mail"
 *      ...
 *                                  ▼ 28 more lines tab
 *
 *  Do you want to proceed?
 *    ▸ [1] Yes
 *      [2] Yes, always in ~/project
 *      [3] No
 * ─────────────────────────────────────────────────────
 */

import { useState } from 'react';
import type { ReactElement } from 'react';
import { Box, Text, useInput } from 'ink';
import { colors, icons } from './theme.js';
import { DiffPreview } from './DiffPreview.js';
import type { ApprovalAction, ApprovalSuggestion } from '../../core/permissions/types.js';

// ============================================================================
// Types
// ============================================================================

interface PermissionPromptProps {
  /** Tool name */
  tool: string;
  /** Tool input/parameters */
  input: Record<string, unknown>;
  /** Available approval options */
  suggestions: ApprovalSuggestion[];
  /** Callback when user makes a decision */
  onDecision: (action: ApprovalAction) => void;
  /** Project path for "don't ask again" option */
  projectPath?: string;
  /** Additional metadata for display (e.g., diff preview) */
  metadata?: Record<string, unknown>;
}

// ============================================================================
// Helpers
// ============================================================================

/**
 * Format tool name for display (Claude Code style: "Web Search" instead of "WebSearch")
 */
function formatToolName(tool: string): string {
  // Convert camelCase/PascalCase to space-separated
  return tool.replace(/([a-z])([A-Z])/g, '$1 $2');
}

/**
 * Format tool input for display
 * Returns string or ReactElement (for special previews like diff)
 */
function formatInput(
  tool: string,
  input: Record<string, unknown>,
  metadata?: Record<string, unknown>
): string | ReactElement {
  // Special case: Edit tool with diff metadata
  if (tool === 'Edit' && metadata?.diff && metadata?.filePath) {
    return (
      <DiffPreview
        filePath={metadata.filePath as string}
        diff={metadata.diff as string}
        collapsed={true}
      />
    );
  }

  // Special case: Write tool with diff metadata (overwriting existing file)
  if (tool === 'Write' && metadata?.diff && metadata?.filePath) {
    return (
      <DiffPreview
        filePath={metadata.filePath as string}
        diff={metadata.diff as string}
        collapsed={true}
      />
    );
  }

  switch (tool) {
    case 'Bash':
      return (input.command as string) ?? JSON.stringify(input);

    case 'Read':
    case 'Edit':
      return (input.file_path as string) ?? (input.path as string) ?? '';

    case 'Write': {
      const filePath = (input.file_path as string) ?? (input.path as string) ?? '';
      const content = input.content as string | undefined;
      if (content) {
        // Show file path as header with content preview (Claude Code style)
        const shortPath = filePath.replace(process.env.HOME || '', '~');
        const lines = content.split('\n');
        const preview = lines.slice(0, 10).join('\n');
        const moreLines = lines.length > 10 ? `\n... +${lines.length - 10} more lines` : '';
        // Use terminal width for dashed lines
        const termWidth = process.stdout.columns || 80;
        const dashedLine = '╌'.repeat(termWidth - 4);
        return `Create file ${shortPath}\n${dashedLine}\n${preview}${moreLines}\n${dashedLine}`;
      }
      return filePath;
    }

    case 'Glob':
      return `${input.pattern ?? ''} in ${input.path ?? '.'}`;

    case 'Grep':
      return `${input.pattern ?? ''} in ${input.path ?? '.'}`;

    case 'WebFetch':
      return (input.url as string) ?? '';

    case 'WebSearch':
      return (input.query as string) ?? '';

    case 'TodoWrite': {
      const todos = input.todos as Array<{ content: string; status: string }> || [];
      const inProgress = todos.filter(t => t.status === 'in_progress');
      const pending = todos.filter(t => t.status === 'pending');
      return `${todos.length} tasks: ${inProgress.length} active, ${pending.length} pending`;
    }

    default:
      const str = JSON.stringify(input);
      return str.length > 80 ? str.slice(0, 77) + '...' : str;
  }
}

/**
 * Get icon for tool (Claude Code style - clean Unicode)
 */
function getToolIcon(tool: string): string {
  switch (tool) {
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

// ============================================================================
// Permission Prompt Component
// ============================================================================

export function PermissionPrompt({
  tool,
  input,
  suggestions,
  onDecision,
  projectPath,
  metadata,
}: PermissionPromptProps) {
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [expanded, setExpanded] = useState(false);
  const [diffCollapsed, setDiffCollapsed] = useState(true);

  // Check if we're showing a diff preview
  const hasDiffPreview = !!(metadata?.diff && metadata?.filePath);

  // Handle keyboard input
  useInput((inputChar, key) => {
    // Arrow navigation
    if (key.upArrow) {
      setSelectedIndex((i) => Math.max(0, i - 1));
    } else if (key.downArrow) {
      setSelectedIndex((i) => Math.min(suggestions.length - 1, i + 1));
    }

    // Enter to select current option
    if (key.return) {
      const selected = suggestions[selectedIndex];
      if (selected) {
        onDecision(selected.action);
      }
    }

    // Escape to deny
    if (key.escape) {
      onDecision('deny');
    }

    // Tab to toggle expand (only when not showing diff preview)
    if (key.tab && !hasDiffPreview) {
      setExpanded((e) => !e);
    }

    // 'e' key to toggle diff expand/collapse when showing diff preview
    if (inputChar.toLowerCase() === 'e' && hasDiffPreview) {
      setDiffCollapsed((c) => !c);
    }

    // Number shortcuts (1-3)
    const num = parseInt(inputChar, 10);
    if (num >= 1 && num <= suggestions.length) {
      onDecision(suggestions[num - 1].action);
    }

    // 'y' for allow once, 'n' for deny
    if (inputChar.toLowerCase() === 'y') {
      onDecision('allow_once');
    } else if (inputChar.toLowerCase() === 'n') {
      onDecision('deny');
    }
  });

  const displayToolName = formatToolName(tool);
  const toolIcon = getToolIcon(tool);

  // For diff preview, we manage state here; otherwise use formatInput
  const fullInput = hasDiffPreview ? null : formatInput(tool, input, metadata);

  // Parse content lines for display (only for non-diff content)
  let contentLines: string[] = [];
  if (typeof fullInput === 'string') {
    contentLines = fullInput.split('\n').filter(Boolean);
  }
  const maxContentLines = 8;
  const isLongInput = contentLines.length > maxContentLines;
  const displayContentLines = expanded ? contentLines : contentLines.slice(0, maxContentLines);
  const hiddenLines = contentLines.length - maxContentLines;

  // Get dynamic label for "don't ask again" option
  const getDynamicLabel = (suggestion: ApprovalSuggestion): string => {
    if (suggestion.action === 'allow_always' && projectPath) {
      const shortPath = projectPath.replace(process.env.HOME || '', '~');
      return `Yes, always in ${shortPath}`;
    }
    return suggestion.label;
  };

  // Get terminal width for horizontal line
  const termWidth = process.stdout.columns || 80;
  const lineChar = '─';

  return (
    <Box flexDirection="column" marginTop={1}>
      {/* Top horizontal line */}
      <Text color={colors.textMuted}>{lineChar.repeat(termWidth)}</Text>

      {/* Tool name header - Claude style with ⏺ icon */}
      <Box marginTop={1}>
        <Text><Text color={colors.tool}>{icons.toolCall}</Text> <Text bold>{displayToolName}</Text></Text>
      </Box>

      {/* Content: either text lines, DiffPreview, or other JSX */}
      <Box flexDirection="column" marginLeft={2} marginY={1}>
        {hasDiffPreview ? (
          // Render DiffPreview with managed collapse state
          <DiffPreview
            filePath={metadata.filePath as string}
            diff={metadata.diff as string}
            collapsed={diffCollapsed}
            selfManaged={false}
          />
        ) : typeof fullInput === 'string' ? (
          <>
            {displayContentLines.map((line, idx) => (
              <Text key={idx} color={colors.textSecondary}>{line}</Text>
            ))}
            {isLongInput && (
              <Box justifyContent="flex-end">
                <Text color={colors.textMuted}>
                  {expanded ? '▲ less' : `▼ ${hiddenLines} more lines`}  tab
                </Text>
              </Box>
            )}
          </>
        ) : fullInput ? (
          // Render other JSX element
          <Box flexDirection="column">{fullInput}</Box>
        ) : null}
      </Box>

      {/* Question - Claude style */}
      <Box marginTop={1}>
        <Text bold>Do you want to proceed?</Text>
      </Box>

      {/* Options - arrow, shortcut, then label */}
      <Box flexDirection="column" marginLeft={2} marginY={1}>
        {suggestions.map((suggestion, index) => {
          const isSelected = index === selectedIndex;
          const label = getDynamicLabel(suggestion);

          return (
            <Box key={suggestion.action}>
              <Text color={isSelected ? colors.brand : colors.textMuted}>
                {isSelected ? '▸ ' : '  '}
              </Text>
              <Text color={colors.textMuted}>[{suggestion.shortcut}] </Text>
              <Text color={isSelected ? colors.text : colors.textSecondary} bold={isSelected}>
                {label}
              </Text>
            </Box>
          );
        })}
      </Box>

      {/* Footer hint (Claude style) */}
      <Box marginTop={1}>
        <Text color={colors.textMuted}>Esc to cancel</Text>
      </Box>

      {/* Bottom horizontal line */}
      <Text color={colors.textMuted}>{lineChar.repeat(termWidth)}</Text>
    </Box>
  );
}

// ============================================================================
// Simple Confirm Prompt (backward compatible)
// ============================================================================

interface SimpleConfirmProps {
  message: string;
  onConfirm: (confirmed: boolean) => void;
}

export function SimpleConfirmPrompt({ message, onConfirm }: SimpleConfirmProps) {
  useInput((inputChar, key) => {
    if (inputChar.toLowerCase() === 'y' || key.return) {
      onConfirm(true);
    } else if (inputChar.toLowerCase() === 'n' || key.escape) {
      onConfirm(false);
    }
  });

  return (
    <Box>
      <Text color={colors.warning}>{icons.warning} </Text>
      <Text>{message} </Text>
      <Text color={colors.textMuted}>[y/n] </Text>
    </Box>
  );
}

// ============================================================================
// Permission Rules Display
// ============================================================================

interface PermissionRule {
  type: string;
  tool: string;
  pattern?: string;
  scope: string;
  mode: string;
}

interface PermissionRulesProps {
  rules: PermissionRule[];
  allowedPrompts?: { tool: string; prompt: string }[];
}

export function PermissionRulesDisplay({ rules, allowedPrompts }: PermissionRulesProps) {
  return (
    <Box flexDirection="column">
      <Text color={colors.primary} bold>Permission Rules</Text>
      <Box flexDirection="column" marginTop={1}>
        {/* Header */}
        <Text color={colors.textMuted}>
          {'Type'.padEnd(10)}{'Tool'.padEnd(12)}{'Pattern'.padEnd(20)}{'Scope'.padEnd(10)}Mode
        </Text>
        <Text color={colors.textMuted}>{'─'.repeat(60)}</Text>

        {/* Rules */}
        {rules.map((rule, i) => (
          <Text key={i}>
            <Text color={colors.textSecondary}>{rule.type.padEnd(10)}</Text>
            <Text color={colors.tool}>{rule.tool.padEnd(12)}</Text>
            <Text>{(rule.pattern ?? '*').slice(0, 18).padEnd(20)}</Text>
            <Text color={colors.textMuted}>{rule.scope.padEnd(10)}</Text>
            <Text color={rule.mode === 'auto' ? colors.success : rule.mode === 'deny' ? colors.error : colors.warning}>
              {rule.mode}
            </Text>
          </Text>
        ))}
      </Box>

      {/* Allowed prompts */}
      {allowedPrompts && allowedPrompts.length > 0 && (
        <Box flexDirection="column" marginTop={1}>
          <Text color={colors.primary}>Pending Prompts (from plan approval):</Text>
          {allowedPrompts.map((p, i) => (
            <Text key={i} color={colors.textSecondary}>
              • {p.tool}: {p.prompt}
            </Text>
          ))}
        </Box>
      )}
    </Box>
  );
}

// ============================================================================
// Permission Audit Display
// ============================================================================

interface AuditEntry {
  time: string;
  tool: string;
  input: string;
  decision: string;
  rule?: string;
}

interface PermissionAuditProps {
  entries: AuditEntry[];
}

export function PermissionAuditDisplay({ entries }: PermissionAuditProps) {
  return (
    <Box flexDirection="column">
      <Text color={colors.primary} bold>Recent Permission Decisions</Text>
      <Box flexDirection="column" marginTop={1}>
        {/* Header */}
        <Text color={colors.textMuted}>
          {'Time'.padEnd(8)}{'Decision'.padEnd(11)}{'Tool'.padEnd(12)}Input
        </Text>
        <Text color={colors.textMuted}>{'─'.repeat(60)}</Text>

        {/* Entries */}
        {entries.slice(0, 10).map((entry, i) => {
          const decisionColor =
            entry.decision === 'allowed' || entry.decision === 'confirmed'
              ? colors.success
              : entry.decision === 'denied' || entry.decision === 'rejected'
              ? colors.error
              : colors.warning;

          return (
            <Text key={i}>
              <Text color={colors.textMuted}>{entry.time.padEnd(8)}</Text>
              <Text color={decisionColor}>{entry.decision.padEnd(11)}</Text>
              <Text color={colors.tool}>{entry.tool.padEnd(12)}</Text>
              <Text>{entry.input.slice(0, 30)}</Text>
            </Text>
          );
        })}
      </Box>
    </Box>
  );
}
