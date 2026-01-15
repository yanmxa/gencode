/**
 * Model Selector Component - Interactive model selection from cached providers
 */
import { useState, useEffect, useMemo } from 'react';
import { Box, Text, useInput } from 'ink';
import TextInput from 'ink-text-input';
import { colors, icons } from './theme.js';
import { LoadingSpinner } from './Spinner.js';
import { getProviderStore, type ModelInfo } from '../../providers/store.js';
import { getProvider } from '../../providers/registry.js';
import type { ProviderName } from '../../providers/index.js';

interface ModelItem {
  providerId: ProviderName;
  providerName: string;
  model: ModelInfo;
}

interface ModelSelectorProps {
  currentModel: string;
  onSelect: (modelId: string, providerId: ProviderName) => void;
  onCancel: () => void;
  listModels: () => Promise<{ id: string; name: string }[]>; // Fallback for current provider
}

export function ModelSelector({
  currentModel,
  onSelect,
  onCancel,
  listModels,
}: ModelSelectorProps) {
  const store = getProviderStore();
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [allModels, setAllModels] = useState<ModelItem[]>([]);

  // Load models from cache
  useEffect(() => {
    const loadModels = async () => {
      const connectedProviders = store.getConnectedProviders();
      const items: ModelItem[] = [];

      for (const providerId of connectedProviders) {
        const cachedModels = store.getModels(providerId);
        const providerDef = getProvider(providerId);
        const providerName = providerDef?.name || providerId;

        for (const model of cachedModels) {
          items.push({
            providerId,
            providerName,
            model,
          });
        }
      }

      // If no cached models, fallback to listModels for current provider
      if (items.length === 0) {
        try {
          const models = await listModels();
          for (const model of models) {
            items.push({
              providerId: 'anthropic' as ProviderName, // Default, will be overridden
              providerName: 'Current Provider',
              model,
            });
          }
        } catch {
          // Ignore errors
        }
      }

      setAllModels(items);
      setLoading(false);
    };

    loadModels();
  }, [store, listModels]);

  // Filter models
  const filterLower = filter.toLowerCase();
  const filtered = useMemo(() => {
    return allModels.filter(
      (item) =>
        item.model.id.toLowerCase().includes(filterLower) ||
        item.model.name.toLowerCase().includes(filterLower) ||
        item.providerName.toLowerCase().includes(filterLower)
    );
  }, [allModels, filterLower]);

  // Group by provider for display
  const groupedModels = useMemo(() => {
    const groups: Record<string, ModelItem[]> = {};
    for (const item of filtered) {
      if (!groups[item.providerId]) {
        groups[item.providerId] = [];
      }
      groups[item.providerId].push(item);
    }
    return groups;
  }, [filtered]);

  // Flat list for navigation
  const flatList = useMemo(() => {
    const items: ModelItem[] = [];
    for (const providerId of Object.keys(groupedModels)) {
      items.push(...groupedModels[providerId]);
    }
    return items;
  }, [groupedModels]);

  // Reset selection when filter changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [filter]);

  // Keyboard navigation
  useInput((input, key) => {
    if (key.upArrow) {
      setSelectedIndex((i) => Math.max(0, i - 1));
    } else if (key.downArrow) {
      setSelectedIndex((i) => Math.min(flatList.length - 1, i + 1));
    } else if (key.return) {
      if (flatList.length > 0) {
        const selected = flatList[selectedIndex];
        onSelect(selected.model.id, selected.providerId);
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

  // Count providers and models
  const providerCount = Object.keys(groupedModels).length;
  const modelCount = flatList.length;

  // Calculate visible window
  const maxVisible = 10;
  const startIndex = Math.max(
    0,
    Math.min(selectedIndex - Math.floor(maxVisible / 2), flatList.length - maxVisible)
  );
  const endIndex = Math.min(startIndex + maxVisible, flatList.length);

  // Build visible items with provider headers
  let currentIdx = 0;
  const renderItems: Array<{ type: 'header' | 'model'; content: string; item?: ModelItem }> = [];

  for (const providerId of Object.keys(groupedModels)) {
    const models = groupedModels[providerId];
    const firstIdx = currentIdx;
    const lastIdx = currentIdx + models.length - 1;

    // Check if any model from this provider is in visible range
    if (lastIdx >= startIndex && firstIdx < endIndex) {
      // Add header if first visible item is from this provider
      const providerDef = getProvider(providerId as ProviderName);
      const connection = store.getConnection(providerId as ProviderName);
      const headerText = `${providerDef?.name || providerId}${connection ? ` (${connection.method})` : ''}:`;

      // Only add header if we're showing models from this provider
      const visibleModelsFromProvider = models.filter((_, i) => {
        const globalIdx = currentIdx + i;
        return globalIdx >= startIndex && globalIdx < endIndex;
      });

      if (visibleModelsFromProvider.length > 0 && (firstIdx >= startIndex || renderItems.length === 0)) {
        renderItems.push({ type: 'header', content: headerText });
      }

      for (let i = 0; i < models.length; i++) {
        const globalIdx = currentIdx + i;
        if (globalIdx >= startIndex && globalIdx < endIndex) {
          renderItems.push({ type: 'model', content: '', item: models[i] });
        }
      }
    }

    currentIdx += models.length;
  }

  return (
    <Box flexDirection="column">
      <Text color={colors.primary} bold>
        Select Model
      </Text>

      <Box marginTop={1}>
        <Text color={colors.textMuted}>{icons.prompt} </Text>
        <TextInput value={filter} onChange={setFilter} placeholder="Filter models..." />
      </Box>

      <Box flexDirection="column" marginTop={1}>
        {flatList.length === 0 ? (
          <Box flexDirection="column">
            <Text color={colors.textMuted}>No cached models.</Text>
            <Text color={colors.textMuted}>Use /provider to connect and cache models.</Text>
          </Box>
        ) : (
          renderItems.map((renderItem, i) => {
            if (renderItem.type === 'header') {
              return (
                <Text key={`header-${i}`} color={colors.textSecondary}>
                  {renderItem.content}
                </Text>
              );
            }

            const item = renderItem.item!;
            const globalIndex = flatList.findIndex(
              (f) => f.providerId === item.providerId && f.model.id === item.model.id
            );
            const isSelected = globalIndex === selectedIndex;
            const isCurrent = item.model.id === currentModel;

            return (
              <Box key={`${item.providerId}-${item.model.id}`} paddingLeft={2}>
                <Text color={isSelected ? colors.primary : colors.textMuted}>
                  {isSelected ? icons.arrow : ' '}
                </Text>
                <Text color={isCurrent ? colors.primary : colors.textMuted}>
                  {isCurrent ? icons.radio : icons.radioEmpty}
                </Text>
                <Text color={isSelected ? colors.text : colors.textSecondary} bold={isSelected}>
                  {' '}{item.model.name || item.model.id}
                </Text>
                {isCurrent && <Text color={colors.success}> (current)</Text>}
              </Box>
            );
          })
        )}
      </Box>

      <Box marginTop={1}>
        <Text color={colors.textMuted}>
          {providerCount} provider{providerCount !== 1 ? 's' : ''} · {modelCount} model
          {modelCount !== 1 ? 's' : ''} · ↑↓ navigate · Enter select · Esc cancel
        </Text>
      </Box>
    </Box>
  );
}
