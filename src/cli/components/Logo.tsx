import { Box, Text } from 'ink';

export function Logo() {
  // Full G with 3D shadow - subdued slate color
  const slateColor = "#64748B"; // Slate 500 - stable, professional
  return (
    <Box flexDirection="column" marginRight={1}>
      <Text color={slateColor}>  ██████╗ </Text>
      <Text color={slateColor}> ██╔════╝ </Text>
      <Text color={slateColor}> ██║  ███╗</Text>
      <Text color={slateColor}> ██║   ██║</Text>
      <Text color={slateColor}> ╚██████╔╝</Text>
      <Text color={slateColor}>  ╚═════╝ </Text>
    </Box>
  );
}
