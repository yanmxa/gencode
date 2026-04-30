package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/core/system"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/tool"
)

var agentRunOpts struct {
	agentType string
	prompt    string
	model     string
	maxTurns  int
}

func init() {
	agentRunCmd.Flags().StringVar(&agentRunOpts.agentType, "type", "", "Agent type to run")
	agentRunCmd.Flags().StringVar(&agentRunOpts.prompt, "prompt", "", "Task prompt")
	agentRunCmd.Flags().StringVar(&agentRunOpts.model, "model", "", "Model override")
	agentRunCmd.Flags().IntVar(&agentRunOpts.maxTurns, "max-turns", 100, "Maximum conversation turns")

	agentCmd.AddCommand(agentRunCmd)
	rootCmd.AddCommand(agentCmd)
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent management commands",
}

var agentRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a headless agent",
	Long: `Run an agent in headless mode without TUI.

Example:
  gen agent run --type Explore --prompt "find main.go"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if agentRunOpts.agentType == "" {
			return fmt.Errorf("--type is required")
		}
		if agentRunOpts.prompt == "" {
			return fmt.Errorf("--prompt is required")
		}
		return runHeadlessAgent()
	},
}

// runHeadlessAgent executes an agent in headless mode (no TUI).
func runHeadlessAgent() error {
	cwd, _ := os.Getwd()

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down agent...")
		cancel()
	}()

	// Initialize provider
	store, _ := llm.NewStore()
	if store == nil {
		return fmt.Errorf("no provider store available")
	}

	currentModel := store.GetCurrentModel()
	var llmProvider llm.Provider

	if currentModel != nil {
		p, err := llm.GetProvider(ctx, currentModel.Provider, currentModel.AuthMethod)
		if err != nil {
			return fmt.Errorf("failed to connect provider: %w", err)
		}
		llmProvider = p
	}
	if llmProvider == nil {
		return fmt.Errorf("no provider available")
	}

	modelID := ""
	if currentModel != nil {
		modelID = currentModel.ModelID
	}
	if agentRunOpts.model != "" {
		modelID = agentRunOpts.model
	}

	// Initialize agent registry
	if err := subagent.Initialize(subagent.Options{CWD: cwd}); err != nil {
		return fmt.Errorf("failed to initialize agent registry: %w", err)
	}

	// Get agent configuration
	agentCfg, ok := subagent.Default().Get(agentRunOpts.agentType)
	if !ok {
		return fmt.Errorf("unknown agent type: %s", agentRunOpts.agentType)
	}

	// Build tool set based on agent config
	toolSet := &tool.Set{}
	if agentCfg.Tools != nil {
		toolSet.Allow = []string(agentCfg.Tools)
	}

	sys := system.Build(system.Config{
		Cwd:   cwd,
		IsGit: setting.IsGitRepo(cwd),
	})

	client := llm.NewClient(llmProvider, modelID, 16384)

	schemas := toolSet.Tools()
	tools := tool.AdaptToolRegistry(schemas, func() string { return cwd })

	maxTurns := agentRunOpts.maxTurns
	if maxTurns <= 0 {
		maxTurns = 100
	}

	ag := core.NewAgent(core.Config{
		LLM:       client,
		System:    sys,
		Tools:     tools,
		CWD:       cwd,
		MaxTurns:  maxTurns,
		OutboxBuf: -1,
	})

	ag.Append(ctx, core.UserMessage(agentRunOpts.prompt, nil))

	fmt.Printf("Agent: %s\n", agentRunOpts.agentType)
	fmt.Printf("Prompt: %s\n", agentRunOpts.prompt)
	fmt.Println("---")

	result, err := ag.ThinkAct(ctx)
	if err != nil {
		return fmt.Errorf("agent failed: %w", err)
	}

	if result.Content != "" {
		fmt.Println(result.Content)
	}

	fmt.Printf("\n---\nDone: %d turns, %d tool uses\n", result.Turns, result.ToolUses)
	return nil
}
