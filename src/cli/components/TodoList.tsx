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
  const inProgress = todos.find((t) => t.status === 'in_progress');

  return (
    <Box flexDirection="column" marginTop={1} marginLeft={2}>
      {/* Header with active task */}
      {inProgress ? (
        <Text color={colors.textSecondary}>
          <Text color={colors.warning}>▸</Text> {inProgress.activeForm || inProgress.content}
          <Text color={colors.textMuted}> ({completed}/{total})</Text>
        </Text>
      ) : (
        <Text color={colors.textMuted}>
          Tasks {completed}/{total}
        </Text>
      )}
      {/* Task list - show only if more than one task */}
      {todos.length > 1 && todos.map((todo, i) => {
        const isCompleted = todo.status === 'completed';
        const isInProgress = todo.status === 'in_progress';

        // Status indicators: ✓ done, ▸ active, ○ pending
        let indicator: string;
        let indicatorColor: string;

        if (isCompleted) {
          indicator = '✓';
          indicatorColor = colors.success;
        } else if (isInProgress) {
          indicator = '▸';
          indicatorColor = colors.warning;
        } else {
          indicator = '○';
          indicatorColor = colors.textMuted;
        }

        return (
          <Text key={i} dimColor={isCompleted}>
            <Text color={indicatorColor}> {indicator}</Text>
            <Text strikethrough={isCompleted} color={isCompleted ? colors.textMuted : colors.textSecondary}> {todo.content}</Text>
          </Text>
        );
      })}
    </Box>
  );
}
