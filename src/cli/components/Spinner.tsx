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

// ============================================================================
// Vivid Animation System - Lively and Playful
// ============================================================================

// Flowing wave animation - smooth braille patterns
const waveFrames = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'];

// Bouncing dots - playful rhythm
const bounceFrames = ['⠁', '⠂', '⠄', '⠂', '⠁', '⠈', '⠐', '⠈'];

// Sparkle effect - twinkling magic
const sparkleFrames = ['✦', '✧', '⋆', '✧', '✦', '★', '✦', '⋆', '·', '⋆'];

// Orbit animation - spinning particles
const orbitFrames = ['◜', '◠', '◝', '◞', '◡', '◟'];

// DNA helix - complex reasoning visualization
const helixFrames = ['⣾', '⣽', '⣻', '⢿', '⡿', '⣟', '⣯', '⣷'];

// Pulse with glow effect
const glowFrames = ['○', '◎', '●', '◉', '●', '◎', '○', '◌'];

// Animated ellipsis patterns
const ellipsisFrames = [
  '.    ',
  '..   ',
  '...  ',
  ' ... ',
  '  ...',
  '   ..',
  '    .',
  '     ',
];

// Letter-by-letter highlight effect for status text
const highlightPatterns = [0, 1, 2, 3, 4, 5, 6, 7, 6, 5, 4, 3, 2, 1];

/**
 * Get animation frames based on processing state
 */
function getAnimationFrames(state: ProcessingState): string[] {
  switch (state) {
    case 'thinking':
      return waveFrames;
    case 'inferring':
      return helixFrames;
    case 'generating':
      return sparkleFrames;
    case 'tool_waiting':
      return orbitFrames;
    case 'processing':
    default:
      return glowFrames;
  }
}

/**
 * Get dynamic status text based on processing state
 */
function getStatusText(state: ProcessingState, toolName?: string): string {
  switch (state) {
    case 'thinking':
      return 'Thinking';
    case 'inferring':
      return 'Reasoning';
    case 'generating':
      return 'Generating';
    case 'tool_waiting':
      return toolName ? `Running ${toolName}` : 'Executing';
    case 'processing':
    default:
      return 'Working';
  }
}

/**
 * Create wave-animated text with highlighted character
 */
function createWaveText(text: string, tick: number, baseColor: string, highlightColor: string): Array<{ char: string; color: string }> {
  const highlightIndex = highlightPatterns[tick % highlightPatterns.length] % text.length;
  return text.split('').map((char, i) => ({
    char,
    color: i === highlightIndex ? highlightColor : baseColor,
  }));
}

/**
 * Animated Text Component - Renders text with wave highlight effect
 */
function AnimatedText({ text, tick, baseColor, highlightColor }: {
  text: string;
  tick: number;
  baseColor: string;
  highlightColor: string;
}) {
  const chars = createWaveText(text, tick, baseColor, highlightColor);
  return (
    <>
      {chars.map((c, i) => (
        <Text key={i} color={c.color}>{c.char}</Text>
      ))}
    </>
  );
}

/**
 * Progress bar animation for processing state
 * GenCode style: ⠋ Thinking... (ctrl+c to interrupt · 42s · ↓ 1.1k tokens)
 * Features: Dynamic spinner, animated text, flowing ellipsis
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
  const [ellipsisFrame, setEllipsisFrame] = useState(0);

  useEffect(() => {
    // Main spinner animation - fast and smooth
    const animTimer = setInterval(() => {
      setFrame((f) => (f + 1) % 10);
    }, 80);

    // Ellipsis animation - slightly slower
    const ellipsisTimer = setInterval(() => {
      setEllipsisFrame((f) => (f + 1) % ellipsisFrames.length);
    }, 150);

    // Tick counter for text animation and elapsed time
    const tickTimer = setInterval(() => {
      setTick((t) => t + 1);
    }, 100);

    return () => {
      clearInterval(animTimer);
      clearInterval(ellipsisTimer);
      clearInterval(tickTimer);
    };
  }, []);

  // Determine display state
  const displayState: ProcessingState = isThinking ? 'thinking' : state;

  // Get animation frames for current state
  const frames = getAnimationFrames(displayState);
  const spinner = frames[frame % frames.length];

  // Get status text
  const mainText = getStatusText(displayState, toolName);

  // Animated ellipsis
  const ellipsis = ellipsisFrames[ellipsisFrame];

  // Compute elapsed time
  const elapsed = startTime ? Date.now() - startTime : 0;
  void tick; // Force re-render

  // Build status parts
  const parts: string[] = ['ctrl+c to interrupt'];
  if (startTime) {
    parts.push(formatElapsed(elapsed));
  }
  if (tokenCount > 0) {
    parts.push(`↓ ${formatTokens(tokenCount)} tokens`);
  }

  const statusInfo = parts.join(' · ');

  return (
    <Box marginTop={1}>
      <Text color={colors.brand}>{spinner}</Text>
      <Text> </Text>
      <AnimatedText
        text={mainText}
        tick={tick}
        baseColor={colors.textSecondary}
        highlightColor={colors.brand}
      />
      <Text color={colors.textMuted}>{ellipsis}</Text>
      <Text color={colors.textMuted}> ({statusInfo})</Text>
    </Box>
  );
}
