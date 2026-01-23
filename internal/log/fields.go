package log

import (
	"encoding/json"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/yanmxa/gencode/internal/provider"
)

// messageMarshaler wraps a Message for zap logging
type messageMarshaler provider.Message

func (m messageMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("role", m.Role)
	enc.AddString("content", m.Content)
	if len(m.ToolCalls) > 0 {
		_ = enc.AddArray("tool_calls", toolCallsMarshaler(m.ToolCalls))
	}
	if m.ToolResult != nil {
		_ = enc.AddObject("tool_result", toolResultMarshaler(*m.ToolResult))
	}
	return nil
}

// messagesMarshaler wraps a slice of Messages for zap logging
type messagesMarshaler []provider.Message

func (m messagesMarshaler) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, msg := range m {
		_ = enc.AppendObject(messageMarshaler(msg))
	}
	return nil
}

// MessagesField creates a zap field for messages
func MessagesField(messages []provider.Message) zap.Field {
	return zap.Array("messages", messagesMarshaler(messages))
}

// toolMarshaler wraps a Tool for zap logging
type toolMarshaler provider.Tool

func (t toolMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("name", t.Name)
	enc.AddString("description", t.Description)
	// Marshal parameters as JSON string for readability
	if t.Parameters != nil {
		paramsJSON, err := json.Marshal(t.Parameters)
		if err == nil {
			enc.AddString("parameters", string(paramsJSON))
		}
	}
	return nil
}

// toolsMarshaler wraps a slice of Tools for zap logging
type toolsMarshaler []provider.Tool

func (t toolsMarshaler) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, tool := range t {
		_ = enc.AppendObject(toolMarshaler(tool))
	}
	return nil
}

// ToolsField creates a zap field for tools
func ToolsField(tools []provider.Tool) zap.Field {
	return zap.Array("tools", toolsMarshaler(tools))
}

// toolCallMarshaler wraps a ToolCall for zap logging
type toolCallMarshaler provider.ToolCall

func (tc toolCallMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("id", tc.ID)
	enc.AddString("name", tc.Name)
	enc.AddString("input", tc.Input)
	return nil
}

// toolCallsMarshaler wraps a slice of ToolCalls for zap logging
type toolCallsMarshaler []provider.ToolCall

func (tc toolCallsMarshaler) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, call := range tc {
		_ = enc.AppendObject(toolCallMarshaler(call))
	}
	return nil
}

// ToolCallsField creates a zap field for tool calls
func ToolCallsField(toolCalls []provider.ToolCall) zap.Field {
	return zap.Array("tool_calls", toolCallsMarshaler(toolCalls))
}

// toolResultMarshaler wraps a ToolResult for zap logging
type toolResultMarshaler provider.ToolResult

func (tr toolResultMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("tool_call_id", tr.ToolCallID)
	enc.AddString("content", tr.Content)
	enc.AddBool("is_error", tr.IsError)
	return nil
}

// usageMarshaler wraps Usage for zap logging
type usageMarshaler provider.Usage

func (u usageMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt("input_tokens", u.InputTokens)
	enc.AddInt("output_tokens", u.OutputTokens)
	return nil
}

// UsageField creates a zap field for usage
func UsageField(usage provider.Usage) zap.Field {
	return zap.Object("usage", usageMarshaler(usage))
}
