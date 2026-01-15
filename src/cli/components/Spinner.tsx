/**
 * Spinner Component - Compact thinking animation (Claude Code style)
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

/**
 * Progress bar animation for processing state
 * Bouncing ball with trail effect
 */
export function ProgressBar() {
  const [frame, setFrame] = useState(0);

  useEffect(() => {
    const timer = setInterval(() => {
      setFrame((f) => (f + 1) % 14);
    }, 100);
    return () => clearInterval(timer);
  }, []);

  // Bouncing ball animation: ball moves left-right with trail
  const width = 7;
  // Frame 0-6: left to right, 7-13: right to left
  const pos = frame < 7 ? frame : 13 - frame;

  let bar = '';
  for (let i = 0; i < width; i++) {
    if (i === pos) {
      bar += '●';
    } else if (i === pos - 1 || i === pos + 1) {
      bar += '○';
    } else {
      bar += '·';
    }
  }

  return (
    <Box>
      <Text color={colors.brand}>{bar}</Text>
      <Text color={colors.textMuted}> esc to stop</Text>
    </Box>
  );
}
