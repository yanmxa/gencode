package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"github.com/yanmxa/gencode/internal/app"
	appsession "github.com/yanmxa/gencode/internal/app/session"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/options"

	// Import providers for registration
	_ "github.com/yanmxa/gencode/internal/provider/alibaba"
	_ "github.com/yanmxa/gencode/internal/provider/anthropic"
	_ "github.com/yanmxa/gencode/internal/provider/google"
	_ "github.com/yanmxa/gencode/internal/provider/moonshot"
	_ "github.com/yanmxa/gencode/internal/provider/openai"
)

var version = "1.11.2"

// cliOpts holds all CLI flag values in one place.
var cliOpts struct {
	print  string // -p/--print: non-interactive print mode
	plan   bool
	cont   bool // --continue
	resume bool // --resume

	fork      bool // --fork: fork from continued/resumed session
	pluginDir string
}

func init() {
	// Load .env file if it exists (silent fail if not found)
	_ = godotenv.Load()

	// Initialize logging (enabled via GEN_DEBUG=1)
	_ = log.Init()

	// Set app version for session entries.
	appsession.AppVersion = version

	// Register flags
	rootCmd.Flags().StringVarP(&cliOpts.print, "print", "p", "", "Non-interactive print mode with prompt")
	rootCmd.Flags().BoolVar(&cliOpts.plan, "plan", false, "Enter plan mode")
	rootCmd.Flags().BoolVarP(&cliOpts.cont, "continue", "c", false, "Resume the most recent session")
	rootCmd.Flags().BoolVarP(&cliOpts.resume, "resume", "r", false, "Select and resume a previous session")
	rootCmd.Flags().BoolVar(&cliOpts.fork, "fork", false, "Fork from the continued/resumed session into a new session")
	rootCmd.Flags().StringVar(&cliOpts.pluginDir, "plugin-dir", "", "Load plugins from a specific directory")

	// Register subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(helpCmd)
	rootCmd.SetHelpCommand(helpCmd)
	rootCmd.AddCommand(mcpCmd)
}

func main() {
	defer func() { _ = log.Sync() }()

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
  gen -p "your prompt"     Print response and exit
  echo "msg" | gen -p ""   Pipe stdin in print mode`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		printPrompt := cliOpts.print
		if printPrompt == "" {
			printPrompt = readStdin()
		}

		// When -r is used with an argument, treat it as a session ID
		var resumeID string
		if cliOpts.resume && len(args) > 0 {
			resumeID = args[0]
			args = args[1:]
		}

		prompt := strings.Join(args, " ")

		opts := options.RunOptions{
			Print:     printPrompt,
			Prompt:    prompt,
			PluginDir: cliOpts.pluginDir,
			PlanMode:  cliOpts.plan,
			Continue:  cliOpts.cont,
			Resume:    cliOpts.resume,
			ResumeID:  resumeID,
			Fork:      cliOpts.fork,
		}
		if err := app.RunWithOptions(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

// readStdin returns piped stdin data, or empty string if stdin is a terminal.
func readStdin() string {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		reader := bufio.NewReader(os.Stdin)
		data, err := io.ReadAll(reader)
		if err == nil && len(data) > 0 {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
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
  gen                        Start interactive chat mode
  gen "message"              Interactive mode with initial prompt
  gen -p "prompt"            Non-interactive print mode
  gen [command]              Run a command

Print Mode (non-interactive):
  gen -p "your prompt"       Print response and exit
  echo "data" | gen -p "analyze"  Pipe stdin with prompt

Interactive Mode:
  gen                        Start chat
  gen "Explain this code"    Start chat with initial prompt
  gen --plan "design login"  Start in plan mode

Session:
  gen -c, --continue         Resume the most recent session
  gen -r, --resume           Select and resume a previous session
  gen -r <session-id>        Resume a specific session by ID
  gen -c --fork              Fork the most recent session into a new one
  gen -r --fork              Select a session and fork it
  gen --plugin-dir <path>    Load plugins from a specific directory

Commands:
  version      Print the version number
  agent run    Run a headless agent
  help         Show this help message

Keybindings:
  Enter        Send message
  Alt+Enter    Insert newline
  Up/Down      Navigate input history
  Esc          Stop AI response
  Ctrl+T       Toggle task list display
  Ctrl+C       Clear input / Quit

Slash Commands:
  /provider    Select and connect to a provider
  /model       Select a model
  /clear       Clear chat history
  /help        Show help

Examples:
  gen                        Start interactive chat
  gen "Explain this code"    Interactive with initial prompt
  gen -p "Explain this code" Print response and exit
  gen --plan "design login"  Plan mode
  gen -c                     Resume previous session
  gen version                Show version

For more information, visit: https://github.com/yanmxa/gencode
`
	fmt.Println(help)
}
