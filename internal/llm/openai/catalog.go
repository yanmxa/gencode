package openai

import (
	"strings"

	"github.com/openai/openai-go/v3/shared"
	"github.com/yanmxa/gencode/internal/llm"
)

type reasoningProfile struct {
	off     shared.ReasoningEffort
	normal  shared.ReasoningEffort
	high    shared.ReasoningEffort
	ultra   shared.ReasoningEffort
	summary bool
}

func openAIReasoningProfile(modelID string) (reasoningProfile, bool) {
	model := normalizeModelID(modelID)
	switch {
	case strings.HasPrefix(model, "gpt-5-pro"):
		return highOnlyProfile(), true
	case strings.HasPrefix(model, "gpt-5.5"),
		strings.HasPrefix(model, "gpt-5.4"),
		strings.HasPrefix(model, "gpt-6"):
		return reasoningProfile{
			off:     shared.ReasoningEffortNone,
			normal:  shared.ReasoningEffortLow,
			high:    shared.ReasoningEffortMedium,
			ultra:   shared.ReasoningEffortXhigh,
			summary: true,
		}, true
	case strings.HasPrefix(model, "gpt-5"),
		strings.HasPrefix(model, "o1"),
		strings.HasPrefix(model, "o3"),
		strings.HasPrefix(model, "o4"),
		strings.Contains(model, "codex"):
		return highOnlyProfile(), true
	default:
		return reasoningProfile{}, false
	}
}

func normalizeModelID(modelID string) string {
	return strings.ToLower(strings.TrimSpace(modelID))
}

func openAIModelInfo(modelID string) llm.ModelInfo {
	return llm.ModelInfo{
		ID:          modelID,
		Name:        modelID,
		DisplayName: modelID,
	}
}

func highOnlyProfile() reasoningProfile {
	return reasoningProfile{
		off:     shared.ReasoningEffortHigh,
		normal:  shared.ReasoningEffortHigh,
		high:    shared.ReasoningEffortHigh,
		ultra:   shared.ReasoningEffortHigh,
		summary: true,
	}
}
