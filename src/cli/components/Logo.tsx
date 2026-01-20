import { Box, Text } from 'ink';
import { colors } from './theme.js';

// Small G logo for inline use
export function Logo() {
  return (
    <Box marginRight={1}>
      <Text bold color={colors.brand}>◆</Text>
    </Box>
  );
}

// Compact logo (Claude Code style - 3 lines)
export function BigLogo() {
  // Warm orange brand colors (matching theme.ts)
  const c1 = colors.brand;      // #FF7B54 - Coral/Orange
  const c2 = colors.brandLight; // #FFB38A - Light coral
  const c3 = '#FFDAB8';         // Even lighter coral

  // Compact "G" logo like Claude's
  return (
    <Box flexDirection="column">
      <Text>
        <Text color={c1}> ▐▛███▜▌</Text>
        <Text color={colors.textMuted}>   GenCode</Text>
      </Text>
      <Text>
        <Text color={c2}>▝▜█████▛▘</Text>
      </Text>
      <Text>
        <Text color={c3}>  ▘▘ ▝▝</Text>
      </Text>
    </Box>
  );
}

// Full ASCII art logo (for help screen or special occasions)
export function FullLogo() {
  // Warm orange gradient (matching theme.ts brand colors)
  const c1 = colors.brand;      // #FF7B54 - Coral/Orange
  const c2 = colors.brand;
  const c3 = colors.brandLight; // #FFB38A - Light coral
  const c4 = colors.brandLight;
  const c5 = '#FFDAB8';         // Even lighter coral
  const c6 = '#FFDAB8';
  const c7 = '#FFDAB8';

  return (
    <Box flexDirection="column">
      <Text>
        <Text color={c1}> ██████╗ </Text>
        <Text color={c2}>███████╗</Text>
        <Text color={c3}>███╗   ██╗</Text>
        <Text color={c4}> ██████╗</Text>
        <Text color={c5}> ██████╗ </Text>
        <Text color={c6}>██████╗ </Text>
        <Text color={c7}>███████╗</Text>
      </Text>
      <Text>
        <Text color={c1}>██╔════╝ </Text>
        <Text color={c2}>██╔════╝</Text>
        <Text color={c3}>████╗  ██║</Text>
        <Text color={c4}>██╔════╝</Text>
        <Text color={c5}>██╔═══██╗</Text>
        <Text color={c6}>██╔══██╗</Text>
        <Text color={c7}>██╔════╝</Text>
      </Text>
      <Text>
        <Text color={c1}>██║  ███╗</Text>
        <Text color={c2}>█████╗  </Text>
        <Text color={c3}>██╔██╗ ██║</Text>
        <Text color={c4}>██║     </Text>
        <Text color={c5}>██║   ██║</Text>
        <Text color={c6}>██║  ██║</Text>
        <Text color={c7}>█████╗  </Text>
      </Text>
      <Text>
        <Text color={c1}>██║   ██║</Text>
        <Text color={c2}>██╔══╝  </Text>
        <Text color={c3}>██║╚██╗██║</Text>
        <Text color={c4}>██║     </Text>
        <Text color={c5}>██║   ██║</Text>
        <Text color={c6}>██║  ██║</Text>
        <Text color={c7}>██╔══╝  </Text>
      </Text>
      <Text>
        <Text color={c1}>╚██████╔╝</Text>
        <Text color={c2}>███████╗</Text>
        <Text color={c3}>██║ ╚████║</Text>
        <Text color={c4}>╚██████╗</Text>
        <Text color={c5}>╚██████╔╝</Text>
        <Text color={c6}>██████╔╝</Text>
        <Text color={c7}>███████╗</Text>
      </Text>
      <Text>
        <Text color={c1}> ╚═════╝ </Text>
        <Text color={c2}>╚══════╝</Text>
        <Text color={c3}>╚═╝  ╚═══╝</Text>
        <Text color={c4}> ╚═════╝</Text>
        <Text color={c5}> ╚═════╝ </Text>
        <Text color={c6}>╚═════╝ </Text>
        <Text color={c7}>╚══════╝</Text>
      </Text>
    </Box>
  );
}
