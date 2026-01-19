/**
 * Mode Indicator Component - Shows current mode (NORMAL/PLAN/ACCEPT)
 *
 * Design:
 * ╭────────────────────────────────────╮
 * │  ◉ NORMAL   ○ PLAN   ○ ACCEPT     │
 * ╰────────────────────────────────────╯
 *
 * Or with Shift+Tab toggle hint
 */

import { Box, Text } from 'ink';
import { colors, icons } from './theme.js';
import type { ModeType } from '../planning/types.js';

// ============================================================================
// Types
// ============================================================================

interface ModeIndicatorProps {
  /** Current mode */
  mode: ModeType;
  /** Show toggle hint */
  showHint?: boolean;
  /** Compact mode (inline) */
  compact?: boolean;
}

// ============================================================================
// Mode Indicator Component
// ============================================================================

/**
 * Mode Indicator - Shows NORMAL/PLAN/ACCEPT mode toggle
 */
export function ModeIndicator({ mode, showHint = false, compact = false }: ModeIndicatorProps) {
  const isNormal = mode === 'normal';
  const isPlan = mode === 'plan';
  const isAccept = mode === 'accept';

  if (compact) {
    // Compact inline indicator
    const modeLabel = isPlan ? 'PLAN' : isAccept ? 'ACCEPT' : '';
    const modeColor = isPlan ? colors.warning : isAccept ? colors.success : colors.textMuted;

    if (!modeLabel) return null;

    return (
      <Box>
        <Text color={modeColor}>
          {isPlan ? icons.modePlan : icons.modeAccept}
        </Text>
        <Text color={modeColor}> {modeLabel}</Text>
      </Box>
    );
  }

  return (
    <Box flexDirection="column">
      <Box borderStyle="round" borderColor={isPlan ? colors.warning : isAccept ? colors.success : colors.textMuted} paddingX={1}>
        {/* NORMAL option */}
        <Text color={isNormal ? colors.primary : colors.textMuted}>
          {isNormal ? icons.radio : icons.radioEmpty}
        </Text>
        <Text color={isNormal ? colors.text : colors.textMuted} bold={isNormal}>
          {' '}NORMAL
        </Text>

        <Text color={colors.textMuted}>   </Text>

        {/* PLAN option */}
        <Text color={isPlan ? colors.warning : colors.textMuted}>
          {isPlan ? icons.radio : icons.radioEmpty}
        </Text>
        <Text color={isPlan ? colors.text : colors.textMuted} bold={isPlan}>
          {' '}PLAN
        </Text>

        <Text color={colors.textMuted}>   </Text>

        {/* ACCEPT option */}
        <Text color={isAccept ? colors.success : colors.textMuted}>
          {isAccept ? icons.radio : icons.radioEmpty}
        </Text>
        <Text color={isAccept ? colors.text : colors.textMuted} bold={isAccept}>
          {' '}ACCEPT
        </Text>
      </Box>

      {showHint && (
        <Box marginTop={0}>
          <Text color={colors.textMuted} dimColor>  Shift+Tab to cycle modes</Text>
        </Box>
      )}
    </Box>
  );
}

// ============================================================================
// Mode Badge Component
// ============================================================================

interface ModeBadgeProps {
  mode: ModeType;
}

/**
 * Mode Badge - Compact badge for header display
 */
export function ModeBadge({ mode }: ModeBadgeProps) {
  const color = mode === 'plan' ? colors.warning : mode === 'accept' ? colors.success : colors.primary;
  const label = mode === 'plan' ? 'PLAN' : mode === 'accept' ? 'ACCEPT' : 'NORMAL';

  return (
    <Text color={color} bold>
      [{label}]
    </Text>
  );
}

// ============================================================================
// Plan Status Bar Component
// ============================================================================

interface PlanStatusBarProps {
  /** Current planning phase */
  phase: string;
  /** Plan file path */
  planFilePath?: string;
}

/**
 * Plan Status Bar - Shows plan mode status
 */
export function PlanStatusBar({ phase, planFilePath }: PlanStatusBarProps) {
  // Shorten plan file path for display
  const displayPath = planFilePath
    ? planFilePath.replace(process.env.HOME || '', '~').split('/').slice(-2).join('/')
    : '';

  return (
    <Box
      borderStyle="round"
      borderColor={colors.warning}
      paddingX={1}
      flexDirection="column"
    >
      {/* Status line */}
      <Box>
        <Text color={colors.warning}>PLAN MODE</Text>
        <Text color={colors.textMuted}> │ </Text>
        <Text color={colors.textSecondary}>Phase: </Text>
        <Text color={colors.info}>{phase}</Text>
        <Text color={colors.textMuted}> │ </Text>
        <Text color={colors.textMuted}>Shift+Tab to switch</Text>
      </Box>

      {/* Tools info */}
      <Box>
        <Text color={colors.textMuted}>
          Allowed: Read, Glob, Grep, WebFetch, WebSearch, TodoWrite
        </Text>
      </Box>

      {/* Plan file */}
      {displayPath && (
        <Box>
          <Text color={colors.textMuted}>Plan: </Text>
          <Text color={colors.primary}>{displayPath}</Text>
        </Box>
      )}
    </Box>
  );
}
