/**
 * Spinner Component - Vivid thinking animation
 */
import { useState, useEffect } from 'react';
import { Box, Text } from 'ink';
import InkSpinner from 'ink-spinner';
import { colors } from './theme.js';

interface SpinnerProps {
  text?: string;
}

export function ThinkingSpinner({ text = 'Thinking' }: SpinnerProps) {
  return (
    <Box marginTop={1} marginBottom={0}>
      <Text color={colors.brand}>
        <InkSpinner type="dots" />
      </Text>
      <Text color={colors.textSecondary}> {text}</Text>
      <Text color={colors.textMuted}> · ctrl+c to stop</Text>
    </Box>
  );
}

export function LoadingSpinner({ text = 'Loading...' }: SpinnerProps) {
  return (
    <Box>
      <Text color={colors.textSecondary}>
        <InkSpinner type="dots" />
      </Text>
      <Text color={colors.textMuted}> {text}</Text>
    </Box>
  );
}


// Format elapsed time
function formatElapsed(ms: number): string {
  const secs = Math.floor(ms / 1000);
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  const remainSecs = secs % 60;
  return `${mins}m ${remainSecs}s`;
}

// Format token count
function formatTokens(count: number): string {
  if (count >= 1000) {
    return `${(count / 1000).toFixed(1)}k`;
  }
  return `${count}`;
}

interface ProgressBarProps {
  startTime?: number;
  tokenCount?: number;
  isThinking?: boolean;
}

// Star animation frames (Claude Code style)
const starFrames = ['✶', '✷', '✸', '✹', '✺', '✹', '✸', '✷'];

/**
 * Progress bar animation for processing state
 * Claude Code style: ✶ Working… (ctrl+c to interrupt · 42s · ↓ 1.1k tokens · thinking)
 */
export function ProgressBar({ startTime, tokenCount = 0, isThinking = false }: ProgressBarProps) {
  const [elapsed, setElapsed] = useState(0);
  const [frame, setFrame] = useState(0);

  useEffect(() => {
    // Star animation
    const animTimer = setInterval(() => {
      setFrame((f) => (f + 1) % starFrames.length);
    }, 100);

    // Update elapsed time every second
    const elapsedTimer = setInterval(() => {
      if (startTime) {
        setElapsed(Date.now() - startTime);
      }
    }, 1000);

    return () => {
      clearInterval(animTimer);
      clearInterval(elapsedTimer);
    };
  }, [startTime]);

  const star = starFrames[frame];

  // Build status parts (Claude style)
  const parts: string[] = ['ctrl+c to interrupt'];
  if (startTime && elapsed > 0) {
    parts.push(formatElapsed(elapsed));
  }
  if (tokenCount > 0) {
    parts.push(`↓ ${formatTokens(tokenCount)} tokens`);
  }
  if (isThinking) {
    parts.push('thinking');
  }

  const statusText = parts.join(' · ');

  return (
    <Box marginTop={1}>
      <Text color={colors.brand}>{star}</Text>
      <Text color={colors.textSecondary}> Inferring…</Text>
      <Text color={colors.textMuted}> ({statusText})</Text>
    </Box>
  );
}
