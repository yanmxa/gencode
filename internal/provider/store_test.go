package provider

import "testing"

func TestStore_PersistsConnectionsCurrentModelSearchProviderAndTokenLimits(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if err := store.Connect(ProviderOpenAI, AuthAPIKey); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if err := store.SetCurrentModel("gpt-5", ProviderOpenAI, AuthAPIKey); err != nil {
		t.Fatalf("SetCurrentModel() error = %v", err)
	}
	if err := store.SetSearchProvider("brave"); err != nil {
		t.Fatalf("SetSearchProvider() error = %v", err)
	}
	if err := store.SetTokenLimit("gpt-5", 200000, 32000); err != nil {
		t.Fatalf("SetTokenLimit() error = %v", err)
	}

	reloaded, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore(reload) error = %v", err)
	}

	if !reloaded.IsConnected(ProviderOpenAI, AuthAPIKey) {
		t.Fatal("expected OpenAI API key connection to persist")
	}
	current := reloaded.GetCurrentModel()
	if current == nil || current.ModelID != "gpt-5" || current.Provider != ProviderOpenAI || current.AuthMethod != AuthAPIKey {
		t.Fatalf("unexpected current model after reload: %#v", current)
	}
	if reloaded.GetSearchProvider() != "brave" {
		t.Fatalf("search provider = %q, want %q", reloaded.GetSearchProvider(), "brave")
	}
	in, out, ok := reloaded.GetTokenLimit("gpt-5")
	if !ok || in != 200000 || out != 32000 {
		t.Fatalf("unexpected token limit after reload: in=%d out=%d ok=%v", in, out, ok)
	}
}

func TestStore_SetTokenLimitUpdatesCachedModelCopy(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	models := []ModelInfo{
		{ID: "gpt-5", Name: "GPT-5"},
		{ID: "gpt-5-mini", Name: "GPT-5 mini"},
	}
	if err := store.CacheModels(ProviderOpenAI, AuthAPIKey, models); err != nil {
		t.Fatalf("CacheModels() error = %v", err)
	}

	cachedBefore, ok := store.GetCachedModels(ProviderOpenAI, AuthAPIKey)
	if !ok {
		t.Fatal("expected cached models")
	}

	if err := store.SetTokenLimit("gpt-5", 256000, 64000); err != nil {
		t.Fatalf("SetTokenLimit() error = %v", err)
	}

	cachedAfter, ok := store.GetCachedModels(ProviderOpenAI, AuthAPIKey)
	if !ok {
		t.Fatal("expected cached models after override")
	}
	if cachedAfter[0].InputTokenLimit != 256000 || cachedAfter[0].OutputTokenLimit != 64000 {
		t.Fatalf("expected cached override applied, got %#v", cachedAfter[0])
	}
	if cachedAfter[1].InputTokenLimit != 0 || cachedAfter[1].OutputTokenLimit != 0 {
		t.Fatalf("expected unrelated model unchanged, got %#v", cachedAfter[1])
	}
	if cachedBefore[0].InputTokenLimit != 0 || cachedBefore[0].OutputTokenLimit != 0 {
		t.Fatalf("expected previously returned cached slice to remain unchanged, got %#v", cachedBefore[0])
	}
}

func TestStore_ClearTokenLimitAndSearchProvider(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if err := store.SetSearchProvider("exa"); err != nil {
		t.Fatalf("SetSearchProvider() error = %v", err)
	}
	if err := store.SetTokenLimit("claude", 200000, 16000); err != nil {
		t.Fatalf("SetTokenLimit() error = %v", err)
	}
	if err := store.ClearSearchProvider(); err != nil {
		t.Fatalf("ClearSearchProvider() error = %v", err)
	}
	if err := store.ClearTokenLimit("claude"); err != nil {
		t.Fatalf("ClearTokenLimit() error = %v", err)
	}

	if got := store.GetSearchProvider(); got != "" {
		t.Fatalf("search provider = %q, want empty", got)
	}
	if _, _, ok := store.GetTokenLimit("claude"); ok {
		t.Fatal("expected token limit override to be cleared")
	}
}
