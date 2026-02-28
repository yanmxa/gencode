// Package app provides the unified entry point for interactive and non-interactive modes.
package app

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/options"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/tui"
)

// RunWithOptions routes to either print mode or interactive TUI.
func RunWithOptions(opts options.RunOptions) error {
	if opts.Print != "" {
		return runNonInteractive(opts.Print)
	}

	p, err := tui.NewProgram(opts)
	if err != nil {
		return err
	}

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}
	return nil
}

// runNonInteractive sends a single message and streams the response to stdout.
func runNonInteractive(userMessage string) error {
	ctx := context.Background()

	store, err := provider.NewStore()
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}

	var llmProvider provider.LLMProvider
	var modelID string

	current := store.GetCurrentModel()
	if current != nil {
		p, err := provider.GetProvider(ctx, current.Provider, current.AuthMethod)
		if err != nil {
			return fmt.Errorf("provider %s (%s) not available: %w. Run 'gen' and use /provider to connect",
				current.Provider, current.AuthMethod, err)
		}
		llmProvider = p
		modelID = current.ModelID
	} else {
		for providerName, conn := range store.GetConnections() {
			p, err := provider.GetProvider(ctx, provider.Provider(providerName), conn.AuthMethod)
			if err == nil {
				llmProvider = p
				modelID = options.DefaultModel(providerName, conn.AuthMethod)
				break
			}
		}
	}

	if llmProvider == nil {
		return fmt.Errorf("no provider connected. Run 'gen' and use /provider to connect")
	}

	completionOpts := options.NewCompletionOptions(modelID, userMessage)

	streamChan := llmProvider.Stream(ctx, completionOpts)
	for chunk := range streamChan {
		switch chunk.Type {
		case message.ChunkTypeText:
			fmt.Print(chunk.Text)
		case message.ChunkTypeError:
			return chunk.Error
		case message.ChunkTypeDone:
			fmt.Println()
		}
	}

	return nil
}
