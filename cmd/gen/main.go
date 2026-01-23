package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"github.com/myan/gencode/internal/log"
	"github.com/myan/gencode/internal/provider"
	"github.com/myan/gencode/internal/tool"
	"github.com/myan/gencode/internal/tui"

	// Import providers for registration
	_ "github.com/myan/gencode/internal/provider/anthropic"
	_ "github.com/myan/gencode/internal/provider/google"
	_ "github.com/myan/gencode/internal/provider/openai"
)

var (
	version = "0.1.0"
)

func init() {
	// Load .env file if it exists (silent fail if not found)
	_ = godotenv.Load()

	// Initialize logging (enabled via GEN_DEBUG=1)
	_ = log.Init()
}

func main() {
	defer log.Sync()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "gen [message]",
	Short: "Gen - AI coding assistant for the terminal",
	Long: `Gen is an open-source AI assistant for the terminal.
Extensible tools, customizable prompts, multi-provider support.

Non-interactive mode:
  gen "your message"       Send a message directly
  echo "message" | gen     Send a message via stdin
  gen -p "prompt"          Use a custom prompt`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check for non-interactive input
		message := getInputMessage(args)

		if message != "" {
			// Non-interactive mode
			if err := runNonInteractive(message); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}

		// Interactive mode (TUI)
		if err := tui.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

// promptFlag is the custom prompt flag
var promptFlag string

func init() {
	rootCmd.Flags().StringVarP(&promptFlag, "prompt", "p", "", "Custom prompt to send")
}

// getInputMessage gets input from args, flags, or stdin
func getInputMessage(args []string) string {
	// Check for -p/--prompt flag
	if promptFlag != "" {
		return promptFlag
	}

	// Check for positional arguments
	if len(args) > 0 {
		return strings.Join(args, " ")
	}

	// Check if stdin has data (non-interactive pipe)
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Data is being piped in
		reader := bufio.NewReader(os.Stdin)
		data, err := io.ReadAll(reader)
		if err == nil && len(data) > 0 {
			return strings.TrimSpace(string(data))
		}
	}

	return ""
}

// runNonInteractive runs in non-interactive mode
func runNonInteractive(message string) error {
	ctx := context.Background()

	// Load store and get connected provider
	store, err := provider.NewStore()
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}

	var llmProvider provider.LLMProvider
	var model string

	// Try to use current model setting first
	current := store.GetCurrentModel()
	if current != nil {
		p, err := provider.GetProvider(ctx, current.Provider, current.AuthMethod)
		if err != nil {
			return fmt.Errorf("provider %s (%s) not available: %w. Run 'gen' and use /provider to connect",
				current.Provider, current.AuthMethod, err)
		}
		llmProvider = p
		model = current.ModelID
	} else {
		// Fall back to first available provider with default model
		connections := store.GetConnections()
		for providerName, conn := range connections {
			p, err := provider.GetProvider(ctx, provider.Provider(providerName), conn.AuthMethod)
			if err == nil {
				llmProvider = p
				model = getDefaultModel(providerName, conn.AuthMethod)
				break
			}
		}
	}

	if llmProvider == nil {
		return fmt.Errorf("no provider connected. Run 'gen' and use /provider to connect")
	}

	// Send message
	opts := provider.CompletionOptions{
		Model:        model,
		MaxTokens:    8192,
		SystemPrompt: "You are a helpful AI coding assistant.",
		Messages: []provider.Message{
			{Role: "user", Content: message},
		},
		Tools: tool.GetToolSchemas(),
	}

	// Stream response
	streamChan := llmProvider.Stream(ctx, opts)

	for chunk := range streamChan {
		switch chunk.Type {
		case provider.ChunkTypeText:
			fmt.Print(chunk.Text)
		case provider.ChunkTypeError:
			return chunk.Error
		case provider.ChunkTypeDone:
			fmt.Println() // Final newline
		}
	}

	return nil
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("gen version %s\n", version)
	},
}

var helpCmd = &cobra.Command{
	Use:   "help",
	Short: "Show help information",
	Long:  "Display help information about Gen and its commands.",
	Run: func(cmd *cobra.Command, args []string) {
		printHelp()
	},
}

func printHelp() {
	help := `
Gen - AI coding assistant for the terminal

Usage:
  gen [message]              Non-interactive mode with message
  gen                        Start interactive chat mode
  gen [command]              Run a command

Non-interactive Mode:
  gen "your message"         Send a message directly
  echo "message" | gen       Send a message via stdin
  gen -p "prompt"            Use a custom prompt

Commands:
  version      Print the version number
  help         Show this help message

Interactive Mode:
  Enter        Send message
  Alt+Enter    Insert newline
  Up/Down      Navigate input history
  Esc          Stop AI response
  Ctrl+C       Clear input / Quit

Interactive Commands:
  /provider    Select and connect to a provider
  /model       Select a model
  /clear       Clear chat history
  /help        Show help

Examples:
  gen                        Start interactive chat
  gen "Explain this code"    Quick question
  cat file.go | gen "Review" Review file via pipe
  gen version                Show version

For more information, visit: https://github.com/myan/gencode
`
	fmt.Println(help)
}

// getDefaultModel returns the default model for a provider and auth method
func getDefaultModel(providerName string, authMethod provider.AuthMethod) string {
	switch providerName {
	case "anthropic":
		if authMethod == provider.AuthVertex {
			return "claude-sonnet-4-5@20250929" // Vertex AI format
		}
		return "claude-sonnet-4-20250514" // API key format
	case "openai":
		return "gpt-4o"
	case "google":
		return "gemini-2.0-flash"
	default:
		return "claude-sonnet-4-20250514"
	}
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(helpCmd)
	rootCmd.SetHelpCommand(helpCmd)
}
