/**
 * Permission Prompt Component - Claude Code style approval UI
 *
 * Claude Code design:
 *
 * Tool use
 * Web Search("query text here")
 *
 * Do you want to proceed?
 *   [1] Yes
 *   [2] Yes, and don't ask again for Web Search in /path/to/project
 *   [3] No
 */

import { useState, useEffect } from 'react';
import { Box, Text, useInput } from 'ink';
import { colors, icons } from './theme.js';
import type { ApprovalAction, ApprovalSuggestion } from '../../permissions/types.js';

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
 */
function formatInput(tool: string, input: Record<string, unknown>): string {
  switch (tool) {
    case 'Bash':
      return (input.command as string) ?? JSON.stringify(input);

    case 'Read':
    case 'Write':
    case 'Edit':
      return (input.file_path as string) ?? (input.path as string) ?? '';

    case 'Glob':
      return `${input.pattern ?? ''} in ${input.path ?? '.'}`;

    case 'Grep':
      return `${input.pattern ?? ''} in ${input.path ?? '.'}`;

    case 'WebFetch':
      return (input.url as string) ?? '';

    case 'WebSearch':
      return (input.query as string) ?? '';

    default:
      const str = JSON.stringify(input);
      return str.length > 200 ? str.slice(0, 197) + '...' : str;
  }
}

/**
 * Format tool call in Claude Code style: Tool("input")
 */
function formatToolCall(tool: string, input: Record<string, unknown>): string {
  const displayName = formatToolName(tool);
  const inputStr = formatInput(tool, input);
  return `${displayName}("${inputStr}")`;
}

/**
 * Get icon for tool
 */
function getToolIcon(tool: string): string {
  switch (tool) {
    case 'Bash':
      return '⚡';
    case 'Read':
    case 'Write':
    case 'Edit':
      return '📄';
    case 'Glob':
    case 'Grep':
      return '🔍';
    case 'WebFetch':
    case 'WebSearch':
      return '🌐';
    default:
      return icons.tool;
  }
}

/**
 * Get shortcut key display
 */
function getShortcutDisplay(shortcut?: string): string {
  if (!shortcut) return '';
  if (shortcut === 'n') return 'n';
  return shortcut;
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
}: PermissionPromptProps) {
  const [selectedIndex, setSelectedIndex] = useState(0);

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

  const toolCall = formatToolCall(tool, input);
  const displayToolName = formatToolName(tool);

  // Get dynamic label for "don't ask again" option
  const getDynamicLabel = (suggestion: ApprovalSuggestion): string => {
    if (suggestion.action === 'allow_always' && projectPath) {
      return `Yes, and don't ask again for ${displayToolName} in ${projectPath}`;
    }
    return suggestion.label;
  };

  return (
    <Box flexDirection="column" marginTop={1}>
      {/* Header - Claude Code style */}
      <Box>
        <Text color={colors.textMuted}>Tool use</Text>
      </Box>

      {/* Tool call display */}
      <Box marginTop={0}>
        <Text color={colors.tool}>{toolCall}</Text>
      </Box>

      {/* Question */}
      <Box marginTop={1}>
        <Text>Do you want to proceed?</Text>
      </Box>

      {/* Options - Claude Code style */}
      <Box flexDirection="column" marginTop={1} paddingLeft={2}>
        {suggestions.map((suggestion, index) => {
          const isSelected = index === selectedIndex;
          const shortcut = getShortcutDisplay(suggestion.shortcut);
          const label = getDynamicLabel(suggestion);

          return (
            <Box key={suggestion.action}>
              <Text color={isSelected ? colors.primary : colors.textMuted}>
                {isSelected ? '>' : ' '}
              </Text>
              <Text> </Text>
              <Text color={colors.textMuted}>[{shortcut}] </Text>
              <Text color={isSelected ? colors.text : colors.textSecondary}>
                {label}
              </Text>
            </Box>
          );
        })}
      </Box>
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
