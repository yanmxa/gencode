package app

import "github.com/yanmxa/gencode/internal/message"

func (m *model) buildPromptSuggestionRequest() (promptSuggestionRequest, bool) {
	if m.loop.Client == nil {
		return promptSuggestionRequest{}, false
	}

	assistantCount := 0
	for _, msg := range m.conv.Messages {
		if msg.Role == message.RoleAssistant {
			assistantCount++
		}
	}
	if assistantCount < 2 {
		return promptSuggestionRequest{}, false
	}

	startIdx := 0
	if len(m.conv.Messages) > maxSuggestionMessages {
		startIdx = len(m.conv.Messages) - maxSuggestionMessages
	}
	msgs := m.conv.ConvertToProviderFrom(startIdx)
	msgs = append(msgs, message.Message{
		Role:    message.RoleUser,
		Content: suggestionUserPrompt,
	})

	return promptSuggestionRequest{
		Client:       m.loop.Client,
		Messages:     msgs,
		SystemPrompt: suggestionSystemPrompt,
		UserPrompt:   suggestionUserPrompt,
		MaxTokens:    60,
	}, true
}

func (m *model) buildTokenLimitFetchRequest() tokenLimitFetchRequest {
	return tokenLimitFetchRequest{
		LLM:          m.provider.LLM,
		Store:        m.provider.Store,
		CurrentModel: m.provider.CurrentModel,
		ModelID:      m.getModelID(),
		Cwd:          m.cwd,
	}
}

func (m *model) buildCompactRequest(focus, trigger string) compactRequest {
	return compactRequest{
		Client:         m.loop.Client,
		Messages:       m.conv.ConvertToProvider(),
		SessionSummary: m.session.Summary,
		Focus:          focus,
		HookEngine:     m.hookEngine,
		Trigger:        trigger,
	}
}
