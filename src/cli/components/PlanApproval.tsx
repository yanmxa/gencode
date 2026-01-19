/**
 * Plan Approval Component - Beautiful plan approval UI
 *
 * Displays the implementation plan and asks for user approval with options:
 * 1. Yes, clear context and auto-accept edits (shift+tab)
 * 2. Yes, auto-accept edits
 * 3. Yes, manually approve edits
 * 4. Type here to tell Claude what to change
 *
 * Based on Claude Code's plan approval UI.
 */

import { useState, useEffect } from 'react';
import { Box, Text, useInput } from 'ink';
import { colors, icons } from './theme.js';
import type { PlanApprovalOption, AllowedPrompt } from '../planning/types.js';

// ============================================================================
// Types
// ============================================================================

interface PlanApprovalProps {
  /** Plan summary text */
  planSummary: string;
  /** Requested permissions from the plan */
  requestedPermissions: AllowedPrompt[];
  /** Files to be changed */
  filesToChange: Array<{ path: string; action: 'create' | 'modify' | 'delete' }>;
  /** Plan file path for display */
  planFilePath: string;
  /** Callback when user makes a decision */
  onDecision: (option: PlanApprovalOption, customInput?: string) => void;
}

interface ApprovalOption {
  label: string;
  shortcut: string;
  option: PlanApprovalOption;
  description?: string;
}

// ============================================================================
// Constants
// ============================================================================

const APPROVAL_OPTIONS: ApprovalOption[] = [
  {
    label: 'Yes, clear context and auto-accept edits',
    shortcut: 'shift+tab',
    option: 'approve_clear',
    description: 'Fresh start with automatic approval',
  },
  {
    label: 'Yes, and manually approve edits',
    shortcut: '2',
    option: 'approve_manual_keep',
    description: 'Keep context, review each change',
  },
  {
    label: 'Yes, auto-accept edits',
    shortcut: '3',
    option: 'approve',
    description: 'Continue with automatic approval',
  },
  {
    label: 'Yes, manually approve edits',
    shortcut: '4',
    option: 'approve_manual',
    description: 'Review each change before applying',
  },
  {
    label: 'Type here to tell Claude what to change',
    shortcut: '5',
    option: 'modify',
    description: 'Go back to modify the plan',
  },
];

// ============================================================================
// Helper Components
// ============================================================================

function FileChangesList({
  files,
}: {
  files: Array<{ path: string; action: 'create' | 'modify' | 'delete' }>;
}) {
  if (files.length === 0) return null;

  const getIcon = (action: string) => {
    switch (action) {
      case 'create':
        return { icon: '+', color: colors.success };
      case 'modify':
        return { icon: '~', color: colors.warning };
      case 'delete':
        return { icon: '-', color: colors.error };
      default:
        return { icon: '?', color: colors.textMuted };
    }
  };

  return (
    <Box flexDirection="column" marginTop={1}>
      <Text color={colors.textSecondary}>Files to change:</Text>
      {files.slice(0, 8).map((file, i) => {
        const { icon, color } = getIcon(file.action);
        return (
          <Text key={i}>
            <Text color={color}>  {icon} </Text>
            <Text>{file.path}</Text>
            <Text color={colors.textMuted}> ({file.action})</Text>
          </Text>
        );
      })}
      {files.length > 8 && (
        <Text color={colors.textMuted}>  ... and {files.length - 8} more files</Text>
      )}
    </Box>
  );
}

function PermissionsList({ permissions }: { permissions: AllowedPrompt[] }) {
  if (permissions.length === 0) return null;

  return (
    <Box flexDirection="column" marginTop={1}>
      <Text color={colors.textSecondary}>Requested permissions:</Text>
      {permissions.map((perm, i) => (
        <Text key={i}>
          <Text color={colors.textMuted}>  - </Text>
          <Text color={colors.tool}>{perm.tool}</Text>
          <Text color={colors.textMuted}>(prompt: </Text>
          <Text>{perm.prompt}</Text>
          <Text color={colors.textMuted}>)</Text>
        </Text>
      ))}
    </Box>
  );
}

// ============================================================================
// Plan Approval Component
// ============================================================================

