/**
 * Model Selector Component - Interactive model selection from cached providers
 */
import { useState, useEffect, useMemo } from 'react';
import { Box, Text, useInput } from 'ink';
import TextInput from 'ink-text-input';
import { colors, icons } from './theme.js';
import { LoadingSpinner } from './Spinner.js';
import { getProviderStore, type ModelInfo } from '../../providers/store.js';
import { getProviderMeta } from '../../providers/registry.js';
import type { Provider, AuthMethod } from '../../providers/index.js';

interface ModelItem {
  providerId: Provider;
  providerName: string;
  authMethod: AuthMethod;
  model: ModelInfo;
}

interface ModelSelectorProps {
  currentModel: string;
  currentProvider?: Provider; // Current provider for adding missing model placeholder
  onSelect: (modelId: string, providerId: Provider, authMethod?: AuthMethod) => void;
  onCancel: () => void;
  listModels: () => Promise<{ id: string; name: string }[]>; // Fallback for current provider
}

export function ModelSelector({
  currentModel,
  currentProvider,
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
        const connection = store.getConnection(providerId);
        const authMethod = connection?.authMethod || 'api_key';
        const cachedModels = store.getModels(providerId);
        const providerMeta = getProviderMeta(providerId);
        const providerName = providerMeta?.name || providerId;

        for (const model of cachedModels) {
          items.push({
            providerId,
            providerName,
            authMethod,
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
              providerId: 'anthropic' as Provider, // Default, will be overridden
              providerName: 'Current Provider',
              authMethod: 'api_key', // Default
              model,
            });
          }
        } catch {
          // Ignore errors
        }
      }

      // Add current model if not in list (e.g., experimental models not cached)
      const hasCurrentModel = items.some((item) => item.model.id === currentModel);

      if (!hasCurrentModel && currentModel && currentProvider) {
        const connection = store.getConnection(currentProvider);
        const authMethod = connection?.authMethod || 'api_key';
        const providerMeta = getProviderMeta(currentProvider);
        const providerName = providerMeta?.name || currentProvider;

        items.unshift({
          providerId: currentProvider,
          providerName,
          authMethod,
          model: {
            id: currentModel,
            name: currentModel, // Display logic will add "(current)" marker
          },
        });
      }

      setAllModels(items);
      setLoading(false);
    };

    loadModels();
  }, [store, listModels, currentModel, currentProvider]);

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

  // Flat list for navigation - sort to put current model first
  const flatList = useMemo(() => {
    const items: ModelItem[] = [];
    for (const providerId of Object.keys(groupedModels)) {
      items.push(...groupedModels[providerId]);
    }
    // Sort: current model first, then alphabetically
    return items.sort((a, b) => {
      const aIsCurrent = a.model.id === currentModel;
      const bIsCurrent = b.model.id === currentModel;
      if (aIsCurrent && !bIsCurrent) return -1;
      if (!aIsCurrent && bIsCurrent) return 1;
      return (a.model.name || a.model.id).localeCompare(b.model.name || b.model.id);
    });
  }, [groupedModels, currentModel]);

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
        onSelect(selected.model.id, selected.providerId, selected.authMethod);
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

  // Build visible items with provider headers from sorted flatList
  const renderItems: Array<{ type: 'header' | 'model'; content: string; item?: ModelItem }> = [];
  let lastProviderId: string | null = null;

  for (let i = startIndex; i < endIndex; i++) {
    const item = flatList[i];
    if (!item) continue;

    // Add provider header when provider changes
    const providerKey = `${item.providerId}:${item.authMethod}`;
    if (providerKey !== lastProviderId) {
      const providerMeta = getProviderMeta(item.providerId);
      const headerText = `${providerMeta?.name || item.providerId} (${item.authMethod}):`;
      renderItems.push({ type: 'header', content: headerText });
      lastProviderId = providerKey;
    }

    renderItems.push({ type: 'model', content: '', item });
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
                <Text> </Text>
                <Text color={isCurrent ? colors.primary : colors.textMuted}>
                  {isCurrent ? icons.radio : icons.radioEmpty}
                </Text>
                <Text> </Text>
                <Text color={isSelected ? colors.text : colors.textSecondary} bold={isSelected}>
                  {item.model.name || item.model.id}
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
