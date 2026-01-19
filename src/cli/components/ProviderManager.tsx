/**
 * Provider Manager - Manage provider connections
 */
import { useState, useEffect, useCallback } from 'react';
import { Box, Text, useInput } from 'ink';
import TextInput from 'ink-text-input';
import { colors, icons } from './theme.js';
import { LoadingSpinner } from './Spinner.js';
import {
  getProvidersSorted,
  getProviderClasses,
  isProviderReady,
  getSearchProvidersSorted,
  type ProviderClass,
  type ProviderMeta,
  type SearchProviderDefinition,
} from '../../core/providers/registry.js';
import { getProviderStore, type ModelInfo } from '../../core/providers/store.js';
import { createProvider, type Provider } from '../../core/providers/index.js';
import { isSearchProviderAvailable, type SearchProviderName } from '../../core/providers/search/index.js';
import type { AuthMethod } from '../../core/providers/types.js';

interface ProviderManagerProps {
  onClose: () => void;
  onProviderChange?: (providerId: Provider, model: string) => void;
}

type View = 'list' | 'select-connection' | 'confirm-remove' | 'search-list';
type Tab = 'llm' | 'search';

interface ProviderItem {
  providerMeta: ProviderMeta;
  connected: boolean;
  modelCount: number;
  authMethod?: AuthMethod;
  availableClasses: ProviderClass[];
}

interface SearchProviderItem {
  provider: SearchProviderDefinition;
  isSelected: boolean;
  isAvailable: boolean;
}

