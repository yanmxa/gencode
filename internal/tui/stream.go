// LLM streaming helpers: channel reading and stream continuation commands.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/message"
)

func (m model) continueWithToolResults() tea.Cmd {
	return func() tea.Msg {
		return streamContinueMsg{
			messages: m.convertMessagesToProvider(),
			modelID:  m.getModelID(),
		}
	}
}

func (m model) waitForChunk() tea.Cmd {
	return func() tea.Msg {
		if m.streamChan == nil {
			return streamDoneMsg{}
		}
		chunk, ok := <-m.streamChan
		if !ok {
			return streamChunkMsg{done: true}
		}

		return convertChunkToMsg(chunk)
	}
}

// convertChunkToMsg converts a stream chunk to a tea message.
func convertChunkToMsg(chunk message.StreamChunk) streamChunkMsg {
	switch chunk.Type {
	case message.ChunkTypeText:
		return streamChunkMsg{text: chunk.Text}
	case message.ChunkTypeThinking:
		return streamChunkMsg{thinking: chunk.Text}
	case message.ChunkTypeDone:
		return convertDoneChunk(chunk)
	case message.ChunkTypeError:
		return streamChunkMsg{err: chunk.Error}
	case message.ChunkTypeToolStart:
		return streamChunkMsg{buildingToolName: chunk.ToolName}
	default:
		return streamChunkMsg{}
	}
}

// convertDoneChunk handles the done chunk type.
func convertDoneChunk(chunk message.StreamChunk) streamChunkMsg {
	var usage *message.Usage
	if chunk.Response != nil {
		usage = &chunk.Response.Usage
	}

	if chunk.Response != nil && len(chunk.Response.ToolCalls) > 0 {
		return streamChunkMsg{
			done:       true,
			toolCalls:  chunk.Response.ToolCalls,
			stopReason: chunk.Response.StopReason,
			usage:      usage,
		}
	}

	return streamChunkMsg{done: true, usage: usage}
}
