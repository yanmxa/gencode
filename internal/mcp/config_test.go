package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigLoader_SaveAndLoad(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "mcp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create project dir
	projectDir := filepath.Join(tmpDir, ".gen")
	os.MkdirAll(projectDir, 0755)

	loader := &ConfigLoader{
		userDir:    filepath.Join(tmpDir, "user", ".gen"),
		projectDir: projectDir,
	}

	// Test saving a server config
	config := ServerConfig{
		Type:    TransportSTDIO,
		Command: "echo",
		Args:    []string{"hello"},
		Env:     map[string]string{"FOO": "bar"},
	}

	err = loader.SaveServer("test-server", config, ScopeLocal)
	if err != nil {
		t.Fatalf("Failed to save server: %v", err)
	}

	// Verify file exists
	localPath := filepath.Join(projectDir, "mcp.local.json")
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Load and verify
	configs, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("Failed to load configs: %v", err)
	}

	if len(configs) != 1 {
		t.Fatalf("Expected 1 config, got %d", len(configs))
	}

	loaded, ok := configs["test-server"]
	if !ok {
		t.Fatal("Server 'test-server' not found in loaded configs")
	}

	if loaded.Command != "echo" {
		t.Errorf("Expected command 'echo', got '%s'", loaded.Command)
	}

	if len(loaded.Args) != 1 || loaded.Args[0] != "hello" {
		t.Errorf("Args mismatch: %v", loaded.Args)
	}
}

func TestConfigLoader_ScopePriority(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "mcp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	userDir := filepath.Join(tmpDir, "user", ".gen")
	projectDir := filepath.Join(tmpDir, "project", ".gen")
	os.MkdirAll(userDir, 0755)
	os.MkdirAll(projectDir, 0755)

	// Create user-level config
	userConfig := MCPConfig{
		MCPServers: map[string]ServerConfig{
			"shared": {
				Command: "user-command",
			},
		},
	}
	userData, _ := json.Marshal(userConfig)
	os.WriteFile(filepath.Join(userDir, "mcp.json"), userData, 0644)

	// Create project-level config (should override)
	projectConfig := MCPConfig{
		MCPServers: map[string]ServerConfig{
			"shared": {
				Command: "project-command",
			},
		},
	}
	projectData, _ := json.Marshal(projectConfig)
	os.WriteFile(filepath.Join(projectDir, "mcp.json"), projectData, 0644)

	loader := &ConfigLoader{
		userDir:    userDir,
		projectDir: projectDir,
	}

	configs, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("Failed to load configs: %v", err)
	}

	// Project config should override user config
	if configs["shared"].Command != "project-command" {
		t.Errorf("Expected project-command, got %s", configs["shared"].Command)
	}
}

func TestServerConfig_GetType(t *testing.T) {
	tests := []struct {
		name     string
		config   ServerConfig
		expected TransportType
	}{
		{
			name:     "default to stdio",
			config:   ServerConfig{Command: "echo"},
			expected: TransportSTDIO,
		},
		{
			name:     "infer http from URL",
			config:   ServerConfig{URL: "https://example.com"},
			expected: TransportHTTP,
		},
		{
			name:     "explicit http",
			config:   ServerConfig{Type: TransportHTTP, URL: "https://example.com"},
			expected: TransportHTTP,
		},
		{
			name:     "explicit sse",
			config:   ServerConfig{Type: TransportSSE, URL: "https://example.com"},
			expected: TransportSSE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetType()
			if got != tt.expected {
				t.Errorf("GetType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseMCPToolName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantServer string
		wantTool   string
		wantOk     bool
	}{
		{
			name:       "valid mcp tool",
			input:      "mcp__filesystem__read_file",
			wantServer: "filesystem",
			wantTool:   "read_file",
			wantOk:     true,
		},
		{
			name:       "valid with dashes",
			input:      "mcp__my-server__my-tool",
			wantServer: "my-server",
			wantTool:   "my-tool",
			wantOk:     true,
		},
		{
			name:   "not mcp tool",
			input:  "Read",
			wantOk: false,
		},
		{
			name:   "incomplete prefix",
			input:  "mcp_server__tool",
			wantOk: false,
		},
		{
			name:   "missing tool",
			input:  "mcp__server",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, tool, ok := ParseMCPToolName(tt.input)
			if ok != tt.wantOk {
				t.Errorf("ParseMCPToolName() ok = %v, want %v", ok, tt.wantOk)
			}
			if ok {
				if server != tt.wantServer {
					t.Errorf("ParseMCPToolName() server = %v, want %v", server, tt.wantServer)
				}
				if tool != tt.wantTool {
					t.Errorf("ParseMCPToolName() tool = %v, want %v", tool, tt.wantTool)
				}
			}
		})
	}
}
