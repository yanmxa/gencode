package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/yanmxa/gencode/internal/ext/subagent"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/loop"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/core/prompt"
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
	agentRunCmd.Flags().IntVar(&agentRunOpts.maxTurns, "max-turns", 50, "Maximum conversation turns")

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
	store, _ := provider.NewStore()
	if store == nil {
		return fmt.Errorf("no provider store available")
	}

	currentModel := store.GetCurrentModel()
	var llmProvider provider.LLMProvider

	if currentModel != nil {
		p, err := provider.GetProvider(ctx, currentModel.Provider, currentModel.AuthMethod)
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
	if err := subagent.Initialize(cwd); err != nil {
		return fmt.Errorf("failed to initialize agent registry: %w", err)
	}

	// Get agent configuration
	agentCfg, ok := subagent.DefaultRegistry.Get(agentRunOpts.agentType)
	if !ok {
		return fmt.Errorf("unknown agent type: %s", agentRunOpts.agentType)
	}

	// Build tool set based on agent config
	toolSet := &tool.Set{}
	if agentCfg.Tools != nil {
		toolSet.Allow = []string(agentCfg.Tools)
	}

	// Set up the loop
	sys := prompt.Build(prompt.Config{
		Cwd:   cwd,
		IsGit: config.IsGitRepo(cwd),
	})

	loopClient := provider.NewLLM(llmProvider, modelID, 16384)

	lp, err := loop.NewLoop(loop.LoopConfig{
		System: sys,
		Client: loopClient,
		Tool:   toolSet,
		Cwd:    cwd,
	})
	if err != nil {
		return err
	}

	// Add user prompt
	lp.AddUser(agentRunOpts.prompt, nil)

	// Print status
	fmt.Printf("Agent: %s\n", agentRunOpts.agentType)
	fmt.Printf("Prompt: %s\n", agentRunOpts.prompt)
	fmt.Println("---")

	// Run turns
	maxTurns := agentRunOpts.maxTurns
	if maxTurns <= 0 {
		maxTurns = 50
	}

	totalTurns := 0
	for totalTurns < maxTurns {
		if ctx.Err() != nil {
			break
		}

		// Run one turn
		result, err := lp.Run(ctx, loop.RunOptions{
			MaxTurns: 1,
			OnResponse: func(resp *core.CompletionResponse) {
				if resp.Content != "" {
					fmt.Println(resp.Content)
				}
			},
			OnToolStart: func(tc core.ToolCall) bool {
				fmt.Printf("[%s] %s\n", tc.Name, tc.ID)
				return true
			},
		})
		totalTurns++

		if err != nil {
			return fmt.Errorf("agent failed: %w", err)
		}

		// Check for end_turn (agent completed its work)
		if result.StopReason == "end_turn" {
			break
		}
	}

	fmt.Printf("\n---\nDone: %d turns\n", totalTurns)
	return nil
}


