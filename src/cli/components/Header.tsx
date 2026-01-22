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
  const displayModel = formatModelName(model);
  const displayCwd = formatCwd(cwd);

  // Large letter logo - clean and professional
  return (
    <Box flexDirection="column" marginTop={1}>
      {/* Line 1: Large G */}
      <Box>
        <Text color={colors.brand} bold> ▐▛███▜▌  </Text>
        <Text color={colors.brand} bold>GenCode</Text>
        <Text color={colors.textMuted}> v{VERSION}</Text>
      </Box>
      {/* Line 2 */}
      <Box>
        <Text color={colors.brand} bold>▝▜█████▛▘ </Text>
        <Text color={colors.textMuted}>{displayModel}</Text>
      </Box>
      {/* Line 3 */}
      <Box>
        <Text color={colors.brand} bold>  ▘▘ ▝▝   </Text>
        <Text color={colors.textMuted}>{displayCwd}</Text>
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
