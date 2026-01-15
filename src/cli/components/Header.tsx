import { Box, Text } from 'ink';
import { colors } from './theme.js';
import { Logo } from './Logo.js';

interface HeaderProps {
  provider: string;
  model: string;
  cwd: string;
}

export function Header({ provider, model, cwd }: HeaderProps) {
  const home = process.env.HOME || '';
  const cwdDisplay = cwd.startsWith(home) ? '~' + cwd.slice(home.length) : cwd;

  return (
    <Box flexDirection="row" marginBottom={1} marginTop={1}>
      <Logo />
      <Box flexDirection="column" marginLeft={1}>
        <Box>
          <Text bold color={colors.text}>gencode </Text>
          <Text color={colors.textMuted}>v0.1.0</Text>
        </Box>
        <Text color={colors.textMuted}>{model} · API Usage Billing</Text>
        <Text color={colors.textMuted}>{cwdDisplay}</Text>
      </Box>
    </Box>
  );
}

export function Welcome() {
  return (
    <Text color={colors.textMuted}>
      Type a message or /help. Ctrl+C to exit.
    </Text>
  );
}
