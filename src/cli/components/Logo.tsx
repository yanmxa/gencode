import { Box, Text } from 'ink';
import { colors } from './theme.js';

// Small icon for inline use
export function Logo() {
  return (
    <Box marginRight={1}>
      <Text bold color={colors.brand}>G</Text>
    </Box>
  );
}

// Large letter logo (3 lines) - matching Claude Code style
export function BigLogo() {
  return (
    <Box flexDirection="column">
      <Text>
        <Text color={colors.brand} bold> ▐▛███▜▌</Text>
      </Text>
      <Text>
        <Text color={colors.brand} bold>▝▜█████▛▘</Text>
      </Text>
      <Text>
        <Text color={colors.brand} bold>  ▘▘ ▝▝</Text>
      </Text>
    </Box>
  );
}

// Full logo for help screen
export function FullLogo() {
  return (
    <Box flexDirection="column">
      <Text>
        <Text color={colors.brand} bold> ▐▛███▜▌  </Text>
        <Text color={colors.brand} bold>GenCode</Text>
      </Text>
      <Text>
        <Text color={colors.brand} bold>▝▜█████▛▘ </Text>
        <Text color={colors.textMuted}>AI Terminal</Text>
      </Text>
      <Text>
        <Text color={colors.brand} bold>  ▘▘ ▝▝</Text>
      </Text>
      <Text> </Text>
      <Text>
        <Text color={colors.textMuted}>  ╭─────────────────╮</Text>
      </Text>
      <Text>
        <Text color={colors.textMuted}>  │ </Text>
        <Text color={colors.brand}>Swift</Text>
        <Text color={colors.textMuted}> • </Text>
        <Text color={colors.info}>Smart</Text>
        <Text color={colors.textMuted}> • </Text>
        <Text color={colors.success}>Strong</Text>
        <Text color={colors.textMuted}> │</Text>
      </Text>
      <Text>
        <Text color={colors.textMuted}>  ╰─────────────────╯</Text>
      </Text>
    </Box>
  );
}
