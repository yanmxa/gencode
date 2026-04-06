package app

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	appoutput "github.com/yanmxa/gencode/internal/app/output"
	appprovider "github.com/yanmxa/gencode/internal/app/provider"
	coreprovider "github.com/yanmxa/gencode/internal/provider"
)

type tokenLimitRuntime struct {
	lastReq    tokenLimitFetchRequest
	fetchCalls int
}

func (r *tokenLimitRuntime) SuggestPromptCmd(promptSuggestionRequest) tea.Cmd { return nil }
func (r *tokenLimitRuntime) CompactCmd(compactRequest) tea.Cmd                { return nil }
func (r *tokenLimitRuntime) StartStream(streamRequest) streamStartResult      { return streamStartResult{} }
func (r *tokenLimitRuntime) FetchTokenLimitsCmd(req tokenLimitFetchRequest) tea.Cmd {
	r.fetchCalls++
	r.lastReq = req
	return func() tea.Msg { return nil }
}

func newTokenLimitStore(t *testing.T) *coreprovider.Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	store, err := coreprovider.NewStore()
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	return store
}

func TestHandleTokenLimitCommand_SetAndShowCustomOverride(t *testing.T) {
	store := newTokenLimitStore(t)
	m := &model{
		provider: providerStateForTest(store, "gpt-5", coreprovider.ProviderOpenAI, coreprovider.AuthAPIKey),
	}

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

	m.provider.InputTokens = 50000
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
	if !strings.Contains(result, "Current usage: 50K tokens (25.0%)") {
		t.Fatalf("expected usage summary, got %q", result)
	}
}

func TestHandleTokenLimitCommand_UsesCachedModelLimitsWhenNoOverride(t *testing.T) {
	store := newTokenLimitStore(t)
	if err := store.CacheModels(coreprovider.ProviderAnthropic, coreprovider.AuthAPIKey, []coreprovider.ModelInfo{
		{ID: "claude-sonnet", InputTokenLimit: 200000, OutputTokenLimit: 16000},
	}); err != nil {
		t.Fatalf("CacheModels() error = %v", err)
	}

	m := &model{
		provider: providerStateForTest(store, "claude-sonnet", coreprovider.ProviderAnthropic, coreprovider.AuthAPIKey),
	}

	result, cmd, err := handleTokenLimitCommand(context.Background(), m, "")
	if err != nil {
		t.Fatalf("handleTokenLimitCommand() error = %v", err)
	}
	if cmd != nil {
		t.Fatal("did not expect fetch cmd when cached limits exist")
	}
	if !strings.Contains(result, "Input:  200K tokens") || !strings.Contains(result, "Output: 16K tokens") {
		t.Fatalf("unexpected cached limit display: %q", result)
	}
	if strings.Contains(result, "custom override") {
		t.Fatalf("did not expect custom marker for cached limits: %q", result)
	}
}

func TestHandleTokenLimitCommand_TriggersFetchWhenLimitsUnknown(t *testing.T) {
	store := newTokenLimitStore(t)
	rt := &tokenLimitRuntime{}
	m := &model{
		cwd:      "/repo",
		output:   appoutput.New(80, nil),
		runtime:  rt,
		provider: providerStateForTest(store, "gpt-unknown", coreprovider.ProviderOpenAI, coreprovider.AuthAPIKey),
	}

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
	if !m.provider.FetchingLimits {
		t.Fatal("expected FetchingLimits to be set")
	}
	if rt.fetchCalls != 1 {
		t.Fatalf("expected one fetch request, got %d", rt.fetchCalls)
	}
	if rt.lastReq.ModelID != "gpt-unknown" || rt.lastReq.Cwd != "/repo" {
		t.Fatalf("unexpected fetch request: %+v", rt.lastReq)
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
	m = &model{
		provider: providerStateForTest(store, "gpt-5", coreprovider.ProviderOpenAI, coreprovider.AuthAPIKey),
	}
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
	m.provider.ThinkingLevel = coreprovider.ThinkingNormal
	m.provider.ThinkingOverride = coreprovider.ThinkingUltra
	if got := m.effectiveThinkingLevel(); got != coreprovider.ThinkingUltra {
		t.Fatalf("effectiveThinkingLevel() = %v, want %v", got, coreprovider.ThinkingUltra)
	}
}

func providerStateForTest(store *coreprovider.Store, modelID string, p coreprovider.Provider, auth coreprovider.AuthMethod) appprovider.State {
	return appprovider.State{
		Store: store,
		CurrentModel: &coreprovider.CurrentModelInfo{
			ModelID:    modelID,
			Provider:   p,
			AuthMethod: auth,
		},
	}
}
