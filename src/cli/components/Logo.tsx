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

// Large ASCII art logo with elegant gradient
export function BigLogo() {
  // Indigo gradient - brand colors
  const c1 = '#818CF8'; // Indigo 400
  const c2 = '#818CF8'; // Indigo 400
  const c3 = '#A5B4FC'; // Indigo 300
  const c4 = '#A5B4FC'; // Indigo 300
  const c5 = '#C7D2FE'; // Indigo 200
  const c6 = '#C7D2FE'; // Indigo 200
  const c7 = '#C7D2FE'; // Indigo 200

  // G E N C O D E
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
