package core

import "context"

type contextKey string

const toolCallIDKey contextKey = "tool_call_id"

func WithToolCallID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, toolCallIDKey, id)
}

func ToolCallIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(toolCallIDKey).(string); ok {
		return id
	}
	return ""
}
