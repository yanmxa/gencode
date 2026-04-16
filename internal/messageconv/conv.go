// Package messageconv provides conversion between message.Message and core.Message types.
// It exists as a bridge so that the message package remains a leaf with no internal dependencies.
package messageconv

import (
	"encoding/json"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/message"
)

func ToCore(m message.Message) core.Message {
	msg := core.Message{
		Content:           m.Content,
		DisplayContent:    m.DisplayContent,
		Thinking:          m.Thinking,
		ThinkingSignature: m.ThinkingSignature,
	}
	switch m.Role {
	case message.RoleUser:
		msg.Role = core.RoleUser
		msg.Images = imagesToCore(m.Images)
	case message.RoleAssistant:
		msg.Role = core.RoleAssistant
		msg.ToolCalls = toolCallsToCore(m.ToolCalls)
	case message.RoleToolResult:
		msg.Role = core.RoleTool
		if m.ToolResult != nil {
			msg.ToolResult = &core.ToolResult{
				ToolCallID: m.ToolResult.ToolCallID,
				ToolName:   m.ToolResult.ToolName,
				Content:    m.ToolResult.Content,
				IsError:    m.ToolResult.IsError,
			}
			msg.Content = m.ToolResult.Content
		}
	}
	return msg
}

func ToCoreSlice(msgs []message.Message) []core.Message {
	out := make([]core.Message, len(msgs))
	for i, m := range msgs {
		out[i] = ToCore(m)
	}
	return out
}

func FromCoreSlice(msgs []core.Message) []message.Message {
	out := make([]message.Message, len(msgs))
	for i, m := range msgs {
		out[i] = fromCore(m)
	}
	return out
}

func fromCore(m core.Message) message.Message {
	msg := message.Message{
		Content:           m.Content,
		DisplayContent:    m.DisplayContent,
		Thinking:          m.Thinking,
		ThinkingSignature: m.ThinkingSignature,
	}
	switch m.Role {
	case core.RoleUser:
		msg.Role = message.RoleUser
		msg.Images = imagesFromCore(m.Images)
	case core.RoleAssistant:
		msg.Role = message.RoleAssistant
		msg.ToolCalls = toolCallsFromCore(m.ToolCalls)
	case core.RoleTool:
		msg.Role = message.RoleToolResult
		if m.ToolResult != nil {
			msg.ToolResult = &message.ToolResult{
				ToolCallID: m.ToolResult.ToolCallID,
				ToolName:   m.ToolResult.ToolName,
				Content:    m.ToolResult.Content,
				IsError:    m.ToolResult.IsError,
			}
		}
	}
	return msg
}

func toolCallsToCore(calls []message.ToolCall) []core.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]core.ToolCall, len(calls))
	for i, tc := range calls {
		input := make(map[string]any)
		if tc.Input != "" {
			if err := json.Unmarshal([]byte(tc.Input), &input); err != nil {
				input = map[string]any{"_raw": tc.Input}
			}
		}
		out[i] = core.ToolCall{
			ID:    tc.ID,
			Name:  tc.Name,
			Input: input,
		}
	}
	return out
}

func toolCallsFromCore(calls []core.ToolCall) []message.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]message.ToolCall, len(calls))
	for i, tc := range calls {
		inputJSON, _ := json.Marshal(tc.Input)
		out[i] = message.ToolCall{
			ID:    tc.ID,
			Name:  tc.Name,
			Input: string(inputJSON),
		}
	}
	return out
}

func imagesToCore(imgs []message.ImageData) []core.Image {
	if len(imgs) == 0 {
		return nil
	}
	out := make([]core.Image, len(imgs))
	for i, img := range imgs {
		out[i] = core.Image{
			MediaType: img.MediaType,
			Data:      img.Data,
			FileName:  img.FileName,
			Size:      img.Size,
		}
	}
	return out
}

func imagesFromCore(imgs []core.Image) []message.ImageData {
	if len(imgs) == 0 {
		return nil
	}
	out := make([]message.ImageData, len(imgs))
	for i, img := range imgs {
		out[i] = message.ImageData{
			MediaType: img.MediaType,
			Data:      img.Data,
			FileName:  img.FileName,
			Size:      img.Size,
		}
	}
	return out
}
