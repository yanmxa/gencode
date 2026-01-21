/**
 * Spinner Component - GenCode thinking animation
 * Features unique animation and dynamic status text
 */
import { useState, useEffect } from 'react';
import { Box, Text } from 'ink';
import InkSpinner from 'ink-spinner';
import { colors } from './theme.js';

// Processing state types for dynamic status text
export type ProcessingState =
  | 'thinking'      // Initial processing
  | 'inferring'     // Active LLM inference
  | 'generating'    // Producing text output
  | 'tool_waiting'  // Waiting for tool to complete
  | 'processing';   // Generic state

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
  state?: ProcessingState;    // Processing state for dynamic text
  toolName?: string;          // Tool name for "Running Bash..."
}

// GenCode pulse animation (unique identity - different from Claude's stars)
const pulseFrames = ['⦿', '⦾', '◉', '◎', '◉', '⦾', '⦿', '○'];

/**
 * Get dynamic status text based on processing state
 */
function getStatusText(state: ProcessingState, toolName?: string): string {
  switch (state) {
    case 'thinking':
      return 'Thinking...';
    case 'inferring':
      return 'Reasoning...';
    case 'generating':
      return 'Generating...';
    case 'tool_waiting':
      return toolName ? `Running ${toolName}...` : 'Executing...';
    case 'processing':
    default:
      return 'Working...';
  }
}

/**
 * Progress bar animation for processing state
 * GenCode style: ⦿ Thinking... (ctrl+c to interrupt · 42s · ↓ 1.1k tokens)
 */
export function ProgressBar({
  startTime,
  tokenCount = 0,
  isThinking = false,
  state = 'inferring',
  toolName,
}: ProgressBarProps) {
  const [tick, setTick] = useState(0);
  const [frame, setFrame] = useState(0);

  useEffect(() => {
    // Pulse animation
    const animTimer = setInterval(() => {
      setFrame((f) => (f + 1) % pulseFrames.length);
    }, 120); // Slightly slower for smoother pulse effect

    // Update tick counter every second to trigger re-render for elapsed time
    const elapsedTimer = setInterval(() => {
      setTick((t) => t + 1);
    }, 1000);

    return () => {
      clearInterval(animTimer);
      clearInterval(elapsedTimer);
    };
  }, []); // Empty dependency array - timers run for component lifetime

  const pulse = pulseFrames[frame];

  // Compute elapsed directly from startTime on each render (triggered by tick updates)
  const elapsed = startTime ? Date.now() - startTime : 0;
  // Use tick to satisfy ESLint (and force re-render)
  void tick;

  // Determine display state - use isThinking for backward compatibility
  const displayState: ProcessingState = isThinking ? 'thinking' : state;
  const mainText = getStatusText(displayState, toolName);

  // Build status parts
  const parts: string[] = ['ctrl+c to interrupt'];
  // Always show elapsed time when we have a startTime
  if (startTime) {
    parts.push(formatElapsed(elapsed));
  }
  if (tokenCount > 0) {
    parts.push(`↓ ${formatTokens(tokenCount)} tokens`);
  }

  const statusInfo = parts.join(' · ');

  return (
    <Box marginTop={1}>
      <Text color={colors.brand}>{pulse}</Text>
      <Text color={colors.textSecondary}> {mainText}</Text>
      <Text color={colors.textMuted}> ({statusInfo})</Text>
    </Box>
  );
}
