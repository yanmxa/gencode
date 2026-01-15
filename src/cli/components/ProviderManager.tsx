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
  getAvailableConnections,
  isConnectionReady,
  type ProviderDefinition,
  type ConnectionOption,
} from '../../providers/registry.js';
import { getProviderStore, type ModelInfo } from '../../providers/store.js';
import { createProvider, type ProviderName } from '../../providers/index.js';

interface ProviderManagerProps {
  onClose: () => void;
  onProviderChange?: (providerId: ProviderName, model: string) => void;
}

type View = 'list' | 'select-connection' | 'confirm-remove';

interface ProviderItem {
  provider: ProviderDefinition;
  connected: boolean;
  modelCount: number;
  connectionMethod?: string;
  readyConnections: ConnectionOption[];
}

export function ProviderManager({ onClose }: ProviderManagerProps) {
  const store = getProviderStore();

  const [view, setView] = useState<View>('list');
  const [filter, setFilter] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [connectionIndex, setConnectionIndex] = useState(0);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [selectedProvider, setSelectedProvider] = useState<ProviderDefinition | null>(null);

  // Build provider list
  const buildProviderList = useCallback((): ProviderItem[] => {
    const allProviders = getProvidersSorted();
    return allProviders.map((provider) => {
      const connected = store.isConnected(provider.id);
      const connection = store.getConnection(provider.id);
      const readyConnections = getAvailableConnections(provider);
      return {
        provider,
        connected,
        modelCount: store.getModelCount(provider.id),
        connectionMethod: connection?.method,
        readyConnections,
      };
    });
  }, [store]);

  const [providerList, setProviderList] = useState<ProviderItem[]>(buildProviderList);

  // Refresh list
  const refreshList = useCallback(() => {
    setProviderList(buildProviderList());
  }, [buildProviderList]);

  // Filter providers
  const filterLower = filter.toLowerCase();
  const filteredProviders = providerList.filter(
    (item) =>
      item.provider.name.toLowerCase().includes(filterLower) ||
      item.provider.id.toLowerCase().includes(filterLower)
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

  // Fetch and cache models for a provider (use providerImpl if specified)
  const fetchModels = async (
    providerId: ProviderName,
    connOption?: ConnectionOption
  ): Promise<ModelInfo[]> => {
    try {
      // Use providerImpl if specified, otherwise use the provider id
      const implId = connOption?.providerImpl || providerId;
      const provider = createProvider({ provider: implId });
      const models = await provider.listModels();
      store.cacheModels(providerId, models);
      return models;
    } catch {
      return [];
    }
  };

  // Connect with a specific connection option
  const connectWithOption = async (item: ProviderItem, connOption: ConnectionOption) => {
    setLoading(true);
    setMessage(`Connecting via ${connOption.name}...`);
    store.connect(item.provider.id, connOption.method);
    const models = await fetchModels(item.provider.id, connOption);
    setLoading(false);
    setMessage(`Connected! Cached ${models.length} models`);
    refreshList();
    setView('list');
    setSelectedProvider(null);
    setTimeout(() => setMessage(null), 2000);
  };

  // Handle connect/refresh
  const handleConnect = async (item: ProviderItem) => {
    if (item.connected) {
      // Refresh: re-fetch models
      setLoading(true);
      setMessage(`Refreshing ${item.provider.name}...`);
      // Get the connection method to find the right provider impl
      const connMethod = item.connectionMethod;
      const connOption = item.provider.connections.find((c) => c.method === connMethod);
      const models = await fetchModels(item.provider.id, connOption);
      setLoading(false);
      setMessage(`Cached ${models.length} models`);
      refreshList();
      setTimeout(() => setMessage(null), 2000);
    } else {
      // Check ready connections
      const readyConns = item.readyConnections;

      if (readyConns.length === 1) {
        // One ready connection - auto-connect
        await connectWithOption(item, readyConns[0]);
      } else {
        // Zero or multiple ready connections - show selection view
        setSelectedProvider(item.provider);
        setConnectionIndex(0);
        setView('select-connection');
      }
    }
  };

  // Handle remove
  const handleRemove = (item: ProviderItem) => {
    if (!item.connected) return;
    setSelectedProvider(item.provider);
    setView('confirm-remove');
  };

  // Confirm remove
  const confirmRemove = () => {
    if (selectedProvider) {
      store.disconnect(selectedProvider.id);
      refreshList();
      setView('list');
      setSelectedProvider(null);
      setMessage('Provider removed');
      setTimeout(() => setMessage(null), 2000);
    }
  };

  // Go back to list
  const goBack = () => {
    setView('list');
    setSelectedProvider(null);
    setConnectionIndex(0);
  };

  // Keyboard navigation for list view
  useInput(
    (input, key) => {
      if (key.upArrow) {
        setSelectedIndex((i) => Math.max(0, i - 1));
      } else if (key.downArrow) {
        setSelectedIndex((i) => Math.min(allItems.length - 1, i + 1));
      } else if (key.return && allItems.length > 0) {
        handleConnect(allItems[selectedIndex]);
      } else if (input === 'r' && allItems.length > 0 && allItems[selectedIndex].connected) {
        handleRemove(allItems[selectedIndex]);
      } else if (key.escape) {
        onClose();
      }
    },
    { isActive: view === 'list' && !loading }
  );

  // Keyboard for select-connection view
  useInput(
    (_input, key) => {
      if (!selectedProvider) return;
      const allConns = selectedProvider.connections;

      if (key.upArrow) {
        setConnectionIndex((i) => Math.max(0, i - 1));
      } else if (key.downArrow) {
        setConnectionIndex((i) => Math.min(allConns.length - 1, i + 1));
      } else if (key.return && allConns.length > 0) {
        const selectedConn = allConns[connectionIndex];
        if (isConnectionReady(selectedConn)) {
          const item = allItems.find((i) => i.provider.id === selectedProvider.id);
          if (item) {
            connectWithOption(item, selectedConn);
          }
        }
        // If not ready, do nothing (user needs to set env vars first)
      } else if (key.escape) {
        goBack();
      }
    },
    { isActive: view === 'select-connection' }
  );

  // Keyboard for confirm-remove view
  useInput(
    (input, key) => {
      if (input === 'y' || input === 'Y') {
        confirmRemove();
      } else if (input === 'n' || input === 'N' || key.escape) {
        goBack();
      }
    },
    { isActive: view === 'confirm-remove' }
  );


  // Loading state
  if (loading) {
    return (
      <Box flexDirection="column">
        <Box>
          <LoadingSpinner />
          <Text color={colors.textMuted}> {message || 'Loading...'}</Text>
        </Box>
      </Box>
    );
  }

  // Confirm remove view
  if (view === 'confirm-remove' && selectedProvider) {
    return (
      <Box flexDirection="column">
        <Text color={colors.warning}>Remove {selectedProvider.name}?</Text>
        <Text color={colors.textMuted}>
          This will clear cached models for this provider.
        </Text>
        <Box marginTop={1}>
          <Text color={colors.textMuted}>[Y] Confirm   [N] Cancel</Text>
        </Box>
      </Box>
    );
  }

  // Select connection method view
  if (view === 'select-connection' && selectedProvider) {
    const allConns = selectedProvider.connections;
    const selectedConn = allConns[connectionIndex];
    const isSelectedReady = selectedConn ? isConnectionReady(selectedConn) : false;

    return (
      <Box flexDirection="column">
        <Text color={colors.primary}>Connect to {selectedProvider.name}</Text>
        <Text color={colors.textMuted}>Select connection method:</Text>

        <Box flexDirection="column" marginTop={1}>
          {allConns.map((conn, idx) => {
            const isSelected = idx === connectionIndex;
            const ready = isConnectionReady(conn);
            return (
              <Box key={conn.method} paddingLeft={2} flexDirection="column">
                <Box>
                  <Text color={isSelected ? colors.primary : colors.textMuted}>
                    {isSelected ? icons.arrow : ' '}
                  </Text>
                  <Text color={isSelected ? colors.text : colors.textSecondary} bold={isSelected}>
                    {conn.name}
                  </Text>
                  {ready ? (
                    <Text color={colors.success}> (ready)</Text>
                  ) : (
                    <Text color={colors.textMuted}> (not configured)</Text>
                  )}
                  {conn.description && (
                    <Text color={colors.textMuted}> - {conn.description}</Text>
                  )}
                </Box>
                {isSelected && !ready && (
                  <Text color={colors.textMuted} dimColor>
                    {'    '}Set: {conn.envVars.join(' or ')}
                  </Text>
                )}
              </Box>
            );
          })}
        </Box>

        <Box marginTop={1}>
          <Text color={colors.textMuted}>
            ↑↓ navigate · {isSelectedReady ? 'Enter connect · ' : ''}Esc back
          </Text>
        </Box>
      </Box>
    );
  }

  // Main list view
  return (
    <Box flexDirection="column">
      <Text color={colors.primary} bold>
        Provider Management
      </Text>

      <Box marginTop={1}>
        <Text color={colors.textMuted}>{icons.prompt} </Text>
        <TextInput value={filter} onChange={setFilter} placeholder="Filter providers..." />
      </Box>

      {message && (
        <Box marginTop={1}>
          <Text color={colors.success}>{icons.success} {message}</Text>
        </Box>
      )}

      <Box flexDirection="column" marginTop={1}>
        {/* Connected section */}
        {connected.length > 0 && (
          <>
            <Text color={colors.textMuted}>Connected:</Text>
            {connected.map((item, idx) => {
              const isSelected = idx === selectedIndex;
              // Find connection name
              const connOption = item.provider.connections.find(
                (c) => c.method === item.connectionMethod
              );
              const connName = connOption?.name || item.connectionMethod;
              return (
                <Box key={item.provider.id} paddingLeft={2}>
                  <Text color={isSelected ? colors.primary : colors.textMuted}>
                    {isSelected ? icons.arrow : ' '}
                  </Text>
                  <Text color={colors.success}>{icons.success} </Text>
                  <Text color={isSelected ? colors.text : colors.textSecondary} bold={isSelected}>
                    {item.provider.name}
                  </Text>
                  <Text color={colors.textMuted}>
                    {' '}({connName}) · {item.modelCount} models
                  </Text>
                </Box>
              );
            })}
          </>
        )}

        {/* Available section */}
        {available.length > 0 && (
          <>
            <Text color={colors.textMuted} dimColor={connected.length > 0}>
              {connected.length > 0 ? '\n' : ''}Available:
            </Text>
            {available.map((item, idx) => {
              const actualIndex = connected.length + idx;
              const isSelected = actualIndex === selectedIndex;
              const hasReady = item.readyConnections.length > 0;

              // Show which connection methods are ready
              const readyNames = item.readyConnections.map((c) => c.name);

              return (
                <Box key={item.provider.id} paddingLeft={2}>
                  <Text color={isSelected ? colors.primary : colors.textMuted}>
                    {isSelected ? icons.arrow : ' '}
                  </Text>
                  <Text color={isSelected ? colors.text : colors.textSecondary} bold={isSelected}>
                    {item.provider.name}
                  </Text>
                  {hasReady && (
                    <Text color={colors.success}> ({readyNames.join(', ')})</Text>
                  )}
                </Box>
              );
            })}
          </>
        )}

        {allItems.length === 0 && (
          <Text color={colors.textMuted}>No providers match "{filter}"</Text>
        )}
      </Box>

      <Box marginTop={1}>
        <Text color={colors.textMuted}>
          {connected.length} connected · {available.length} available · ↑↓ navigate · Enter
          connect/refresh · r remove · Esc exit
        </Text>
      </Box>
    </Box>
  );
}
