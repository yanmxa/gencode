package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yanmxa/gencode/internal/mcp"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP (Model Context Protocol) servers",
	Long: `Manage MCP servers for extending GenCode with external tools.

MCP servers provide additional tools, resources, and prompts that can be used
by the LLM during conversations.

Configuration files are stored at:
  ~/.gen/mcp.json           User-level (global)
  ./.gen/mcp.json           Project-level (team shared)
  ./.gen/mcp.local.json     Local-level (personal, git-ignored)`,
}

var (
	mcpTransport string
	mcpScope     string
	mcpEnvVars   []string
	mcpHeaders   []string
)

func init() {
	// Add subcommands
	mcpCmd.AddCommand(mcpAddCmd)
	mcpCmd.AddCommand(mcpAddJSONCmd)
	mcpCmd.AddCommand(mcpListCmd)
	mcpCmd.AddCommand(mcpGetCmd)
	mcpCmd.AddCommand(mcpRemoveCmd)

	// Add flags
	mcpAddCmd.Flags().StringVarP(&mcpTransport, "transport", "t", "stdio", "Transport type (stdio, http, sse)")
	mcpAddCmd.Flags().StringVarP(&mcpScope, "scope", "s", "local", "Config scope (user, project, local)")
	mcpAddCmd.Flags().StringArrayVarP(&mcpEnvVars, "env", "e", nil, "Environment variables (KEY=value)")
	mcpAddCmd.Flags().StringArrayVarP(&mcpHeaders, "header", "H", nil, "HTTP headers (Key: Value)")

	mcpAddJSONCmd.Flags().StringVarP(&mcpScope, "scope", "s", "local", "Config scope (user, project, local)")
}

var mcpAddCmd = &cobra.Command{
	Use:   "add <name> [-- <command> [args...]] or add <name> <url>",
	Short: "Add an MCP server",
	Long: `Add an MCP server configuration.

For STDIO transport (default):
  gen mcp add <name> -- <command> [args...]

For HTTP transport:
  gen mcp add --transport http <name> <url>

For SSE transport:
  gen mcp add --transport sse <name> <url>

Examples:
  gen mcp add filesystem -- npx -y @modelcontextprotocol/server-filesystem .
  gen mcp add github --transport http https://api.github.com/mcp
  gen mcp add sentry --transport sse https://mcp.sentry.dev/mcp`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		var config mcp.ServerConfig
		config.Type = mcp.TransportType(mcpTransport)

		switch config.Type {
		case mcp.TransportSTDIO:
			// Find -- separator
			dashIdx := -1
			for i, arg := range os.Args {
				if arg == "--" {
					dashIdx = i
					break
				}
			}
			if dashIdx == -1 || dashIdx >= len(os.Args)-1 {
				return fmt.Errorf("STDIO transport requires: gen mcp add <name> -- <command> [args...]")
			}

			cmdArgs := os.Args[dashIdx+1:]
			config.Command = cmdArgs[0]
			if len(cmdArgs) > 1 {
				config.Args = cmdArgs[1:]
			}

		case mcp.TransportHTTP, mcp.TransportSSE:
			if len(args) < 2 {
				return fmt.Errorf("%s transport requires a URL: gen mcp add --transport %s <name> <url>", mcpTransport, mcpTransport)
			}
			config.URL = args[1]
			config.Headers = parseKeyValues(mcpHeaders, ":")

		default:
			return fmt.Errorf("unsupported transport type: %s", mcpTransport)
		}

		config.Env = parseKeyValues(mcpEnvVars, "=")

		// Save configuration
		cwd, _ := os.Getwd()
		loader := mcp.NewConfigLoader(cwd)
		scope := parseScope(mcpScope)

		if err := loader.SaveServer(name, config, scope); err != nil {
			return fmt.Errorf("failed to save server: %w", err)
		}

		fmt.Printf("Added MCP server '%s' to %s scope\n", name, mcpScope)
		return nil
	},
}

var mcpAddJSONCmd = &cobra.Command{
	Use:   "add-json <name> <json>",
	Short: "Add an MCP server from JSON configuration",
	Long: `Add an MCP server using a JSON configuration.

Example:
  gen mcp add-json filesystem '{"command":"npx","args":["-y","@modelcontextprotocol/server-filesystem","."]}'`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		jsonStr := args[1]

		var config mcp.ServerConfig
		if err := json.Unmarshal([]byte(jsonStr), &config); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}

		cwd, _ := os.Getwd()
		loader := mcp.NewConfigLoader(cwd)
		scope := parseScope(mcpScope)

		if err := loader.SaveServer(name, config, scope); err != nil {
			return fmt.Errorf("failed to save server: %w", err)
		}

		fmt.Printf("Added MCP server '%s' to %s scope\n", name, mcpScope)
		return nil
	},
}

var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured MCP servers",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		loader := mcp.NewConfigLoader(cwd)
		configs, err := loader.LoadAll()
		if err != nil {
			return fmt.Errorf("failed to load configs: %w", err)
		}

		if len(configs) == 0 {
			fmt.Println("No MCP servers configured.")
			fmt.Println("\nAdd a server with:")
			fmt.Println("  gen mcp add <name> -- <command> [args...]")
			fmt.Println("  gen mcp add --transport http <name> <url>")
			return nil
		}

		fmt.Printf("MCP Servers (%d configured):\n\n", len(configs))
		for name, config := range configs {
			transportType := config.GetType()
			var location string
			switch transportType {
			case mcp.TransportSTDIO:
				location = config.Command
				if len(config.Args) > 0 {
					location += " " + strings.Join(config.Args, " ")
				}
			case mcp.TransportHTTP, mcp.TransportSSE:
				location = config.URL
			}

			fmt.Printf("  %s [%s] (%s)\n", name, transportType, config.Scope)
			fmt.Printf("    %s\n", location)
		}

		return nil
	},
}

var mcpGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get details of an MCP server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cwd, _ := os.Getwd()
		loader := mcp.NewConfigLoader(cwd)
		configs, err := loader.LoadAll()
		if err != nil {
			return fmt.Errorf("failed to load configs: %w", err)
		}

		config, ok := configs[name]
		if !ok {
			return fmt.Errorf("server not found: %s", name)
		}

		// Pretty print the configuration
		data, _ := json.MarshalIndent(config, "", "  ")
		fmt.Printf("Server: %s\n", name)
		fmt.Printf("Scope: %s\n", config.Scope)
		fmt.Printf("Config:\n%s\n", string(data))

		return nil
	},
}

var mcpRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an MCP server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cwd, _ := os.Getwd()
		loader := mcp.NewConfigLoader(cwd)

		if err := loader.RemoveServerFromAll(name); err != nil {
			return fmt.Errorf("failed to remove server: %w", err)
		}

		fmt.Printf("Removed MCP server '%s'\n", name)
		return nil
	},
}

func parseScope(s string) mcp.Scope {
	switch strings.ToLower(s) {
	case "user", "global":
		return mcp.ScopeUser
	case "project":
		return mcp.ScopeProject
	default:
		return mcp.ScopeLocal
	}
}

// parseKeyValues parses a slice of "key=value" or "key:value" strings into a map
func parseKeyValues(items []string, sep string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	result := make(map[string]string, len(items))
	for _, item := range items {
		if key, value, ok := strings.Cut(item, sep); ok {
			result[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return result
}
