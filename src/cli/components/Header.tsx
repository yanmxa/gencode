import { Box, Text } from 'ink';
import { colors } from './theme.js';
import { BigLogo } from './Logo.js';

interface HeaderProps {
  provider: string;
  model: string;
  cwd: string;
}

export function Header({ model, cwd }: HeaderProps) {
  return (
    <Box flexDirection="column" marginTop={1}>
      <BigLogo />
      <Box marginTop={1}>
        <Text color={colors.textSecondary}>{model}</Text>
        <Text color={colors.textMuted}> · </Text>
        <Text color={colors.textMuted}>{cwd}</Text>
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
