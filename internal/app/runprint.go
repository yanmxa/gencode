package app

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/tool"
)

// runPrint sends a single message and streams the response to stdout.
func runPrint(userMessage string) error {
	ctx := context.Background()

	store, err := llm.NewStore()
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}

	var llmProvider llm.Provider
	var modelID string

	current := store.GetCurrentModel()
	if current != nil {
		p, err := llm.GetProvider(ctx, current.Provider, current.AuthMethod)
		if err != nil {
			return fmt.Errorf("provider %s (%s) not available: %w. Run 'gen' and use /provider to connect",
				current.Provider, current.AuthMethod, err)
		}
		llmProvider = p
		modelID = current.ModelID
	} else {
		for providerName, conn := range store.GetConnections() {
			p, err := llm.GetProvider(ctx, llm.Name(providerName), conn.AuthMethod)
			if err == nil {
				llmProvider = p
				modelID = setting.DefaultModel(providerName, string(conn.AuthMethod))
				break
			}
		}
	}

	if llmProvider == nil {
		return fmt.Errorf("no provider connected. Run 'gen' and use /provider to connect")
	}

	completionOpts := llm.CompletionOptions{
		Model:        modelID,
		MaxTokens:    setting.DefaultMaxTokens,
		SystemPrompt: setting.DefaultSystemPrompt,
		Messages:     []core.Message{core.UserMessage(userMessage, nil)},
		Tools:        tool.GetToolSchemas(),
	}

	streamChan := llmProvider.Stream(ctx, completionOpts)
	for chunk := range streamChan {
		switch chunk.Type {
		case llm.ChunkTypeText:
			fmt.Print(chunk.Text)
		case llm.ChunkTypeError:
			return chunk.Error
		case llm.ChunkTypeDone:
			fmt.Println()
		}
	}

	return nil
}
