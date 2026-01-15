import { Box, Text } from 'ink';
import { colors } from './theme.js';

interface Command {
  name: string;
  description: string;
}

export const COMMANDS: Command[] = [
  { name: '/model', description: 'Switch model' },
  { name: '/provider', description: 'Manage providers' },
  { name: '/sessions', description: 'List sessions' },
  { name: '/resume', description: 'Resume session' },
  { name: '/new', description: 'New session' },
  { name: '/save', description: 'Save session' },
  { name: '/clear', description: 'Clear chat' },
  { name: '/info', description: 'Session info' },
  { name: '/help', description: 'Show help' },
  { name: '/init', description: 'Generate AGENT.md' },
  { name: '/memory', description: 'Show memory files' },
];

interface CommandSuggestionsProps {
  input: string;
  selectedIndex: number;
}

export function CommandSuggestions({ input, selectedIndex }: CommandSuggestionsProps) {
  // Filter commands matching input
  const prefix = input.toLowerCase();
  const suggestions = COMMANDS.filter((cmd) =>
    cmd.name.toLowerCase().startsWith(prefix)
  );

  if (suggestions.length === 0) {
    return null;
  }

  return (
    <Box flexDirection="column" marginLeft={2}>
      {suggestions.map((cmd, i) => {
        const isSelected = i === selectedIndex;
        return (
          <Box key={cmd.name}>
            <Text color={isSelected ? colors.primary : colors.textSecondary}>
              {cmd.name.padEnd(12)}
            </Text>
            <Text color={colors.textMuted}>{cmd.description}</Text>
          </Box>
        );
      })}
    </Box>
  );
}

export function getFilteredCommands(input: string): Command[] {
  const prefix = input.toLowerCase();
  return COMMANDS.filter((cmd) => cmd.name.toLowerCase().startsWith(prefix));
}