export function PlanApproval({
  planSummary,
  requestedPermissions,
  filesToChange,
  planFilePath,
  onDecision,
}: PlanApprovalProps) {
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [isTyping, setIsTyping] = useState(false);
  const [customInput, setCustomInput] = useState('');

  // Handle keyboard input
  useInput((inputChar, key) => {
    if (isTyping) {
      // Handle typing mode
      if (key.escape) {
        setIsTyping(false);
        setCustomInput('');
      } else if (key.return && customInput.trim()) {
        onDecision('modify', customInput.trim());
      } else if (key.backspace || key.delete) {
        setCustomInput((prev) => prev.slice(0, -1));
      } else if (inputChar && !key.ctrl && !key.meta) {
        setCustomInput((prev) => prev + inputChar);
      }
      return;
    }

    // Arrow navigation
    if (key.upArrow) {
      setSelectedIndex((i) => Math.max(0, i - 1));
    } else if (key.downArrow) {
      setSelectedIndex((i) => Math.min(APPROVAL_OPTIONS.length - 1, i + 1));
    }

    // Enter to select current option
    if (key.return) {
      const selected = APPROVAL_OPTIONS[selectedIndex];
      if (selected.option === 'modify') {
        setIsTyping(true);
      } else {
        onDecision(selected.option);
      }
    }

    // Escape to cancel
    if (key.escape) {
      onDecision('cancel');
    }

    // Shift+Tab for approve_clear
    if (key.shift && key.tab) {
      onDecision('approve_clear');
    }

    // Number shortcuts
    if (inputChar === '1') {
      onDecision('approve_clear');
    } else if (inputChar === '2') {
      onDecision('approve_manual_keep');
    } else if (inputChar === '3') {
      onDecision('approve');
    } else if (inputChar === '4') {
      onDecision('approve_manual');
    } else if (inputChar === '5') {
      setSelectedIndex(4);
      setIsTyping(true);
    }
  });

  // Shorten plan file path
  const displayPath = planFilePath
    .replace(process.env.HOME || '', '~')
    .split('/')
    .slice(-3)
    .join('/');

  return (
    <Box flexDirection="column" borderStyle="round" borderColor={colors.warning} paddingX={1} paddingY={0}>
      {/* Header */}
      <Box marginBottom={1}>
        <Text color={colors.warning} bold>Implementation Plan</Text>
      </Box>

      {/* Plan Summary */}
      <Box flexDirection="column">
        <Text color={colors.text}>{planSummary}</Text>
      </Box>

      {/* Files to change */}
      <FileChangesList files={filesToChange} />

      {/* Requested permissions */}
      <PermissionsList permissions={requestedPermissions} />

      {/* Question */}
      <Box marginTop={1}>
        <Text bold>Would you like to proceed?</Text>
      </Box>

      {/* Options */}
      <Box flexDirection="column" marginTop={0} marginLeft={1}>
        {APPROVAL_OPTIONS.map((opt, index) => {
          const isSelected = index === selectedIndex;
          return (
            <Box key={opt.option}>
              <Text color={isSelected ? colors.primary : colors.textMuted}>
                {isSelected ? icons.radio : icons.radioEmpty}
              </Text>
              <Text color={colors.textMuted}> {String(index + 1)}. </Text>
              <Text color={isSelected ? colors.text : colors.textSecondary}>
                {opt.label}
              </Text>
              {opt.shortcut !== String(index + 1) && (
                <Text color={colors.textMuted}> ({opt.shortcut})</Text>
              )}
            </Box>
          );
        })}
      </Box>

      {/* Typing mode */}
      {isTyping && (
        <Box marginTop={1} marginLeft={1}>
          <Text color={colors.primary}>{icons.prompt} </Text>
          <Text>{customInput}</Text>
          <Text color={colors.primary}>{icons.cursor}</Text>
        </Box>
      )}

      {/* Footer hint */}
      <Box marginTop={1}>
        <Text color={colors.textMuted}>
          ctrl-g to edit in VS Code · {displayPath}
        </Text>
      </Box>
    </Box>
  );
}

// ============================================================================
// Plan Mode Toggle Notification
// ============================================================================

interface ModeChangeNotificationProps {
  fromMode: 'build' | 'plan';
  toMode: 'build' | 'plan';
  planFilePath?: string;
}

export function ModeChangeNotification({
  fromMode,
  toMode,
  planFilePath,
}: ModeChangeNotificationProps) {
  const isPlanMode = toMode === 'plan';
  const displayPath = planFilePath
    ? planFilePath.replace(process.env.HOME || '', '~').split('/').slice(-2).join('/')
    : '';

  return (
    <Box
      flexDirection="column"
      borderStyle="round"
      borderColor={isPlanMode ? colors.warning : colors.success}
      paddingX={1}
    >
      <Box>
        <Text color={colors.textMuted}>{icons.radioEmpty} {fromMode.toUpperCase()}</Text>
        <Text color={colors.textMuted}>  {icons.arrow}  </Text>
        <Text color={isPlanMode ? colors.warning : colors.success} bold>
          {icons.radio} {toMode.toUpperCase()}
        </Text>
      </Box>

      <Box>
        {isPlanMode ? (
          <Text color={colors.warning}>Switched to PLAN mode</Text>
        ) : (
          <Text color={colors.success}>Switched to BUILD mode</Text>
        )}
      </Box>

      {isPlanMode && displayPath && (
        <Box>
          <Text color={colors.textMuted}>Plan file: </Text>
          <Text color={colors.primary}>{displayPath}</Text>
        </Box>
      )}
    </Box>
  );
}
