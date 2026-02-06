package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/yanmxa/gencode/internal/provider"
)

// Registry manages multiple MCP server connections
type Registry struct {
	mu      sync.RWMutex
	clients map[string]*Client
	configs map[string]ServerConfig
	loader  *ConfigLoader
	cwd     string

	// Callback when tool schemas change
	onToolsChanged func()
}

// DefaultRegistry is the global MCP registry
var DefaultRegistry *Registry

// Initialize initializes the global MCP registry with the given working directory
func Initialize(cwd string) error {
	loader := NewConfigLoader(cwd)
	configs, err := loader.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load MCP configs: %w", err)
	}

	DefaultRegistry = &Registry{
		clients: make(map[string]*Client),
		configs: configs,
		loader:  loader,
		cwd:     cwd,
	}

	return nil
}

// NewRegistry creates a new MCP registry
func NewRegistry(cwd string) (*Registry, error) {
	loader := NewConfigLoader(cwd)
	configs, err := loader.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to load MCP configs: %w", err)
	}

	return &Registry{
		clients: make(map[string]*Client),
		configs: configs,
		loader:  loader,
		cwd:     cwd,
	}, nil
}

// Reload reloads configurations from disk
func (r *Registry) Reload() error {
	configs, err := r.loader.LoadAll()
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs = configs
	return nil
}

// AddServer adds a new server configuration
func (r *Registry) AddServer(name string, config ServerConfig, scope Scope) error {
	if err := r.loader.SaveServer(name, config, scope); err != nil {
		return err
	}

	r.mu.Lock()
	config.Name = name
	config.Scope = scope
	r.configs[name] = config
	r.mu.Unlock()

	return nil
}

// RemoveServer removes a server configuration
func (r *Registry) RemoveServer(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Disconnect if connected
	if client, ok := r.clients[name]; ok {
		client.Disconnect()
		delete(r.clients, name)
	}

	// Remove from all configs
	if err := r.loader.RemoveServerFromAll(name); err != nil {
		return err
	}

	delete(r.configs, name)
	return nil
}

// Connect connects to an MCP server
func (r *Registry) Connect(ctx context.Context, name string) error {
	r.mu.Lock()
	config, ok := r.configs[name]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("server not found: %s", name)
	}

	// Already connected?
	if client, ok := r.clients[name]; ok && client.IsConnected() {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	// Create and connect client
	client := NewClient(config)
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to %s: %w", name, err)
	}

	// Set up tools changed callback
	client.SetOnToolsChanged(r.notifyToolsChanged)

	r.mu.Lock()
	r.clients[name] = client
	r.mu.Unlock()

	r.notifyToolsChanged()
	return nil
}

// Disconnect disconnects from an MCP server
func (r *Registry) Disconnect(name string) error {
	r.mu.Lock()
	client, ok := r.clients[name]
	if !ok {
		r.mu.Unlock()
		return nil
	}

	err := client.Disconnect()
	delete(r.clients, name)
	r.mu.Unlock()

	r.notifyToolsChanged()
	return err
}

// DisconnectAll disconnects from all MCP servers
func (r *Registry) DisconnectAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, client := range r.clients {
		client.Disconnect()
		delete(r.clients, name)
	}
}

// GetClient returns a client by name
func (r *Registry) GetClient(name string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	client, ok := r.clients[name]
	return client, ok
}

// GetConfig returns a server config by name
func (r *Registry) GetConfig(name string) (ServerConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	config, ok := r.configs[name]
	return config, ok
}

// List returns all configured servers with their current status
func (r *Registry) List() []Server {
	r.mu.RLock()
	defer r.mu.RUnlock()

	servers := make([]Server, 0, len(r.configs))
	for name, config := range r.configs {
		server := Server{
			Config: config,
			Status: StatusDisconnected,
		}

		if client, ok := r.clients[name]; ok {
			server = client.ToServer()
		}

		servers = append(servers, server)
	}

	return servers
}

// emptySchema is the default empty JSON schema for tools without input schema
var emptySchema = map[string]any{
	"type":       "object",
	"properties": map[string]any{},
}

// GetToolSchemas returns provider.Tool schemas for all connected MCP servers
func (r *Registry) GetToolSchemas() []provider.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tools []provider.Tool
	for serverName, client := range r.clients {
		if !client.IsConnected() {
			continue
		}

		for _, mcpTool := range client.GetCachedTools() {
			tools = append(tools, provider.Tool{
				Name:        fmt.Sprintf("mcp__%s__%s", serverName, mcpTool.Name),
				Description: mcpTool.Description,
				Parameters:  parseInputSchema(mcpTool.InputSchema),
			})
		}
	}

	return tools
}

// parseInputSchema parses the input schema or returns a default empty schema
func parseInputSchema(raw json.RawMessage) any {
	if len(raw) == 0 {
		return emptySchema
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return emptySchema
	}
	return schema
}

// CallTool calls a tool on an MCP server
// The tool name should be in the format: mcp__<server>__<tool>
func (r *Registry) CallTool(ctx context.Context, fullName string, arguments map[string]any) (*ToolResult, error) {
	serverName, toolName, ok := ParseMCPToolName(fullName)
	if !ok {
		return nil, fmt.Errorf("invalid MCP tool name: %s", fullName)
	}

	client, ok := r.GetClient(serverName)
	if !ok {
		return nil, fmt.Errorf("MCP server not connected: %s", serverName)
	}

	return client.CallTool(ctx, toolName, arguments)
}

// SetOnToolsChanged sets a callback for when tools change
func (r *Registry) SetOnToolsChanged(callback func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onToolsChanged = callback
}

// notifyToolsChanged calls the tools changed callback if set
func (r *Registry) notifyToolsChanged() {
	r.mu.RLock()
	callback := r.onToolsChanged
	r.mu.RUnlock()
	if callback != nil {
		callback()
	}
}

// ParseMCPToolName parses a tool name in the format mcp__<server>__<tool>
func ParseMCPToolName(name string) (serverName, toolName string, ok bool) {
	rest, found := strings.CutPrefix(name, "mcp__")
	if !found {
		return "", "", false
	}

	serverName, toolName, ok = strings.Cut(rest, "__")
	if !ok || serverName == "" || toolName == "" {
		return "", "", false
	}

	return serverName, toolName, true
}

// IsMCPTool returns true if the tool name is an MCP tool
func IsMCPTool(name string) bool {
	_, _, ok := ParseMCPToolName(name)
	return ok
}
