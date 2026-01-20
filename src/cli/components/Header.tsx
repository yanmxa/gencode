import { Box, Text } from 'ink';
import { colors } from './theme.js';

// Get version from package.json (injected at build time or read dynamically)
const VERSION = '0.5.0';

interface HeaderProps {
  provider: string;
  model: string;
  cwd: string;
  contextStats?: {
    activeMessages: number;
    totalMessages: number;
    usagePercent: number;
  };
}

// Format model name to be more readable (Claude Code style)
function formatModelName(model: string): string {
  // Convert "claude-opus-4-5@20251101" to "Opus 4.5"
  if (model.includes('opus')) return 'Opus 4.5';
  if (model.includes('sonnet')) return 'Sonnet 4';
  if (model.includes('haiku')) return 'Haiku 4';
  if (model.includes('gpt-4')) return 'GPT-4';
  if (model.includes('gpt-3.5')) return 'GPT-3.5';
  if (model.includes('gemini')) return 'Gemini';
  return model;
}

// Shorten path for display
function formatCwd(cwd: string): string {
  const home = process.env.HOME || '';
  if (home && cwd.startsWith(home)) {
    return '~' + cwd.slice(home.length);
  }
  return cwd;
}

export function Header({ model, cwd, contextStats }: HeaderProps) {
  // Warm orange brand colors (matching theme.ts)
  const c1 = colors.brand;      // #FF7B54 - Coral/Orange
  const c2 = colors.brandLight; // #FFB38A - Light coral
  const c3 = '#FFDAB8';         // Even lighter coral

  const displayModel = formatModelName(model);
  const displayCwd = formatCwd(cwd);

  return (
    <Box flexDirection="column" marginTop={1}>
      {/* Claude Code style: logo with info on right */}
      <Box>
        <Text color={c1}> ▐▛███▜▌</Text>
        <Text color={colors.textMuted}>   GenCode v{VERSION}</Text>
      </Box>
      <Box>
        <Text color={c2}>▝▜█████▛▘</Text>
        <Text color={colors.textMuted}>  {displayModel}</Text>
      </Box>
      <Box>
        <Text color={c3}>  ▘▘ ▝▝</Text>
        <Text color={colors.textMuted}>    {displayCwd}</Text>
      </Box>

      {contextStats && contextStats.activeMessages > 0 && (
        <Box marginTop={1}>
          <Text color={colors.textSecondary}>
            Context: {contextStats.activeMessages}/{contextStats.totalMessages} msgs
          </Text>
          <Text color={colors.textMuted}> ({Math.round(contextStats.usagePercent)}%)</Text>
        </Box>
      )}
    </Box>
  );
}

export function Welcome() {
  return (
    <Box marginTop={1}>
      <Text color={colors.textMuted}>? for shortcuts</Text>
    </Box>
  );
}
