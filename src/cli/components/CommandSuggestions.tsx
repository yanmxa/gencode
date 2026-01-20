import { Box, Text } from 'ink';
import { colors } from './theme.js';

interface Command {
  name: string;
  description: string;
  argumentHint?: string;
}

interface CommandMatch {
  command: Command;
  score: number;
}

// Built-in commands
export const BUILTIN_COMMANDS: Command[] = [
  { name: '/plan', description: 'Enter plan mode (Shift+Tab to cycle)' },
  { name: '/normal', description: 'Exit to normal mode' },
  { name: '/accept', description: 'Enter auto-accept mode' },
  { name: '/model', description: 'Switch model' },
  { name: '/provider', description: 'Manage providers' },
  { name: '/permissions', description: 'View permission rules' },
  { name: '/permissions audit', description: 'View permission audit log' },
  { name: '/sessions', description: 'List sessions' },
  { name: '/tasks', description: 'List background tasks' },
  { name: '/resume', description: 'Resume session' },
  { name: '/new', description: 'New session' },
  { name: '/save', description: 'Save session' },
  { name: '/clear', description: 'Clear chat' },
  { name: '/info', description: 'Session info' },
  { name: '/help', description: 'Show help' },
  { name: '/init', description: 'Generate AGENT.md' },
  { name: '/memory', description: 'Show memory files' },
  { name: '/changes', description: 'List file changes' },
  { name: '/rewind', description: 'Undo file changes' },
  { name: '/context', description: 'Show context usage stats' },
  { name: '/compact', description: 'Manually compact conversation' },
  { name: '/commands', description: 'List custom commands' },
];

/**
 * Fuzzy match scoring algorithm
 * Returns a score for how well the query matches the text
 * Higher score = better match
 *
 * Matching rules:
 * - Must match from the beginning of the command
 * - /email matches /email:digest (prefix match)
 * - /email:d matches /email:digest (prefix match)
 * - /digest does NOT match /email:digest (no namespace prefix)
 */
function fuzzyMatchScore(query: string, text: string): number {
  const q = query.toLowerCase();
  const t = text.toLowerCase();

  // Exact match gets highest score
  if (t === q) return 1000;

  // Starts with query gets high score (prefix match)
  if (t.startsWith(q)) return 900;

  // Sequential character matching (fuzzy) - but only if it starts from beginning
  let score = 0;
  let queryIndex = 0;
  let lastMatchIndex = -1;

  for (let i = 0; i < t.length && queryIndex < q.length; i++) {
    if (t[i] === q[queryIndex]) {
      // First character MUST match at position 0 (start of command)
      if (queryIndex === 0 && i !== 0) {
        return 0; // Reject if first char doesn't match at start
      }

      // Bonus for consecutive matches
      const consecutiveBonus = (i === lastMatchIndex + 1) ? 5 : 0;
      score += 10 + consecutiveBonus;
      lastMatchIndex = i;
      queryIndex++;
    }
  }

  // Return 0 if not all query characters were found
  if (queryIndex !== q.length) return 0;

  return score;
}

interface CommandSuggestionsProps {
  input: string;
  selectedIndex: number;
  customCommands?: Command[];
}

export function CommandSuggestions({ input, selectedIndex, customCommands = [] }: CommandSuggestionsProps) {
  // Combine built-in and custom commands
  const allCommands = [...BUILTIN_COMMANDS, ...customCommands];

  // Fuzzy match and sort by score
  const matches: CommandMatch[] = allCommands
    .map((cmd) => ({
      command: cmd,
      score: fuzzyMatchScore(input, cmd.name),
    }))
    .filter((match) => match.score > 0)
    .sort((a, b) => b.score - a.score)
    .slice(0, 10); // Show top 10 matches

  if (matches.length === 0) {
    return null;
  }

  return (
    <Box flexDirection="column" marginLeft={2}>
      {matches.map((match, i) => {
        const isSelected = i === selectedIndex;
        const cmd = match.command;
        return (
          <Box key={cmd.name}>
            <Text color={isSelected ? colors.primary : colors.textSecondary}>
              {cmd.name.padEnd(25)}
            </Text>
            <Text color={colors.textMuted}>{cmd.description}</Text>
          </Box>
        );
      })}
    </Box>
  );
}

export function getFilteredCommands(input: string, customCommands: Command[] = []): Command[] {
  const allCommands = [...BUILTIN_COMMANDS, ...customCommands];

  // Fuzzy match and sort
  const matches = allCommands
    .map((cmd) => ({
      command: cmd,
      score: fuzzyMatchScore(input, cmd.name),
    }))
    .filter((match) => match.score > 0)
    .sort((a, b) => b.score - a.score)
    .slice(0, 10);

  return matches.map((m) => m.command);
}
