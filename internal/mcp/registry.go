package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yanmxa/gencode/internal/provider"
)

// Registry manages multiple MCP server connections
type Registry struct {
	mu         sync.RWMutex
	clients    map[string]*Client
	configs    map[string]ServerConfig
	disabled   map[string]bool   // servers explicitly disabled by the user
	connecting map[string]bool   // servers currently being connected (async)
	connectErr map[string]string // last connection error for servers without a client
	loader     *ConfigLoader
	cwd        string

	// Callback when tool schemas change
	onToolsChanged func()
}

// mcpState is the on-disk format for persisted MCP runtime state.
type mcpState struct {
	Disabled []string `json:"disabled,omitempty"`
}

// DefaultRegistry is the global MCP registry
var DefaultRegistry *Registry

// Initialize initializes the global MCP registry with the given working directory
func Initialize(cwd string) error {
	reg, err := NewRegistry(cwd)
	if err != nil {
		return err
	}
	DefaultRegistry = reg
	return nil
}

// NewRegistryForTest creates a registry with pre-loaded configs for testing.
// It does not read from disk.
func NewRegistryForTest(configs map[string]ServerConfig) *Registry {
	return &Registry{
		clients:    make(map[string]*Client),
		configs:    configs,
		disabled:   make(map[string]bool),
		connecting: make(map[string]bool),
		connectErr: make(map[string]string),
	}
}

// NewRegistry creates a new MCP registry
func NewRegistry(cwd string) (*Registry, error) {
	loader := NewConfigLoader(cwd)
	configs, err := loader.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to load MCP configs: %w", err)
	}

	reg := &Registry{
		clients:    make(map[string]*Client),
		configs:    configs,
		disabled:   make(map[string]bool),
		connecting: make(map[string]bool),
		connectErr: make(map[string]string),
		loader:     loader,
		cwd:        cwd,
	}
	reg.loadState()
	return reg, nil
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
	delete(r.connecting, name)
	delete(r.connectErr, name)
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

// ConnectAll connects to all configured MCP servers.
// Connection errors are collected but don't stop other connections.
func (r *Registry) ConnectAll(ctx context.Context) []error {
	r.mu.RLock()
	names := make([]string, 0, len(r.configs))
	for name := range r.configs {
		names = append(names, name)
	}
	r.mu.RUnlock()

	var errs []error
	for _, name := range names {
		if err := r.Connect(ctx, name); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	return errs
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
		} else if r.connecting[name] {
			server.Status = StatusConnecting
		} else if errMsg, ok := r.connectErr[name]; ok {
			server.Status = StatusError
			server.Error = errMsg
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

// SetConnecting marks or unmarks a server as currently connecting.
// On failure, call SetConnectError to store the error; List() will report StatusError.
func (r *Registry) SetConnecting(name string, val bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if val {
		r.connecting[name] = true
		delete(r.connectErr, name)
	} else {
		delete(r.connecting, name)
	}
}

// SetConnectError stores a connection error for a server that failed to connect.
func (r *Registry) SetConnectError(name string, errMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if errMsg != "" {
		r.connectErr[name] = errMsg
	} else {
		delete(r.connectErr, name)
	}
}

// IsDisabled returns whether a server has been explicitly disabled by the user.
func (r *Registry) IsDisabled(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.disabled[name]
}

// SetDisabled sets the disabled state for a server and persists it.
func (r *Registry) SetDisabled(name string, disabled bool) {
	r.mu.Lock()
	if disabled {
		r.disabled[name] = true
	} else {
		delete(r.disabled, name)
	}
	r.mu.Unlock()
	r.saveState()
}

// statePath returns the path to the state file.
func (r *Registry) statePath() string {
	if r.loader != nil {
		return filepath.Join(r.loader.GetProjectDir(), "mcp-state.json")
	}
	return ""
}

// loadState loads persisted disabled state from disk.
func (r *Registry) loadState() {
	path := r.statePath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var state mcpState
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, name := range state.Disabled {
		r.disabled[name] = true
	}
}

// saveState persists disabled state to disk.
func (r *Registry) saveState() {
	path := r.statePath()
	if path == "" {
		return
	}
	r.mu.RLock()
	var state mcpState
	for name := range r.disabled {
		state.Disabled = append(state.Disabled, name)
	}
	r.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0644)
}
