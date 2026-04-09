// Package openaicompat provides shared helpers for OpenAI-compatible providers
// (OpenAI, Moonshot, Alibaba/Qwen). All three use the openai-go SDK with the
// same message format; only model-specific parameters differ.
package openaicompat

import (
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
)

// ConvertMessages converts the internal message slice to OpenAI SDK format.
//
// convertAssistant is called for each assistant message, allowing callers to
// inject provider-specific fields (e.g. reasoning_content extra fields).
// Pass nil to use the default assistant conversion (no extra fields).
func ConvertMessages(
	msgs []message.Message,
	systemPrompt string,
	convertAssistant func(msg message.Message) openai.ChatCompletionMessageParamUnion,
) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs)+1)

	if systemPrompt != "" {
		out = append(out, openai.SystemMessage(systemPrompt))
	}

	for _, msg := range msgs {
		switch msg.Role {
		case message.RoleUser:
			out = append(out, convertUserMessage(msg))
		case message.RoleAssistant:
			if convertAssistant != nil {
				out = append(out, convertAssistant(msg))
			} else {
				out = append(out, DefaultAssistantMessage(msg))
			}
		default:
			out = append(out, openai.SystemMessage(msg.Content))
		}
	}
	return out
}

// convertUserMessage converts a user-role message (text, images, or tool result).
func convertUserMessage(msg message.Message) openai.ChatCompletionMessageParamUnion {
	if msg.ToolResult != nil {
		return openai.ToolMessage(msg.ToolResult.Content, msg.ToolResult.ToolCallID)
	}
	if len(msg.Images) > 0 {
		if parts := message.InterleavedContentParts(msg); parts != nil {
			oaiParts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(parts))
			for _, p := range parts {
				switch p.Type {
				case message.ContentPartText:
					oaiParts = append(oaiParts, openai.ChatCompletionContentPartUnionParam{
						OfText: &openai.ChatCompletionContentPartTextParam{Text: p.Text},
					})
				case message.ContentPartImage:
					dataURI := fmt.Sprintf("data:%s;base64,%s", p.Image.MediaType, p.Image.Data)
					oaiParts = append(oaiParts, openai.ChatCompletionContentPartUnionParam{
						OfImageURL: &openai.ChatCompletionContentPartImageParam{
							ImageURL: openai.ChatCompletionContentPartImageImageURLParam{URL: dataURI},
						},
					})
				}
			}
			return openai.ChatCompletionMessageParamUnion{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfArrayOfContentParts: oaiParts,
					},
				},
			}
		}
		parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(msg.Images)+1)
		for _, img := range msg.Images {
			dataURI := fmt.Sprintf("data:%s;base64,%s", img.MediaType, img.Data)
			parts = append(parts, openai.ChatCompletionContentPartUnionParam{
				OfImageURL: &openai.ChatCompletionContentPartImageParam{
					ImageURL: openai.ChatCompletionContentPartImageImageURLParam{URL: dataURI},
				},
			})
		}
		if msg.Content != "" {
			parts = append(parts, openai.ChatCompletionContentPartUnionParam{
				OfText: &openai.ChatCompletionContentPartTextParam{Text: msg.Content},
			})
		}
		return openai.ChatCompletionMessageParamUnion{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfArrayOfContentParts: parts,
				},
			},
		}
	}
	return openai.UserMessage(msg.Content)
}

// DefaultAssistantMessage converts an assistant message without extra fields.
// Use this for the base OpenAI provider; providers needing reasoning_content
// should implement their own assistant converter and pass it to ConvertMessages.
func DefaultAssistantMessage(msg message.Message) openai.ChatCompletionMessageParamUnion {
	if len(msg.ToolCalls) > 0 {
		var asstMsg openai.ChatCompletionAssistantMessageParam
		if msg.Content != "" {
			asstMsg.Content.OfString = openai.Opt(msg.Content)
		}
		asstMsg.ToolCalls = convertToolCallParams(msg.ToolCalls)
		return openai.ChatCompletionMessageParamUnion{OfAssistant: &asstMsg}
	}
	return openai.AssistantMessage(msg.Content)
}

// AssistantMessageWithReasoning converts an assistant message and sets
// reasoning_content as an extra field. Pass empty string to set the field
// to "" (some providers require this for all assistant messages).
func AssistantMessageWithReasoning(msg message.Message, reasoning string) openai.ChatCompletionMessageParamUnion {
	var asstMsg openai.ChatCompletionAssistantMessageParam
	if msg.Content != "" {
		asstMsg.Content.OfString = openai.Opt(msg.Content)
	}
	if len(msg.ToolCalls) > 0 {
		asstMsg.ToolCalls = convertToolCallParams(msg.ToolCalls)
	}
	asstMsg.SetExtraFields(map[string]any{"reasoning_content": reasoning})
	return openai.ChatCompletionMessageParamUnion{OfAssistant: &asstMsg}
}

// convertToolCallParams converts internal ToolCall slice to OpenAI SDK format.
func convertToolCallParams(calls []message.ToolCall) []openai.ChatCompletionMessageToolCallUnionParam {
	result := make([]openai.ChatCompletionMessageToolCallUnionParam, len(calls))
	for i, tc := range calls {
		result[i] = openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: tc.ID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      tc.Name,
					Arguments: tc.Input,
				},
			},
		}
	}
	return result
}

// ConvertTools converts generic provider.ToolSchema schemas to OpenAI SDK tool params.
func ConvertTools(tools []provider.ToolSchema) []openai.ChatCompletionToolUnionParam {
	result := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, t := range tools {
		var funcParams openai.FunctionParameters
		if props, ok := t.Parameters.(map[string]any); ok {
			funcParams = props
		}
		result = append(result, openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        t.Name,
					Description: openai.String(t.Description),
					Parameters:  funcParams,
				},
			},
		})
	}
	return result
}

// MapFinishReason normalizes OpenAI-style finish reasons to the canonical
// GenCode stop reason strings used by all providers.
//
//	"stop"       → "end_turn"
//	"tool_calls" → "tool_use"
//	"length"     → "max_tokens"
//	anything else is returned as-is
func MapFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return reason
	}
}

// ExtractReasoningContent parses the reasoning_content field from a raw JSON
// stream delta. This extension field is used by Moonshot (Kimi) and Alibaba
// (Qwen) thinking models and is not part of the standard OpenAI SDK struct.
// Returns empty string if the field is absent or the JSON is malformed.
func ExtractReasoningContent(rawJSON string) string {
	if rawJSON == "" {
		return ""
	}
	var delta map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &delta); err != nil {
		return ""
	}
	if rc, ok := delta["reasoning_content"].(string); ok {
		return rc
	}
	return ""
}
