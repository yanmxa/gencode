package app

import (
	"context"
	"strings"
	"testing"

	appoutput "github.com/yanmxa/gencode/internal/app/output"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
)

func newTokenLimitStore(t *testing.T) *llm.Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	store, err := llm.NewStore()
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	return store
}

func TestHandleTokenLimitCommand_SetAndShowCustomOverride(t *testing.T) {
	store := newTokenLimitStore(t)
	m := &model{}
	setProviderDomainForTest(m, store, "gpt-5", llm.OpenAI, llm.AuthAPIKey)

	result, cmd, err := handleTokenLimitCommand(context.Background(), m, "200000 32000")
	if err != nil {
		t.Fatalf("handleTokenLimitCommand(set) error = %v", err)
	}
	if cmd != nil {
		t.Fatal("did not expect follow-up cmd when setting limits")
	}
	if !strings.Contains(result, "Set token limits for gpt-5") {
		t.Fatalf("unexpected set result: %q", result)
	}
	in, out, ok := store.GetTokenLimit("gpt-5")
	if !ok || in != 200000 || out != 32000 {
		t.Fatalf("unexpected persisted override: in=%d out=%d ok=%v", in, out, ok)
	}

	m.runtime.InputTokens = 50000
	result, cmd, err = handleTokenLimitCommand(context.Background(), m, "")
	if err != nil {
		t.Fatalf("handleTokenLimitCommand(show) error = %v", err)
	}
	if cmd != nil {
		t.Fatal("did not expect fetch cmd for custom override")
	}
	if !strings.Contains(result, "(custom override)") {
		t.Fatalf("expected custom override marker, got %q", result)
	}
	if !strings.Contains(result, "Current usage: 50.0k tokens (25.0%)") {
		t.Fatalf("expected usage summary, got %q", result)
	}
}

func TestHandleTokenLimitCommand_UsesCachedModelLimitsWhenNoOverride(t *testing.T) {
	store := newTokenLimitStore(t)
	if err := store.CacheModels(llm.Anthropic, llm.AuthAPIKey, []llm.ModelInfo{
		{ID: "claude-sonnet", InputTokenLimit: 200000, OutputTokenLimit: 16000},
	}); err != nil {
		t.Fatalf("CacheModels() error = %v", err)
	}

	m := &model{}
	setProviderDomainForTest(m, store, "claude-sonnet", llm.Anthropic, llm.AuthAPIKey)

	result, cmd, err := handleTokenLimitCommand(context.Background(), m, "")
	if err != nil {
		t.Fatalf("handleTokenLimitCommand() error = %v", err)
	}
	if cmd != nil {
		t.Fatal("did not expect fetch cmd when cached limits exist")
	}
	if !strings.Contains(result, "Input:  200.0k tokens") || !strings.Contains(result, "Output: 16.0k tokens") {
		t.Fatalf("unexpected cached limit display: %q", result)
	}
	if strings.Contains(result, "custom override") {
		t.Fatalf("did not expect custom marker for cached limits: %q", result)
	}
}

func TestHandleTokenLimitCommand_TriggersFetchWhenLimitsUnknown(t *testing.T) {
	store := newTokenLimitStore(t)
	m := &model{
		cwd:         "/repo",
		agentOutput: appoutput.New(80, nil),
	}
	setProviderDomainForTest(m, store, "gpt-unknown", llm.OpenAI, llm.AuthAPIKey)

	result, cmd, err := handleTokenLimitCommand(context.Background(), m, "")
	if err != nil {
		t.Fatalf("handleTokenLimitCommand() error = %v", err)
	}
	if result != "" {
		t.Fatalf("expected empty immediate result while fetching, got %q", result)
	}
	if cmd == nil {
		t.Fatal("expected fetch command when no limits are known")
	}
	if !m.userInput.Provider.FetchingLimits {
		t.Fatal("expected FetchingLimits to be set")
	}
}

func TestHandleTokenLimitCommand_ValidationAndModelFallbacks(t *testing.T) {
	m := &model{}
	result, cmd, err := handleTokenLimitCommand(context.Background(), m, "")
	if err != nil {
		t.Fatalf("handleTokenLimitCommand(no model) error = %v", err)
	}
	if cmd != nil || !strings.Contains(result, "No model selected") {
		t.Fatalf("unexpected no-model response: result=%q cmd=%v", result, cmd != nil)
	}

	store := newTokenLimitStore(t)
	m = &model{}
	setProviderDomainForTest(m, store, "gpt-5", llm.OpenAI, llm.AuthAPIKey)
	result, cmd, err = handleTokenLimitCommand(context.Background(), m, "abc 123")
	if err != nil {
		t.Fatalf("handleTokenLimitCommand(bad args) error = %v", err)
	}
	if cmd != nil || !strings.Contains(result, "Usage:") {
		t.Fatalf("unexpected bad-args response: result=%q cmd=%v", result, cmd != nil)
	}

	if got := (&model{}).getModelID(); got != "claude-sonnet-4-20250514" {
		t.Fatalf("default model ID = %q", got)
	}

	m = &model{}
	m.runtime.ThinkingLevel = llm.ThinkingNormal
	m.runtime.ThinkingOverride = llm.ThinkingUltra
	if got := m.effectiveThinkingLevel(); got != llm.ThinkingUltra {
		t.Fatalf("effectiveThinkingLevel() = %v, want %v", got, llm.ThinkingUltra)
	}
}

func setProviderDomainForTest(m *model, store *llm.Store, modelID string, p llm.Name, auth llm.AuthMethod) {
	m.runtime.ProviderStore = store
	m.runtime.CurrentModel = &llm.CurrentModelInfo{
		ModelID:    modelID,
		Provider:   p,
		AuthMethod: auth,
	}
}

func TestHandleCompactResultStoresVisibleSuccessState(t *testing.T) {
	m := &model{
		conv: appoutput.ConversationModel{
			Messages: []core.ChatMessage{
				{Role: core.RoleUser, Content: "one"},
				{Role: core.RoleAssistant, Content: "two"},
			},
			Compact: appoutput.CompactState{
				Active: true,
			},
		},
	}

	cmd := m.handleCompactResult(appoutput.CompactResultMsg{
		Summary:       "summary",
		OriginalCount: 2,
		Trigger:       "manual",
	})
	if cmd == nil {
		t.Fatal("expected compact result command")
	}
	if m.conv.Compact.Active {
		t.Fatal("expected compact active state to clear")
	}
	if m.conv.Compact.LastError {
		t.Fatal("expected success compact state")
	}
	if !strings.Contains(m.conv.Compact.LastResult, "Condensed 2 earlier messages.") {
		t.Fatalf("unexpected compact result text: %q", m.conv.Compact.LastResult)
	}
	if len(m.conv.Messages) != 0 {
		t.Fatalf("expected messages cleared after compact, got %#v", m.conv.Messages)
	}
}

func TestHandleCompactResultStoresVisibleErrorState(t *testing.T) {
	m := &model{
		conv: appoutput.ConversationModel{
			Compact: appoutput.CompactState{Active: true},
		},
	}

	cmd := m.handleCompactResult(appoutput.CompactResultMsg{
		Error: context.DeadlineExceeded,
	})
	_ = cmd
	if !m.conv.Compact.LastError {
		t.Fatal("expected error compact state")
	}
	if !strings.Contains(m.conv.Compact.LastResult, "Compaction could not be completed") {
		t.Fatalf("unexpected compact error text: %q", m.conv.Compact.LastResult)
	}
}
