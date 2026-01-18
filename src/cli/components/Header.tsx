import { Box, Text } from 'ink';
import { colors } from './theme.js';
import { BigLogo } from './Logo.js';

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

export function Header({ model, cwd, contextStats }: HeaderProps) {
  return (
    <Box flexDirection="column" marginTop={1}>
      <BigLogo />
      <Box marginTop={1}>
        <Text color={colors.textSecondary}>{model}</Text>
        <Text color={colors.textMuted}> · </Text>
        <Text color={colors.textMuted}>{cwd}</Text>

        {contextStats && contextStats.activeMessages > 0 && (
          <>
            <Text color={colors.textMuted}> · </Text>
            <Text color={colors.textSecondary}>
              Context: {contextStats.activeMessages}/{contextStats.totalMessages} msgs
            </Text>
            <Text color={colors.textMuted}> ({Math.round(contextStats.usagePercent)}%)</Text>
          </>
        )}
      </Box>
    </Box>
  );
}

export function Welcome() {
  return (
    <Box marginTop={1}>
      <Text color={colors.textMuted}>? for help · Ctrl+C to exit</Text>
    </Box>
  );
}
