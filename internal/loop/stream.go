package loop

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/core"
)

// Collect synchronously drains a stream into a CompletionResponse.
func Collect(ctx context.Context, ch <-chan core.StreamChunk) (*core.CompletionResponse, error) {
	var response core.CompletionResponse

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case chunk, ok := <-ch:
			if !ok {
				if response.StopReason == "" {
					return nil, fmt.Errorf("stream closed without completion")
				}
				return &response, nil
			}

			switch chunk.Type {
			case core.ChunkTypeText:
				response.Content += chunk.Text
			case core.ChunkTypeThinking:
				response.Thinking += chunk.Text
			case core.ChunkTypeToolStart:
				response.ToolCalls = append(response.ToolCalls, core.ToolCall{
					ID:   chunk.ToolID,
					Name: chunk.ToolName,
				})
			case core.ChunkTypeToolInput:
				if len(response.ToolCalls) > 0 {
					idx := len(response.ToolCalls) - 1
					response.ToolCalls[idx].Input += chunk.Text
				}
			case core.ChunkTypeDone:
				if chunk.Response != nil {
					return chunk.Response, nil
				}
				return &response, nil
			case core.ChunkTypeError:
				return nil, chunk.Error
			}
		}
	}
}

// Stream starts an LLM stream and returns the chunk channel.
// It builds the system prompt and tool set from the loop's fields.
// The message slice is snapshotted to avoid a data race when the TUI
// goroutine appends messages while the streaming goroutine reads them.
func (l *Loop) Stream(ctx context.Context) <-chan core.StreamChunk {
	sysPrompt := l.System.Prompt()
	tools := l.Tool.Tools()
	msgs := make([]core.Message, len(l.messages))
	copy(msgs, l.messages)
	return l.Client.Stream(ctx, msgs, tools, sysPrompt)
}
