/**
 * Spinner Component - Vivid thinking animation
 */
import { useState, useEffect, useMemo } from 'react';
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

// Thinking phrases that rotate during processing
const thinkingPhrases = [
  'Thinking',
  'Pondering',
  'Analyzing',
  'Processing',
  'Reasoning',
  'Contemplating',
  'Figuring out',
  'Working on it',
  'Almost there',
  'Crafting response',
];

// Animation frames for different styles
const animations = {
  // Brainwave animation
  brainwave: ['🧠 ∿∿∿', '🧠∿ ∿∿', '🧠∿∿ ∿', '🧠∿∿∿ ', '🧠 ∿∿∿', '🧠∿ ∿∿'],
  // Sparkle animation
  sparkle: ['✨    ', ' ✨   ', '  ✨  ', '   ✨ ', '    ✨', '   ✨ ', '  ✨  ', ' ✨   '],
  // DNA helix
  dna: ['🔬 ⌬⌬⌬', '🔬⌬ ⌬⌬', '🔬⌬⌬ ⌬', '🔬⌬⌬⌬ ', '🔬 ⌬⌬⌬'],
  // Pulse dots
  pulse: ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'],
  // Wave animation
  wave: ['≋≈∼∽', '∽≋≈∼', '∼∽≋≈', '≈∼∽≋'],
  // Bounce bar with gradient
  bounceGradient: [
    '█▓▒░    ',
    ' █▓▒░   ',
    '  █▓▒░  ',
    '   █▓▒░ ',
    '    █▓▒░',
    '   ░▒▓█ ',
    '  ░▒▓█  ',
    ' ░▒▓█   ',
    '░▒▓█    ',
  ],
  // Orbit animation
  orbit: ['◐', '◓', '◑', '◒'],
  // Loading bar with shimmer
  shimmer: [
    '▓▓▓▓▓░░░',
    '░▓▓▓▓▓░░',
    '░░▓▓▓▓▓░',
    '░░░▓▓▓▓▓',
    '░░░░▓▓▓▓',
    '░░░▓▓▓▓▓',
    '░░▓▓▓▓▓░',
    '░▓▓▓▓▓░░',
  ],
};

type AnimationType = keyof typeof animations;
const animationTypes = Object.keys(animations) as AnimationType[];

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

/**
 * Progress bar animation for processing state
 * Claude Code style with time, tokens, and thinking status
 */
export function ProgressBar({ startTime, tokenCount = 0, isThinking = false }: ProgressBarProps) {
  const [frame, setFrame] = useState(0);
  const [phraseIndex, setPhraseIndex] = useState(0);
  const [elapsed, setElapsed] = useState(0);

  // Pick a random animation style on mount
  const animStyle = useMemo(() => {
    const randomIndex = Math.floor(Math.random() * animationTypes.length);
    return animationTypes[randomIndex];
  }, []);

  const currentAnim = animations[animStyle];

  useEffect(() => {
    // Fast animation update
    const animTimer = setInterval(() => {
      setFrame((f) => (f + 1) % currentAnim.length);
    }, 120);

    // Slower phrase rotation (every 2.5 seconds)
    const phraseTimer = setInterval(() => {
      setPhraseIndex((p) => (p + 1) % thinkingPhrases.length);
    }, 2500);

    // Update elapsed time every second
    const elapsedTimer = setInterval(() => {
      if (startTime) {
        setElapsed(Date.now() - startTime);
      }
    }, 1000);

    return () => {
      clearInterval(animTimer);
      clearInterval(phraseTimer);
      clearInterval(elapsedTimer);
    };
  }, [currentAnim.length, startTime]);

  const animFrame = currentAnim[frame];
  const phrase = thinkingPhrases[phraseIndex];

  // Animated ellipsis
  const ellipsis = '.'.repeat((frame % 3) + 1).padEnd(3, ' ');

  // Build status parts
  const parts: string[] = [];
  if (startTime && elapsed > 0) {
    parts.push(formatElapsed(elapsed));
  }
  if (tokenCount > 0) {
    parts.push(`↓ ${formatTokens(tokenCount)} tokens`);
  }
  if (isThinking) {
    parts.push('thinking');
  }

  const statusText = parts.length > 0 ? ` · ${parts.join(' · ')}` : '';

  return (
    <Box>
      <Text color={colors.brand}>{animFrame}</Text>
      <Text color={colors.textSecondary}> {phrase}</Text>
      <Text color={colors.textMuted}>{ellipsis}</Text>
      <Text color={colors.textMuted}>(esc to stop{statusText})</Text>
    </Box>
  );
}
