/**
 * Model Selector Component - Interactive model selection with fuzzy filter
 */
import { useState, useEffect } from 'react';
import { Box, Text, useInput } from 'ink';
import TextInput from 'ink-text-input';
import { colors, icons } from './theme.js';
import { LoadingSpinner } from './Spinner.js';

interface Model {
  id: string;
  name: string;
}

interface ModelSelectorProps {
  currentModel: string;
  onSelect: (modelId: string) => void;
  onCancel: () => void;
  listModels: () => Promise<Model[]>;
}

export function ModelSelector({
  currentModel,
  onSelect,
  onCancel,
  listModels,
}: ModelSelectorProps) {
  const [models, setModels] = useState<Model[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);

  useEffect(() => {
    const fetchModels = async () => {
      try {
        const result = await listModels();
        setModels(result);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch models');
      } finally {
        setLoading(false);
      }
    };
    fetchModels();
  }, [listModels]);

  // Fuzzy filter models
  const filtered = models.filter(
    (m) =>
      m.id.toLowerCase().includes(filter.toLowerCase()) ||
      m.name.toLowerCase().includes(filter.toLowerCase())
  );

  // Reset selection when filter changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [filter]);

  // Keyboard navigation
  useInput((input, key) => {
    if (key.upArrow) {
      setSelectedIndex((i) => Math.max(0, i - 1));
    } else if (key.downArrow) {
      setSelectedIndex((i) => Math.min(filtered.length - 1, i + 1));
    } else if (key.return) {
      if (filtered.length > 0) {
        onSelect(filtered[selectedIndex].id);
      }
    } else if (key.escape) {
      onCancel();
    }
  });

  if (loading) {
    return (
      <Box>
        <LoadingSpinner />
        <Text color={colors.textMuted}> Loading models...</Text>
      </Box>
    );
  }

  if (error) {
    onCancel();
    return null;
  }

  const maxVisible = 8;
  const startIndex = Math.max(
    0,
    Math.min(selectedIndex - Math.floor(maxVisible / 2), filtered.length - maxVisible)
  );
  const visibleModels = filtered.slice(startIndex, startIndex + maxVisible);

  return (
    <Box flexDirection="column">
      <Box>
        <Text color={colors.primary}>{icons.prompt} </Text>
        <TextInput
          value={filter}
          onChange={setFilter}
          placeholder="Type to filter models..."
        />
      </Box>
      <Box flexDirection="column" marginTop={1}>
        {visibleModels.length === 0 ? (
          <Text color={colors.textMuted}>No models match "{filter}"</Text>
        ) : (
          visibleModels.map((m, i) => {
            const actualIndex = startIndex + i;
            const isSelected = actualIndex === selectedIndex;
            const isCurrent = m.id === currentModel;
            return (
              <Box key={m.id}>
                <Text color={isSelected ? colors.primary : colors.textMuted}>
                  {isSelected ? icons.arrow : ' '}
                </Text>
                <Text color={isSelected ? colors.text : colors.textSecondary} bold={isSelected}>
                  {m.name}
                </Text>
                {isCurrent && <Text color={colors.success}> (current)</Text>}
              </Box>
            );
          })
        )}
      </Box>
      <Box marginTop={1}>
        <Text color={colors.textMuted}>
          {filtered.length} models · ↑↓ navigate · Enter select · Esc cancel
        </Text>
      </Box>
    </Box>
  );
}
