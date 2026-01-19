/**
 * TodoList Component - Display current todos in CLI
 * Design: Minimal, clean, status-driven with clear visual hierarchy
 */
import { Box, Text } from 'ink';
import { colors } from './theme.js';
import type { TodoItem } from '../../core/tools/types.js';

interface TodoListProps {
  todos: TodoItem[];
}

export function TodoList({ todos }: TodoListProps) {
  if (todos.length === 0) return null;

  const completed = todos.filter((t) => t.status === 'completed').length;
  const total = todos.length;

  return (
    <Box flexDirection="column" marginTop={1} marginLeft={2}>
      {/* Header with count */}
      <Text color={colors.textMuted}>
        Tasks {completed}/{total}
      </Text>
      {/* Task list */}
      {todos.map((todo, i) => {
        const isCompleted = todo.status === 'completed';
        const isInProgress = todo.status === 'in_progress';

        // Status indicators: [x] done, [>] active, [ ] pending
        let bracket: string;
        let bracketColor: string;

        if (isCompleted) {
          bracket = '[x]';
          bracketColor = colors.success;
        } else if (isInProgress) {
          bracket = '[>]';
          bracketColor = colors.warning;
        } else {
          bracket = '[ ]';
          bracketColor = colors.textMuted;
        }

        return (
          <Text key={i} dimColor={isCompleted}>
            <Text color={bracketColor}>{bracket}</Text>
            <Text strikethrough={isCompleted}> {todo.content}</Text>
          </Text>
        );
      })}
    </Box>
  );
}