export function ProviderManager({ onClose }: ProviderManagerProps) {
  const store = getProviderStore();

  const [view, setView] = useState<View>('list');
  const [tab, setTab] = useState<Tab>('llm');
  const [filter, setFilter] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [connectionIndex, setConnectionIndex] = useState(0);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [selectedProviderMeta, setSelectedProviderMeta] = useState<ProviderMeta | null>(null);
  const [searchSelectedIndex, setSearchSelectedIndex] = useState(0);

  // Build provider list
  const buildProviderList = useCallback((): ProviderItem[] => {
    const allProviders = getProvidersSorted();
    return allProviders.map((providerMeta) => {
      const connected = store.isConnected(providerMeta.id);
      const connection = store.getConnection(providerMeta.id);
      const availableClasses = getProviderClasses(providerMeta.id).filter((cls) =>
        isProviderReady(cls)
      );
      return {
        providerMeta,
        connected,
        modelCount: store.getModelCount(providerMeta.id),
        authMethod: connection?.authMethod,
        availableClasses,
      };
    });
  }, [store]);

  const [providerList, setProviderList] = useState<ProviderItem[]>(buildProviderList);

  // Refresh list
  const refreshList = useCallback(() => {
    setProviderList(buildProviderList());
  }, [buildProviderList]);

  // Build search provider list
  const buildSearchProviderList = useCallback((): SearchProviderItem[] => {
    const currentSearch = store.getSearchProvider();
    return getSearchProvidersSorted().map((provider) => ({
      provider,
      isSelected: provider.id === currentSearch || (!currentSearch && provider.id === 'exa'),
      isAvailable: isSearchProviderAvailable(provider.id),
    }));
  }, [store]);

  const searchProviders = buildSearchProviderList();

  // Select search provider
  const selectSearchProvider = (id: SearchProviderName) => {
    if (id === 'exa') {
      store.clearSearchProvider(); // Use default
    } else {
      store.setSearchProvider(id);
    }
    setMessage(`Search provider set to ${id}`);
    setTimeout(() => setMessage(null), 2000);
  };

  // Filter providers
  const filterLower = filter.toLowerCase();
  const filteredProviders = providerList.filter(
    (item) =>
      item.providerMeta.name.toLowerCase().includes(filterLower) ||
      item.providerMeta.id.toLowerCase().includes(filterLower)
  );

  // Split into connected and available
  const connected = filteredProviders.filter((p) => p.connected);
  const available = filteredProviders.filter((p) => !p.connected);

  // Combined list for navigation
  const allItems = [...connected, ...available];

  // Reset selection when filter changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [filter]);

  // Fetch and cache models for a provider with specific auth method
  const fetchModels = async (
    providerId: Provider,
    authMethod: AuthMethod
  ): Promise<ModelInfo[]> => {
    try {
      const provider = createProvider({ provider: providerId, authMethod });
      const models = await provider.listModels();
      store.cacheModels(providerId, authMethod, models);
      return models;
    } catch {
      return [];
    }
  };

  // Connect with a specific provider class (auth method)
  const connectWithClass = async (item: ProviderItem, providerClass: ProviderClass) => {
    setLoading(true);
    setMessage(`Connecting via ${providerClass.meta.displayName}...`);
    store.connect(
      item.providerMeta.id,
      providerClass.meta.authMethod,
      providerClass.meta.displayName
    );
    const models = await fetchModels(item.providerMeta.id, providerClass.meta.authMethod);
    setLoading(false);
    setMessage(`Connected! Cached ${models.length} models`);
    refreshList();
    setView('list');
    setSelectedProviderMeta(null);
    setTimeout(() => setMessage(null), 2000);
  };

  // Handle connect/refresh
  const handleConnect = async (item: ProviderItem) => {
    if (item.connected) {
      // Refresh: re-fetch models using the saved authMethod
      setLoading(true);
      setMessage(`Refreshing ${item.providerMeta.name}...`);
      const authMethod = item.authMethod || 'api_key';
      const models = await fetchModels(item.providerMeta.id, authMethod);
      setLoading(false);
      setMessage(`Cached ${models.length} models`);
      refreshList();
      setTimeout(() => setMessage(null), 2000);
    } else {
      // Check available provider classes
      const availableClasses = item.availableClasses;

      if (availableClasses.length === 1) {
        // One available auth method - auto-connect
        await connectWithClass(item, availableClasses[0]);
      } else {
        // Zero or multiple auth methods - show selection view
        setSelectedProviderMeta(item.providerMeta);
        setConnectionIndex(0);
        setView('select-connection');
      }
    }
  };

  // Handle remove
  const handleRemove = (item: ProviderItem) => {
    if (!item.connected) return;
    setSelectedProviderMeta(item.providerMeta);
    setView('confirm-remove');
  };

  // Confirm remove
  const confirmRemove = () => {
    if (selectedProviderMeta) {
      store.disconnect(selectedProviderMeta.id);
      refreshList();
      setView('list');
      setSelectedProviderMeta(null);
      setMessage('Provider removed');
      setTimeout(() => setMessage(null), 2000);
    }
  };

  // Input handling for list view
  useInput(
    (input, key) => {
      if (view === 'list' && tab === 'llm') {
        if (key.downArrow || input === 'j') {
          setSelectedIndex((i) => Math.min(i + 1, allItems.length - 1));
        } else if (key.upArrow || input === 'k') {
          setSelectedIndex((i) => Math.max(i - 1, 0));
        } else if (key.return) {
          const item = allItems[selectedIndex];
          if (item) handleConnect(item);
        } else if (input === 'r' || input === 'R') {
          const item = allItems[selectedIndex];
          if (item?.connected) handleConnect(item);
        } else if (input === 'd' || input === 'D') {
          const item = allItems[selectedIndex];
          if (item) handleRemove(item);
        } else if (input === 'q' || input === 'Q' || key.escape) {
          onClose();
        } else if (input === '/') {
          // Start filter mode (handled by TextInput)
        } else if (key.tab) {
          setTab('search');
          setSearchSelectedIndex(0);
        }
      } else if (view === 'list' && tab === 'search') {
        if (key.downArrow || input === 'j') {
          setSearchSelectedIndex((i) => Math.min(i + 1, searchProviders.length - 1));
        } else if (key.upArrow || input === 'k') {
          setSearchSelectedIndex((i) => Math.max(i - 1, 0));
        } else if (key.return) {
          const item = searchProviders[searchSelectedIndex];
          if (item && item.isAvailable) {
            selectSearchProvider(item.provider.id);
          }
        } else if (input === 'q' || input === 'Q' || key.escape) {
          onClose();
        } else if (key.tab) {
          setTab('llm');
          setSelectedIndex(0);
        }
      } else if (view === 'select-connection') {
        const item = allItems.find((p) => p.providerMeta.id === selectedProviderMeta?.id);
        const availableClasses = item?.availableClasses || [];

        if (key.downArrow || input === 'j') {
          setConnectionIndex((i) => Math.min(i + 1, availableClasses.length - 1));
        } else if (key.upArrow || input === 'k') {
          setConnectionIndex((i) => Math.max(i - 1, 0));
        } else if (key.return) {
          if (item && availableClasses[connectionIndex]) {
            connectWithClass(item, availableClasses[connectionIndex]);
          }
        } else if (key.escape) {
          setView('list');
          setSelectedProviderMeta(null);
        }
      } else if (view === 'confirm-remove') {
        if (input === 'y' || input === 'Y') {
          confirmRemove();
        } else if (input === 'n' || input === 'N' || key.escape) {
          setView('list');
          setSelectedProviderMeta(null);
        }
      }
    },
    { isActive: !loading }
  );

  // Render connection selection view
  if (view === 'select-connection' && selectedProviderMeta) {
    const item = allItems.find((p) => p.providerMeta.id === selectedProviderMeta.id);
    const availableClasses = item?.availableClasses || [];

    return (
      <Box flexDirection="column" padding={1}>
        <Text bold color={colors.primary}>
          Select Authentication Method for {selectedProviderMeta.name}
        </Text>
        <Text dimColor>Choose how to connect to this provider</Text>
        <Box marginTop={1} flexDirection="column">
          {availableClasses.length === 0 ? (
            <Text color={colors.error}>
              {icons.warning} No authentication methods available
            </Text>
          ) : (
            availableClasses.map((providerClass, idx) => (
              <Box key={providerClass.meta.authMethod}>
                <Text color={idx === connectionIndex ? colors.success : undefined}>
                  {idx === connectionIndex ? icons.radio : ' '}{' '}
                </Text>
                <Text
                  bold={idx === connectionIndex}
                  color={idx === connectionIndex ? colors.success : undefined}
                >
                  {providerClass.meta.displayName}
                </Text>
                <Text dimColor> - {providerClass.meta.description}</Text>
              </Box>
            ))
          )}
        </Box>
        <Box marginTop={1}>
          <Text dimColor>Press Enter to connect, Esc to cancel</Text>
        </Box>
      </Box>
    );
  }

  // Render confirm remove view
  if (view === 'confirm-remove' && selectedProviderMeta) {
    return (
      <Box flexDirection="column" padding={1}>
        <Text bold color={colors.error}>
          Remove {selectedProviderMeta.name}?
        </Text>
        <Text dimColor>This will clear all cached models for this provider</Text>
        <Box marginTop={1}>
          <Text dimColor>Press Y to confirm, N to cancel</Text>
        </Box>
      </Box>
    );
  }

  // Render main list view
  return (
    <Box flexDirection="column" padding={1}>
      {/* Title */}
      <Box>
        <Text bold color={colors.primary}>
          Provider Manager
        </Text>
        <Text dimColor> - Manage LLM and Search providers</Text>
      </Box>

      {/* Tabs */}
      <Box marginTop={1}>
        <Box marginRight={2}>
          <Text bold={tab === 'llm'} color={tab === 'llm' ? colors.primary : 'gray'}>
            {tab === 'llm' ? '▶' : ' '} LLM Providers
          </Text>
        </Box>
        <Box>
          <Text bold={tab === 'search'} color={tab === 'search' ? colors.primary : 'gray'}>
            {tab === 'search' ? '▶' : ' '} Search Providers
          </Text>
        </Box>
      </Box>

      {/* LLM Providers Tab */}
      {tab === 'llm' && (
        <>
          {/* Filter */}
          <Box marginTop={1}>
            <Text dimColor>Filter: </Text>
            <TextInput value={filter} onChange={setFilter} placeholder="Type to filter..." />
          </Box>

          {/* Status */}
          {message && (
            <Box marginTop={1}>
              <Text color={colors.success}>{message}</Text>
            </Box>
          )}

          {loading && (
            <Box marginTop={1}>
              <LoadingSpinner />
            </Box>
          )}

          {/* Connected Providers */}
          {connected.length > 0 && (
            <>
              <Box marginTop={1}>
                <Text bold color={colors.success}>
                  Connected ({connected.length})
                </Text>
              </Box>
              {connected.map((item, idx) => {
                const globalIdx = idx;
                const isSelected = globalIdx === selectedIndex;
                return (
                  <Box key={item.providerMeta.id}>
                    <Text color={isSelected ? colors.success : undefined}>
                      {isSelected ? icons.radio : ' '}{' '}
                    </Text>
                    <Text bold={isSelected}>{item.providerMeta.name}</Text>
                    <Text dimColor> ({item.modelCount} models)</Text>
                    {item.authMethod && (
                      <Text dimColor> - {item.authMethod}</Text>
                    )}
                  </Box>
                );
              })}
            </>
          )}

          {/* Available Providers */}
          {available.length > 0 && (
            <>
              <Box marginTop={1}>
                <Text bold dimColor>
                  Available ({available.length})
                </Text>
              </Box>
              {available.map((item, idx) => {
                const globalIdx = connected.length + idx;
                const isSelected = globalIdx === selectedIndex;
                const hasAuth = item.availableClasses.length > 0;
                return (
                  <Box key={item.providerMeta.id}>
                    <Text color={isSelected ? colors.success : undefined}>
                      {isSelected ? icons.radio : ' '}{' '}
                    </Text>
                    <Text
                      bold={isSelected}
                      dimColor={!hasAuth}
                      color={!hasAuth ? colors.error : undefined}
                    >
                      {item.providerMeta.name}
                    </Text>
                    {!hasAuth && (
                      <Text dimColor color={colors.error}>
                        {' '}
                        (no credentials)
                      </Text>
                    )}
                  </Box>
                );
              })}
            </>
          )}

          {/* Help */}
          <Box marginTop={1}>
            <Text dimColor>
              Enter: Connect/Refresh | R: Refresh | D: Remove | Tab: Switch | Q: Quit
            </Text>
          </Box>
        </>
      )}

      {/* Search Providers Tab */}
      {tab === 'search' && (
        <>
          <Box marginTop={1}>
            <Text bold>Search Providers</Text>
          </Box>

          {searchProviders.map((item, idx) => {
            const isSelected = idx === searchSelectedIndex;
            return (
              <Box key={item.provider.id}>
                <Text color={isSelected ? colors.success : undefined}>
                  {isSelected ? icons.radio : ' '}{' '}
                </Text>
                <Text
                  bold={isSelected}
                  color={item.isSelected ? colors.success : undefined}
                  dimColor={!item.isAvailable}
                >
                  {item.provider.name}
                </Text>
                {item.isSelected && <Text color={colors.success}> (current)</Text>}
                {!item.isAvailable && (
                  <Text dimColor color={colors.error}>
                    {' '}
                    (not available)
                  </Text>
                )}
              </Box>
            );
          })}

          <Box marginTop={1}>
            <Text dimColor>Enter: Select | Tab: Switch | Q: Quit</Text>
          </Box>
        </>
      )}
    </Box>
  );
}
