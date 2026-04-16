package runtime

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/message"
)

// Collect synchronously drains a stream into a CompletionResponse.
func Collect(ctx context.Context, ch <-chan message.StreamChunk) (*message.CompletionResponse, error) {
	var response message.CompletionResponse

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
			case message.ChunkTypeText:
				response.Content += chunk.Text
			case message.ChunkTypeThinking:
				response.Thinking += chunk.Text
			case message.ChunkTypeToolStart:
				response.ToolCalls = append(response.ToolCalls, message.ToolCall{
					ID:   chunk.ToolID,
					Name: chunk.ToolName,
				})
			case message.ChunkTypeToolInput:
				if len(response.ToolCalls) > 0 {
					idx := len(response.ToolCalls) - 1
					response.ToolCalls[idx].Input += chunk.Text
				}
			case message.ChunkTypeDone:
				if chunk.Response != nil {
					return chunk.Response, nil
				}
				return &response, nil
			case message.ChunkTypeError:
				return nil, chunk.Error
			}
		}
	}
}
